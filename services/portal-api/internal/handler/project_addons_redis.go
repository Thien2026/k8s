package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net/http"
	"strconv"
	"strings"

	"github.com/Thien2026/k8s/services/portal-api/internal/auth"
	"github.com/Thien2026/k8s/services/portal-api/internal/plugins"
	"github.com/go-chi/chi/v5"
)

type redisAddonPodView struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Restarts int    `json:"restarts"`
	Ready    bool   `json:"ready"`
}

func redisAddonNodePort(slug, env string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(slug + "|redis|" + env))
	return 30000 + int(h.Sum32()%2768)
}

func (h *Handler) redisZoneDomain() string {
	if z := strings.TrimSpace(h.cfg.RedisZone); z != "" {
		return z
	}
	if d := strings.TrimSpace(h.cfg.ApexDomain); d != "" {
		return "redis." + d
	}
	pd := strings.TrimSpace(h.cfg.PlatformDomain)
	parts := strings.Split(pd, ".")
	if len(parts) >= 2 {
		return "redis." + strings.Join(parts[len(parts)-2:], ".")
	}
	return ""
}

func redisAddonExternalHostname(slug, env, redisZone string) string {
	if redisZone == "" {
		return ""
	}
	return slug + "-redis-" + env + "." + redisZone
}

func (h *Handler) redisAddonExternalURL(slug, env, password string) (hostname string, port int, raw string) {
	if env != "dev" {
		return "", 0, ""
	}
	zone := h.redisZoneDomain()
	hostname = redisAddonExternalHostname(slug, env, zone)
	if hostname == "" {
		return "", 0, ""
	}
	port = redisAddonNodePort(slug, env)
	raw = fmt.Sprintf("redis://:%s@%s:%d/0", password, hostname, port)
	return hostname, port, raw
}

func (h *Handler) applyRedisNetworkPolicy(ctx context.Context, release, ns string) error {
	np := map[string]any{
		"apiVersion": "networking.k8s.io/v1",
		"kind":       "NetworkPolicy",
		"metadata": map[string]any{
			"name": release + "-netpol",
			"labels": map[string]string{
				"app.kubernetes.io/name":     "redis",
				"app.kubernetes.io/instance": release,
			},
		},
		"spec": map[string]any{
			"podSelector": map[string]any{
				"matchLabels": map[string]string{
					"app.kubernetes.io/name":     "redis",
					"app.kubernetes.io/instance": release,
				},
			},
			"policyTypes": []string{"Ingress"},
			"ingress": []map[string]any{
				{
					"from": []map[string]any{
						{"podSelector": map[string]any{}},
					},
					"ports": []map[string]any{
						{"protocol": "TCP", "port": redisAddonServicePort},
					},
				},
			},
		},
	}
	npJSON, _ := json.Marshal(np)
	return h.rancher.ApplyNamespacedObject(ctx, "", "/apis/networking.k8s.io/v1/networkpolicies", ns, npJSON)
}

func (h *Handler) redisAddonPodView(ctx context.Context, release, ns string) *redisAddonPodView {
	if h.rancher == nil || !h.rancher.Enabled() {
		return nil
	}
	list, err := h.rancher.ListK8s(ctx, "", "pods", ns, 1, 50)
	if err != nil {
		return nil
	}
	prefix := release + "-"
	for _, it := range list.Items {
		if !strings.HasPrefix(it.Name, prefix) {
			continue
		}
		return &redisAddonPodView{
			Name:     it.Name,
			Status:   it.Status,
			Restarts: it.Restarts,
			Ready:    it.Ready,
		}
	}
	return nil
}

