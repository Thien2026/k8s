package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

const defaultPrometheusURL = "http://kube-prometheus-stack-prometheus.monitoring.svc:9090"

type promResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Value []any `json:"value"`
		} `json:"result"`
	} `json:"data"`
}

func (h *Handler) prometheusBaseURL() string {
	// Dùng endpoint nội bộ trong cluster để tránh lộ credentials ra client.
	return defaultPrometheusURL
}

func (h *Handler) queryPrometheusInstant(r *http.Request, expr string) (float64, error) {
	base := strings.TrimRight(h.prometheusBaseURL(), "/")
	u, err := url.Parse(base + "/api/v1/query")
	if err != nil {
		return 0, err
	}
	q := u.Query()
	q.Set("query", expr)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, u.String(), nil)
	if err != nil {
		return 0, err
	}
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return 0, fmt.Errorf("prometheus status %d", resp.StatusCode)
	}

	var out promResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, err
	}
	if out.Status != "success" || len(out.Data.Result) == 0 || len(out.Data.Result[0].Value) < 2 {
		return 0, nil
	}
	raw, _ := out.Data.Result[0].Value[1].(string)
	if raw == "" {
		return 0, nil
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, nil
	}
	return v, nil
}

func (h *Handler) GetProjectMonitoring(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	p, ok := h.requireProjectAccess(w, r, slug)
	if !ok {
		return
	}

	env := strings.TrimSpace(r.URL.Query().Get("env"))
	if env != "prod" {
		env = "dev"
	}
	namespace := p.NamespaceDev
	if env == "prod" {
		namespace = p.NamespaceProd
	}

	cpuExpr := fmt.Sprintf(`sum(rate(container_cpu_usage_seconds_total{namespace="%s",container!="",container!="POD"}[5m]))`, namespace)
	memExpr := fmt.Sprintf(`sum(container_memory_working_set_bytes{namespace="%s",container!="",container!="POD"})`, namespace)
	restartExpr := fmt.Sprintf(`sum(increase(kube_pod_container_status_restarts_total{namespace="%s"}[1h]))`, namespace)
	podExpr := fmt.Sprintf(`count(kube_pod_status_phase{namespace="%s",phase="Running"})`, namespace)

	cpu, cpuErr := h.queryPrometheusInstant(r, cpuExpr)
	memBytes, memErr := h.queryPrometheusInstant(r, memExpr)
	restarts, restErr := h.queryPrometheusInstant(r, restartExpr)
	runningPods, podErr := h.queryPrometheusInstant(r, podExpr)

	payload := map[string]any{
		"env":            env,
		"namespace":      namespace,
		"cpu_cores_5m":   cpu,
		"memory_bytes":   memBytes,
		"memory_mib":     memBytes / 1024 / 1024,
		"restarts_1h":    restarts,
		"running_pods":   runningPods,
		"grafana_url":    trimURL(h.cfg.GrafanaURL),
		"dashboard_url":  grafanaNamespaceDashboardURL(h.cfg.GrafanaURL, namespace),
		"prometheus_url": trimURL(h.prometheusBaseURL()),
	}

	if cpuErr != nil || memErr != nil || restErr != nil || podErr != nil {
		payload["warning"] = "Một số metric chưa sẵn sàng"
	}
	writeJSON(w, http.StatusOK, payload)
}
