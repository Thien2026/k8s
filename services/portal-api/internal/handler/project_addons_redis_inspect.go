package handler

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"
)

func parseRedisInfoMap(raw string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		i := strings.Index(line, ":")
		if i <= 0 {
			continue
		}
		out[strings.TrimSpace(line[:i])] = strings.TrimSpace(line[i+1:])
	}
	return out
}

func redisInfoFloat(m map[string]string, key string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(m[key]), 64)
	if err != nil {
		return 0
	}
	return v
}

func redisInfoInt(m map[string]string, key string) int64 {
	v, err := strconv.ParseInt(strings.TrimSpace(m[key]), 10, 64)
	if err != nil {
		return 0
	}
	return v
}

func redisKeyspaceTotalKeys(m map[string]string) int64 {
	var total int64
	for k, v := range m {
		if !strings.HasPrefix(k, "db") {
			continue
		}
		// db0:keys=12,expires=3,avg_ttl=12345
		for _, part := range strings.Split(v, ",") {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(part, "keys=") {
				n, _ := strconv.ParseInt(strings.TrimPrefix(part, "keys="), 10, 64)
				total += n
			}
		}
	}
	return total
}

func normalizeRedisScanPattern(pattern string) (string, error) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return "*", nil
	}
	if len(pattern) > 120 {
		return "", errRedisPlay("pattern tối đa 120 ký tự")
	}
	return pattern, nil
}

func redisKeyPreview(ctx context.Context, client *redis.Client, key, typ string) string {
	const maxLen = 160
	switch typ {
	case "string":
		s, err := client.Get(ctx, key).Result()
		if err == redis.Nil {
			return "(nil)"
		}
		if err != nil {
			return "—"
		}
		return truncateRedisPreview(s, maxLen)
	case "list":
		n, _ := client.LLen(ctx, key).Result()
		items, _ := client.LRange(ctx, key, 0, 2).Result()
		return fmt.Sprintf("list · %d phần tử · %s", n, truncateRedisPreview(strings.Join(items, ", "), maxLen))
	case "hash":
		n, _ := client.HLen(ctx, key).Result()
		return fmt.Sprintf("hash · %d field", n)
	case "set":
		n, _ := client.SCard(ctx, key).Result()
		items, _ := client.SRandMemberN(ctx, key, 3).Result()
		return fmt.Sprintf("set · %d phần tử · %s", n, truncateRedisPreview(strings.Join(items, ", "), maxLen))
	case "zset":
		n, _ := client.ZCard(ctx, key).Result()
		return fmt.Sprintf("zset · %d phần tử", n)
	case "stream":
		n, _ := client.XLen(ctx, key).Result()
		return fmt.Sprintf("stream · %d entry", n)
	default:
		return typ
	}
}

