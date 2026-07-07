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
			Metric map[string]string `json:"metric"`
			Value  []any             `json:"value"`
		} `json:"result"`
	} `json:"data"`
}

type promRangeResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Values [][]any `json:"values"`
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

func (h *Handler) queryPrometheusRange(r *http.Request, expr string, start, end time.Time, step time.Duration) ([][2]float64, error) {
	base := strings.TrimRight(h.prometheusBaseURL(), "/")
	u, err := url.Parse(base + "/api/v1/query_range")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("query", expr)
	q.Set("start", strconv.FormatInt(start.Unix(), 10))
	q.Set("end", strconv.FormatInt(end.Unix(), 10))
	q.Set("step", fmt.Sprintf("%.0f", step.Seconds()))
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("prometheus status %d", resp.StatusCode)
	}

	var out promRangeResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if out.Status != "success" || len(out.Data.Result) == 0 {
		return [][2]float64{}, nil
	}
	points := make([][2]float64, 0, len(out.Data.Result[0].Values))
	for _, row := range out.Data.Result[0].Values {
		if len(row) < 2 {
			continue
		}
		ts, okTS := row[0].(float64)
		raw, okV := row[1].(string)
		if !okTS || !okV {
			continue
		}
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			continue
		}
		points = append(points, [2]float64{ts, v})
	}
	return points, nil
}

func (h *Handler) queryPrometheusTopVector(r *http.Request, expr, labelKey string, divisor float64) ([]map[string]any, error) {
	base := strings.TrimRight(h.prometheusBaseURL(), "/")
	u, err := url.Parse(base + "/api/v1/query")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("query", expr)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("prometheus status %d", resp.StatusCode)
	}
	var out promResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if out.Status != "success" || len(out.Data.Result) == 0 {
		return []map[string]any{}, nil
	}
	rows := make([]map[string]any, 0, len(out.Data.Result))
	for _, item := range out.Data.Result {
		if len(item.Value) < 2 {
			continue
		}
		name := item.Metric[labelKey]
		if name == "" {
			name = "unknown"
		}
		raw, _ := item.Value[1].(string)
		val, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			continue
		}
		if divisor > 0 {
			val = val / divisor
		}
		rows = append(rows, map[string]any{
			"name":  name,
			"value": val,
		})
	}
	return rows, nil
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
	window := strings.TrimSpace(r.URL.Query().Get("window"))
	windowDur := 6 * time.Hour
	step := 5 * time.Minute
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

	cpuExpr := fmt.Sprintf(`sum(rate(container_cpu_usage_seconds_total{namespace="%s",container!="",container!="POD"}[5m]))`, namespace)
	memExpr := fmt.Sprintf(`sum(container_memory_working_set_bytes{namespace="%s",container!="",container!="POD"})`, namespace)
	restartExpr := fmt.Sprintf(`sum(increase(kube_pod_container_status_restarts_total{namespace="%s"}[1h]))`, namespace)
	podExpr := fmt.Sprintf(`count(kube_pod_status_phase{namespace="%s",phase="Running"})`, namespace)

	cpu, cpuErr := h.queryPrometheusInstant(r, cpuExpr)
	memBytes, memErr := h.queryPrometheusInstant(r, memExpr)
	restarts, restErr := h.queryPrometheusInstant(r, restartExpr)
	runningPods, podErr := h.queryPrometheusInstant(r, podExpr)
	end := time.Now().UTC()
	start := end.Add(-windowDur)
	cpuSeries, cpuSeriesErr := h.queryPrometheusRange(r, cpuExpr, start, end, step)
	memSeries, memSeriesErr := h.queryPrometheusRange(r, memExpr, start, end, step)
	topCPUExpr := fmt.Sprintf(`topk(5, sum by (pod) (rate(container_cpu_usage_seconds_total{namespace="%s",container!="",container!="POD"}[5m])))`, namespace)
	topMemExpr := fmt.Sprintf(`topk(5, sum by (pod) (container_memory_working_set_bytes{namespace="%s",container!="",container!="POD"}))`, namespace)
	topCPU, topCPUErr := h.queryPrometheusTopVector(r, topCPUExpr, "pod", 1)
	topMem, topMemErr := h.queryPrometheusTopVector(r, topMemExpr, "pod", 1024*1024)

	payload := map[string]any{
		"env":            env,
		"window":         window,
		"namespace":      namespace,
		"cpu_cores_5m":   cpu,
		"memory_bytes":   memBytes,
		"memory_mib":     memBytes / 1024 / 1024,
		"restarts_1h":    restarts,
		"running_pods":   runningPods,
		"grafana_url":    trimURL(h.cfg.GrafanaURL),
		"dashboard_url":  grafanaNamespaceDashboardURL(h.cfg.GrafanaURL, namespace),
		"prometheus_url": trimURL(h.prometheusBaseURL()),
		"cpu_series":     cpuSeries,
		"memory_series":  memSeries,
		"top_cpu_pods":   topCPU,
		"top_mem_pods":   topMem,
	}

	if cpuErr != nil || memErr != nil || restErr != nil || podErr != nil || cpuSeriesErr != nil || memSeriesErr != nil || topCPUErr != nil || topMemErr != nil {
		payload["warning"] = "Một số metric chưa sẵn sàng"
	}
	writeJSON(w, http.StatusOK, payload)
}
