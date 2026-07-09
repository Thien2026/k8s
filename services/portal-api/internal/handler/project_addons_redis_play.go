package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/Thien2026/k8s/services/portal-api/internal/auth"
	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"
)

type redisPlayRequest struct {
	Environment string `json:"environment"`
	Action      string `json:"action"`
	Key         string `json:"key"`
	Value       string `json:"value"`
	TTLSeconds  int    `json:"ttl_seconds"`
}

type redisPlayResponse struct {
	OK          bool   `json:"ok"`
	Action      string `json:"action"`
	Result      string `json:"result,omitempty"`
	Key         string `json:"key,omitempty"`
	Value       string `json:"value,omitempty"`
	LatencyMS   int64  `json:"latency_ms,omitempty"`
	Error       string `json:"error,omitempty"`
	RedisURL    string `json:"redis_url_masked,omitempty"`
	AppProbeURL string `json:"app_probe_url,omitempty"`
}

func normalizeRedisPlayKey(key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", errRedisPlayKey
	}
	if len(key) > 200 {
		return "", errRedisPlayKeyLong
	}
	if strings.ContainsAny(key, " \t\n\r") {
		return "", errRedisPlayKey
	}
	return key, nil
}

type redisPlayError string

func (e redisPlayError) Error() string { return string(e) }

func errRedisPlay(msg string) error { return redisPlayError(msg) }

var (
	errRedisPlayKey     = errRedisPlay("key không hợp lệ")
	errRedisPlayKeyLong = errRedisPlay("key quá dài (tối đa 200)")
)

func (h *Handler) projectRedisURL(ctx context.Context, projectID int64, env string) (string, error) {
	vars, err := h.envVarsMap(ctx, projectID, env)
	if err != nil {
		return "", err
	}
	u := strings.TrimSpace(vars["REDIS_URL"])
	if u == "" {
		return "", errRedisPlay("REDIS_URL chưa có — bật Redis addon trước")
	}
	return u, nil
}

func (h *Handler) redisPlayClient(redisURL string) (*redis.Client, error) {
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, errRedisPlay("REDIS_URL không hợp lệ: " + err.Error())
	}
	opt.DialTimeout = 5 * time.Second
	opt.ReadTimeout = 5 * time.Second
	opt.WriteTimeout = 5 * time.Second
	return redis.NewClient(opt), nil
}

func (h *Handler) redisAppProbeURL(p projectRow, env string) string {
	host := strings.TrimSpace(h.cfg.PlatformDomain)
	if host == "" {
		host = strings.TrimSpace(h.cfg.ApexDomain)
	}
	if host == "" {
		return ""
	}
	suffix := "dev"
	if env == "prod" {
		suffix = "prod"
	}
	return "https://" + p.Slug + "-" + suffix + "." + host + "/api/redis/ping"
}

// RedisAddonPlay POST /projects/{slug}/addons/redis/play
func (h *Handler) RedisAddonPlay(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	p, ok := h.requireProjectAccess(w, r, slug)
	if !ok {
		return
	}
	u, _ := auth.UserFromContext(r.Context())
	if !h.canManageProject(r.Context(), u, p.ID) {
		writeAccessDenied(w)
		return
	}

	var body redisPlayRequest
	_ = json.NewDecoder(r.Body).Decode(&body)
	env := strings.TrimSpace(body.Environment)
	if env == "" {
		env = strings.TrimSpace(r.URL.Query().Get("environment"))
	}
	if env == "" {
		env = "dev"
	}
	if !validAddonEnv(env) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "environment phải là dev hoặc prod"})
		return
	}

	redisURL, err := h.projectRedisURL(r.Context(), p.ID, env)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	action := strings.ToLower(strings.TrimSpace(body.Action))
	if action == "" {
		action = "ping"
	}

	client, err := h.redisPlayClient(redisURL)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()

	resp := redisPlayResponse{
		Action:      action,
		RedisURL:    maskRedisURL(redisURL),
		AppProbeURL: h.redisAppProbeURL(p, env),
	}
	start := time.Now()

	switch action {
	case "ping":
		if err := client.Ping(ctx).Err(); err != nil {
			resp.Error = err.Error()
		} else {
			resp.OK = true
			resp.Result = "PONG"
		}
	case "info":
		s, err := client.Info(ctx, "server", "memory", "clients").Result()
		if err != nil {
			resp.Error = err.Error()
		} else {
			resp.OK = true
			if len(s) > 4000 {
				s = s[:4000] + "\n…"
			}
			resp.Result = s
		}
	case "get":
		key, err := normalizeRedisPlayKey(body.Key)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		val, err := client.Get(ctx, key).Result()
		if err == redis.Nil {
			resp.OK = true
			resp.Key = key
			resp.Result = "(nil)"
		} else if err != nil {
			resp.Error = err.Error()
		} else {
			resp.OK = true
			resp.Key = key
			resp.Value = val
			resp.Result = val
		}
	case "set":
		key, err := normalizeRedisPlayKey(body.Key)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		val := body.Value
		if len(val) > 4096 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "value tối đa 4096 ký tự"})
			return
		}
		ttl := time.Duration(body.TTLSeconds) * time.Second
		var setErr error
		if body.TTLSeconds > 0 {
			setErr = client.Set(ctx, key, val, ttl).Err()
		} else {
			setErr = client.Set(ctx, key, val, 0).Err()
		}
		if setErr != nil {
			resp.Error = setErr.Error()
		} else {
			resp.OK = true
			resp.Key = key
			resp.Value = val
			resp.Result = "OK"
		}
	case "del":
		key, err := normalizeRedisPlayKey(body.Key)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		n, err := client.Del(ctx, key).Result()
		if err != nil {
			resp.Error = err.Error()
		} else {
			resp.OK = true
			resp.Key = key
			resp.Result = "deleted"
			if n == 0 {
				resp.Result = "key không tồn tại"
			}
		}
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "action không hỗ trợ — dùng ping, get, set, del, info",
		})
		return
	}

	resp.LatencyMS = time.Since(start).Milliseconds()
	status := http.StatusOK
	if !resp.OK {
		status = http.StatusBadGateway
	}
	writeJSON(w, status, resp)
}