func truncateRedisPreview(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

func formatRedisTTL(ttl time.Duration) string {
	if ttl < 0 {
		return "∞"
	}
	if ttl == 0 {
		return "0s"
	}
	sec := int64(ttl.Seconds())
	if sec < 60 {
		return fmt.Sprintf("%ds", sec)
	}
	if sec < 3600 {
		return fmt.Sprintf("%dm", sec/60)
	}
	if sec < 86400 {
		return fmt.Sprintf("%dh", sec/3600)
	}
	return fmt.Sprintf("%dd", sec/86400)
}

func (h *Handler) redisAddonPodMetrics(r *http.Request, namespace, release string) map[string]any {
	out := map[string]any{"available": false}
	if !h.monitoringConfigured() {
		return out
	}
	pod := release + "-0"
	nsEsc := strings.ReplaceAll(namespace, `"`, `\"`)
	podEsc := strings.ReplaceAll(pod, `"`, `\"`)
	memExpr := fmt.Sprintf(`container_memory_working_set_bytes{namespace="%s",pod="%s",container="redis"}`, nsEsc, podEsc)
	cpuExpr := fmt.Sprintf(`rate(container_cpu_usage_seconds_total{namespace="%s",pod="%s",container="redis"}[5m])`, nsEsc, podEsc)
	mem, errMem := h.queryPrometheusInstant(r, memExpr)
	cpu, errCPU := h.queryPrometheusInstant(r, cpuExpr)
	if errMem != nil && errCPU != nil {
		out["error"] = "prometheus không phản hồi"
		return out
	}
	out["available"] = true
	out["pod"] = pod
	out["memory_mib"] = math.Round(mem/1024/1024*10) / 10
	out["cpu_cores"] = math.Round(cpu*1000) / 1000
	return out
}

func promSeriesToPoints(series [][2]float64) []map[string]any {
	out := make([]map[string]any, 0, len(series))
	for _, pt := range series {
		out = append(out, map[string]any{"t": int64(pt[0]), "v": pt[1]})
	}
	return out
}

func redisMetricsWindow(r *http.Request) (window string, windowDur, step time.Duration) {
	window = strings.TrimSpace(r.URL.Query().Get("window"))
	windowDur = 6 * time.Hour
	step = 5 * time.Minute
	switch window {
	case "15m":
		windowDur = 15 * time.Minute
		step = 30 * time.Second
	case "1h":
		windowDur = 1 * time.Hour
		step = 1 * time.Minute
	case "24h":
		windowDur = 24 * time.Hour
		step = 10 * time.Minute
	default:
		window = "6h"
	}
	return window, windowDur, step
}

func (h *Handler) redisAddonExporterMetrics(r *http.Request, namespace, release string) map[string]any {
	out := map[string]any{"available": false}
	if !h.monitoringConfigured() {
		return out
	}
	window, windowDur, step := redisMetricsWindow(r)
	prom := redisStatsPromQL(namespace, release)
	mem, _ := h.queryPrometheusInstant(r, prom["memory_bytes"])
	clients, _ := h.queryPrometheusInstant(r, prom["clients"])
	ops, _ := h.queryPrometheusInstant(r, prom["ops_rate"])
	end := time.Now()
	start := end.Add(-windowDur)
	memSeries, _ := h.queryPrometheusRange(r, prom["memory_bytes"], start, end, step)
	opsSeries, _ := h.queryPrometheusRange(r, prom["ops_rate"], start, end, step)
	if mem == 0 && clients == 0 && ops == 0 && len(memSeries) == 0 && len(opsSeries) == 0 {
		out["hint"] = "Chưa có metrics redis_exporter — re-provision Redis để bật sidecar."
		return out
	}
	out["available"] = true
	out["window"] = window
	out["memory_bytes"] = mem
	out["memory_mib"] = math.Round(mem/1024/1024*10) / 10
	out["clients"] = clients
	out["ops_per_sec"] = math.Round(ops*100) / 100
	out["memory_series"] = promSeriesToPoints(memSeries)
	out["ops_series"] = promSeriesToPoints(opsSeries)
	return out
}

func redisSlowlogRows(ctx context.Context, client *redis.Client, limit int64) ([]map[string]any, error) {
	entries, err := client.SlowLogGet(ctx, limit).Result()
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		out = append(out, map[string]any{
			"id":          e.ID,
			"timestamp":   e.Time.Unix(),
			"duration_us": e.Duration.Microseconds(),
			"duration_ms": e.Duration.Milliseconds(),
			"command":     strings.Join(e.Args, " "),
			"client_addr": e.ClientAddr,
			"client_name": e.ClientName,
		})
	}
	return out, nil
}

func redisMemoryDoctor(ctx context.Context, client *redis.Client) string {
	s, err := client.Do(ctx, "MEMORY", "DOCTOR").Text()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(s)
}

// GetRedisAddonStats GET /projects/{slug}/addons/redis/stats
func (h *Handler) GetRedisAddonStats(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	p, ok := h.requireProjectAccess(w, r, slug)
	if !ok {
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

	redisURL, err := h.projectRedisURL(r.Context(), p.ID, env)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	client, err := h.redisPlayClient(redisURL)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()

	start := time.Now()
	if err := client.Ping(ctx).Err(); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}

	raw, err := client.Info(ctx, "server", "memory", "clients", "stats", "keyspace", "cpu", "persistence").Result()
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	m := parseRedisInfoMap(raw)
	hits := redisInfoInt(m, "keyspace_hits")
	misses := redisInfoInt(m, "keyspace_misses")
	hitTotal := hits + misses
	hitRate := 0.0
	if hitTotal > 0 {
		hitRate = float64(hits) / float64(hitTotal) * 100
	}

	release := redisAddonRelease(p.Slug, env)
	ns := h.projectNamespace(p, env)

	addonPolicy := "allkeys-lru"
	defaultTTL := 86400
	if addon, err := h.getProjectAddon(r.Context(), p.ID, "redis", env); err == nil && addon != nil {
		addonPolicy = addon.MaxmemoryPolicy
		defaultTTL = addon.DefaultKeyTTLSec
	}

	slowlog, _ := redisSlowlogRows(ctx, client, 15)
	doctor := redisMemoryDoctor(ctx, client)

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":            true,
		"environment":   env,
		"window":        func() string { w, _, _ := redisMetricsWindow(r); return w }(),
		"latency_ms":    time.Since(start).Milliseconds(),
		"redis_version": m["redis_version"],
		"role":          m["role"],
		"uptime_sec":    redisInfoInt(m, "uptime_in_seconds"),
		"policy": map[string]any{
			"configured":      addonPolicy,
			"active":          m["maxmemory_policy"],
			"hint":            redisMaxmemoryPolicyHint(addonPolicy),
			"default_ttl_sec": defaultTTL,
		},
		"grafana_dashboard_url": grafanaRedisAddonDashboardURL(h.cfg.GrafanaURL, ns, release),
		"memory": map[string]any{
			"used_bytes":          redisInfoInt(m, "used_memory"),
			"used_human":          m["used_memory_human"],
			"peak_human":          m["used_memory_peak_human"],
			"maxmemory_human":     m["maxmemory_human"],
			"maxmemory_bytes":     redisInfoInt(m, "maxmemory"),
			"fragmentation_ratio": redisInfoFloat(m, "mem_fragmentation_ratio"),
			"used_pct":            redisMemoryUsedPct(m),
		},
		"clients": map[string]any{
			"connected":       redisInfoInt(m, "connected_clients"),
			"blocked":         redisInfoInt(m, "blocked_clients"),
			"max_clients_cfg": redisInfoInt(m, "maxclients"),
		},
		"ops": map[string]any{
			"instantaneous_ops_per_sec": redisInfoInt(m, "instantaneous_ops_per_sec"),
			"total_commands":            redisInfoInt(m, "total_commands_processed"),
			"keyspace_hits":             hits,
			"keyspace_misses":           misses,
			"hit_rate_pct":              math.Round(hitRate*10) / 10,
		},
		"keys": map[string]any{
			"total": redisKeyspaceTotalKeys(m),
			"raw":   pickRedisKeyspaceLines(m),
		},
		"persistence": map[string]any{
			"aof_enabled":   m["aof_enabled"],
			"rdb_last_save": redisInfoInt(m, "rdb_last_save_time"),
		},
		"slowlog":       slowlog,
		"memory_doctor": doctor,
		"exporter":      h.redisAddonExporterMetrics(r, ns, release),
		"k8s":           h.redisAddonPodMetrics(r, ns, release),
	})
}