func (h *Handler) enrichRedisAddonAPIView(ctx context.Context, p projectRow, v projectAddonView, exposeFullURL bool, password string) redisAddonAPIView {
	out := h.buildRedisAddonAPIView(ctx, p, v, exposeFullURL)
	out.Pod = h.redisAddonPodView(ctx, out.K8sRelease, h.projectNamespace(p, v.Environment))
	if password == "" && exposeFullURL {
		vars, _ := h.envVarsMap(ctx, p.ID, v.Environment)
		if u := strings.TrimSpace(vars["REDIS_URL"]); u != "" {
			if _, pass, ok := parseRedisURLPassword(u); ok {
				password = pass
			}
		}
	}
	if password != "" {
		host, port, ext := h.redisAddonExternalURL(p.Slug, v.Environment, password)
		if ext != "" {
			out.ExternalHostname = host
			out.ExternalPort = port
			out.ConnectionURLExternalMasked = maskRedisURL(ext)
			if exposeFullURL {
				out.ConnectionURLExternal = ext
			}
		}
	}
	return out
}

func parseRedisURLPassword(raw string) (host, password string, ok bool) {
	raw = strings.TrimSpace(raw)
	schemeEnd := strings.Index(raw, "://")
	if schemeEnd < 0 {
		return "", "", false
	}
	rest := raw[schemeEnd+3:]
	at := strings.LastIndex(rest, "@")
	if at < 0 {
		return rest, "", true
	}
	authPart := rest[:at]
	host = rest[at+1:]
	if i := strings.Index(authPart, ":"); i >= 0 {
		password = authPart[i+1:]
	}
	return host, password, true
}

// RestartRedisAddon POST /projects/{slug}/addons/redis/restart
func (h *Handler) RestartRedisAddon(w http.ResponseWriter, r *http.Request) {
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
	if !h.rancherRequired(w, r) || !h.pluginEnabled(r.Context(), plugins.Rancher) {
		return
	}

	var body struct {
		Environment string `json:"environment"`
	}
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

	item, err := h.getProjectAddon(r.Context(), p.ID, "redis", env)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Redis chưa được bật"})
		return
	}
	release := item.K8sRelease
	if release == "" {
		release = redisAddonRelease(p.Slug, env)
	}
	ns := h.projectNamespace(p, env)
	if _, ok := h.guardK8sWrite(w, r, ns); !ok {
		return
	}
	if err := h.rancher.RolloutRestartStatefulSet(r.Context(), clusterQuery(r), ns, release); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	auditAction(r.Context(), h, r, "addon.redis.restart", slug, map[string]any{
		"environment": env, "release": release, "by": u.Email,
	})
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "Đang restart Redis — pod mới sẽ lên trong vài giây",
	})
}

// GetRedisAddonLogs GET /projects/{slug}/addons/redis/logs
func (h *Handler) GetRedisAddonLogs(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	p, ok := h.requireProjectAccess(w, r, slug)
	if !ok {
		return
	}
	if !h.rancherRequired(w, r) {
		return
	}
	env := strings.TrimSpace(r.URL.Query().Get("environment"))
	if env == "" {
		env = "dev"
	}
	if !validAddonEnv(env) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "environment phải là dev hoặc prod"})
		return
	}
	item, err := h.getProjectAddon(r.Context(), p.ID, "redis", env)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Redis chưa được bật"})
		return
	}
	release := item.K8sRelease
	if release == "" {
		release = redisAddonRelease(p.Slug, env)
	}
	ns := h.projectNamespace(p, env)
	if _, ok := h.guardK8sRead(w, r, "pods", ns); !ok {
		return
	}
	pod := release + "-0"
	tail := 200
	if v := strings.TrimSpace(r.URL.Query().Get("tail")); v != "" {
		if n, err := parsePositiveInt(v); err == nil && n > 0 && n <= 2000 {
			tail = n
		}
	}
	logs, err := h.rancher.GetPodLogs(r.Context(), clusterQuery(r), ns, pod, "redis", tail)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"pod":         pod,
		"namespace":   ns,
		"environment": env,
		"logs":        logs,
	})
}

func parsePositiveInt(s string) (int, error) {
	return strconv.Atoi(strings.TrimSpace(s))
}
