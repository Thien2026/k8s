package handler

import (
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

const minioGrafanaDashboardUID = "platform-minio-addon"

func grafanaMinioAddonDashboardURL(base, namespace, release string) string {
	base = trimURL(base)
	if base == "" || namespace == "" || release == "" {
		return ""
	}
	u := base + "/d/" + minioGrafanaDashboardUID + "/platform-minio-addon"
	q := url.Values{}
	q.Set("var-namespace", namespace)
	q.Set("var-release", release)
	q.Set("orgId", "1")
	q.Set("from", "now-6h")
	q.Set("to", "now")
	q.Set("timezone", "browser")
	return u + "?" + q.Encode()
}

func minioAddonServiceMonitor(release, ns string) map[string]any {
	return map[string]any{
		"apiVersion": "monitoring.coreos.com/v1",
		"kind":       "ServiceMonitor",
		"metadata": map[string]any{
			"name":      release + "-metrics",
			"namespace": ns,
			"labels": map[string]string{
				"app.kubernetes.io/name":     "minio",
				"app.kubernetes.io/instance": release,
				"release":                    "kube-prometheus-stack",
			},
		},
		"spec": map[string]any{
			"selector": map[string]any{
				"matchLabels": map[string]string{
					"app.kubernetes.io/name":     "minio",
					"app.kubernetes.io/instance": release,
					"platform/metrics":           "true",
				},
			},
			"namespaceSelector": map[string]any{
				"matchNames": []string{ns},
			},
			"endpoints": []map[string]any{
				{
					"port":     "api",
					"interval": "30s",
					"path":     "/minio/v2/metrics/cluster",
				},
			},
		},
	}
}

func minioStatsPromQL(namespace, release string) map[string]string {
	ns := strings.ReplaceAll(namespace, `"`, `\"`)
	rel := strings.ReplaceAll(release, `"`, `\"`)
	sel := fmt.Sprintf(`namespace="%s",pod=~"%s-.*"`, ns, rel)
	return map[string]string{
		"usable_total": fmt.Sprintf(`minio_cluster_capacity_usable_total_bytes{%s}`, sel),
		"usable_free":  fmt.Sprintf(`minio_cluster_capacity_usable_free_bytes{%s}`, sel),
		"usage_total":  fmt.Sprintf(`minio_cluster_usage_total_bytes{%s}`, sel),
		"nodes_online": fmt.Sprintf(`minio_cluster_nodes_online_total{%s}`, sel),
		"s3_rate":      fmt.Sprintf(`sum(rate(minio_s3_requests_total{%s}[5m]))`, sel),
		"ttfb":         fmt.Sprintf(`histogram_quantile(0.95, sum(rate(minio_s3_ttfb_seconds_bucket{%s}[5m])) by (le))`, sel),
	}
}

func (h *Handler) minioAddonPromMetrics(r *http.Request, namespace, release string) map[string]any {
	out := map[string]any{"available": false}
	if !h.monitoringConfigured() {
		out["hint"] = "Monitoring stack chưa cấu hình"
		return out
	}
	window, windowDur, step := redisMetricsWindow(r)
	prom := minioStatsPromQL(namespace, release)
	usableTotal, _ := h.queryPrometheusInstant(r, prom["usable_total"])
	usableFree, _ := h.queryPrometheusInstant(r, prom["usable_free"])
	usageTotal, _ := h.queryPrometheusInstant(r, prom["usage_total"])
	nodesOnline, _ := h.queryPrometheusInstant(r, prom["nodes_online"])
	s3Rate, _ := h.queryPrometheusInstant(r, prom["s3_rate"])
	ttfb, _ := h.queryPrometheusInstant(r, prom["ttfb"])

	usedPct := 0.0
	if usableTotal > 0 {
		usedPct = (usableTotal - usableFree) / usableTotal * 100
	}

	end := time.Now()
	start := end.Add(-windowDur)
	usageSeries, _ := h.queryPrometheusRange(r, prom["usage_total"], start, end, step)
	s3Series, _ := h.queryPrometheusRange(r, prom["s3_rate"], start, end, step)

	hasAny := usableTotal > 0 || usageTotal > 0 || nodesOnline > 0 || s3Rate > 0 || len(usageSeries) > 0
	if !hasAny {
		out["hint"] = "Chưa có metric — re-provision MinIO để gắn ServiceMonitor + MINIO_PROMETHEUS_AUTH_TYPE=public, chờ ~1 phút"
		return out
	}
	out["available"] = true
	out["window"] = window
	out["usable_total_bytes"] = usableTotal
	out["usable_free_bytes"] = usableFree
	out["usage_total_bytes"] = usageTotal
	out["used_pct"] = math.Round(usedPct*10) / 10
	out["nodes_online"] = nodesOnline
	out["s3_requests_per_sec"] = math.Round(s3Rate*100) / 100
	out["s3_ttfb_p95_sec"] = math.Round(ttfb*1000) / 1000
	out["usage_series"] = promSeriesToPoints(usageSeries)
	out["s3_series"] = promSeriesToPoints(s3Series)
	return out
}

func (h *Handler) minioAddonPodMetrics(r *http.Request, namespace, release string) map[string]any {
	out := map[string]any{"available": false}
	if !h.monitoringConfigured() {
		return out
	}
	pod := release + "-0"
	nsEsc := strings.ReplaceAll(namespace, `"`, `\"`)
	podEsc := strings.ReplaceAll(pod, `"`, `\"`)
	memExpr := fmt.Sprintf(`container_memory_working_set_bytes{namespace="%s",pod="%s",container="minio"}`, nsEsc, podEsc)
	cpuExpr := fmt.Sprintf(`rate(container_cpu_usage_seconds_total{namespace="%s",pod="%s",container="minio"}[5m])`, nsEsc, podEsc)
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

// GetMinioAddonStats GET /projects/{slug}/addons/minio/stats
func (h *Handler) GetMinioAddonStats(w http.ResponseWriter, r *http.Request) {
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
	addon, err := h.getProjectAddon(r.Context(), p.ID, "minio", env)
	if err != nil || addon == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "MinIO chưa được bật"})
		return
	}
	release := addon.K8sRelease
	if release == "" {
		release = minioAddonRelease(p.Slug, env)
	}
	ns := h.projectNamespace(p, env)
	window, _, _ := redisMetricsWindow(r)
	prom := h.minioAddonPromMetrics(r, ns, release)
	k8s := h.minioAddonPodMetrics(r, ns, release)

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":                    true,
		"environment":           env,
		"window":                window,
		"topology":              normalizeMinioTopology(addon.Topology),
		"status":                addon.Status,
		"release":               release,
		"namespace":             ns,
		"grafana_dashboard_url": grafanaMinioAddonDashboardURL(h.cfg.GrafanaURL, ns, release),
		"prometheus":            prom,
		"k8s":                   k8s,
	})
}