func redisMemoryUsedPct(m map[string]string) float64 {
	used := redisInfoFloat(m, "used_memory")
	max := redisInfoFloat(m, "maxmemory")
	if max <= 0 {
		return 0
	}
	return math.Round(used/max*1000) / 10
}

func pickRedisKeyspaceLines(m map[string]string) []string {
	var lines []string
	for k, v := range m {
		if strings.HasPrefix(k, "db") {
			lines = append(lines, k+":"+v)
		}
	}
	return lines
}

type redisKeyRow struct {
	Key     string `json:"key"`
	Type    string `json:"type"`
	TTL     string `json:"ttl"`
	TTLSec  int64  `json:"ttl_sec"`
	NoTTL   bool   `json:"no_ttl"`
	Preview string `json:"preview"`
	Value   string `json:"value,omitempty"`
}

func redisKeyFullValue(ctx context.Context, client *redis.Client, key, typ string) string {
	const maxLen = 4096
	switch typ {
	case "string":
		s, err := client.Get(ctx, key).Result()
		if err != nil {
			return ""
		}
		return truncateRedisPreview(s, maxLen)
	default:
		return ""
	}
}

// GetRedisAddonKeys GET /projects/{slug}/addons/redis/keys
func (h *Handler) GetRedisAddonKeys(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	p, ok := h.requireProjectAccess(w, r, slug)
	if !ok {
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

	pattern, err := normalizeRedisScanPattern(r.URL.Query().Get("pattern"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	limit := 40
	if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 80 {
			limit = n
		}
	}
	cursor := uint64(0)
	if v := strings.TrimSpace(r.URL.Query().Get("cursor")); v != "" {
		if n, err := strconv.ParseUint(v, 10, 64); err == nil {
			cursor = n
		}
	}

	redisURL, err := h.projectRedisURL(r.Context(), p.ID, env)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	client, err := h.redisPlayClient(redisURL)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()

	keys, nextCursor, err := client.Scan(ctx, cursor, pattern, int64(limit)).Result()
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}

	fullValue := strings.TrimSpace(r.URL.Query().Get("full")) == "1"

	rows := make([]redisKeyRow, 0, len(keys))
	var noTTLCount int
	for _, key := range keys {
		typ, err := client.Type(ctx, key).Result()
		if err != nil {
			continue
		}
		ttl, err := client.TTL(ctx, key).Result()
		if err != nil {
			continue
		}
		noTTL := ttl < 0
		if noTTL {
			noTTLCount++
		}
		row := redisKeyRow{
			Key:     key,
			Type:    typ,
			TTL:     formatRedisTTL(ttl),
			TTLSec:  int64(ttl.Seconds()),
			NoTTL:   noTTL,
			Preview: redisKeyPreview(ctx, client, key, typ),
		}
		if fullValue {
			row.Value = redisKeyFullValue(ctx, client, key, typ)
		}
		rows = append(rows, row)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":           true,
		"environment":  env,
		"pattern":      pattern,
		"cursor":       nextCursor,
		"has_more":     nextCursor != 0,
		"count":        len(rows),
		"no_ttl_count": noTTLCount,
		"items":        rows,
	})
}
