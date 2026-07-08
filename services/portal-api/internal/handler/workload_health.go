package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Thien2026/k8s/services/portal-api/internal/auth"
	"github.com/Thien2026/k8s/services/portal-api/internal/rancher"
	"github.com/go-chi/chi/v5"
)

type workloadHealthSummary struct {
	Overall       string `json:"overall"`
	TotalRestarts int    `json:"total_restarts"`
	PodsRunning   int    `json:"pods_running"`
	PodsUnhealthy int    `json:"pods_unhealthy"`
	PodsCrashLoop int    `json:"pods_crash_loop"`
	PodsOOM       int    `json:"pods_oom"`
}

type workloadPodView struct {
	Name                  string `json:"name"`
	Status                string `json:"status"`
	Health                string `json:"health"`
	Restarts              int    `json:"restarts"`
	LastTerminationReason string `json:"last_termination_reason,omitempty"`
	Ready                 bool   `json:"ready"`
	Node                  string `json:"node,omitempty"`
	Images                string `json:"images,omitempty"`
}

type workloadDeployView struct {
	Name     string `json:"name"`
	Replicas string `json:"replicas"`
	Status   string `json:"status"`
	Health   string `json:"health"`
}

type workloadHealthResponse struct {
	Environment string                `json:"environment"`
	Namespace   string                `json:"namespace"`
	Summary     workloadHealthSummary `json:"summary"`
	Deployments []workloadDeployView  `json:"deployments"`
	Pods        []workloadPodView     `json:"pods"`
	Events      []string              `json:"events"`
	CanRestart  bool                  `json:"can_restart"`
}

func classifyPodHealth(pod rancher.ResourceRow) string {
	st := strings.ToLower(strings.TrimSpace(pod.Status))
	term := strings.ToLower(strings.TrimSpace(pod.LastTerminationReason))

	if strings.Contains(term, "oom") || st == "oomkilled" {
		return "oom_killed"
	}
	if strings.Contains(st, "crashloop") || strings.Contains(st, "backoff") {
		return "crash_loop"
	}
	if strings.Contains(st, "error") || strings.Contains(st, "imagepull") || st == "failed" {
		return "failed"
	}
	if st == "pending" || st == "containercreating" {
		return "pending"
	}
	if st == "running" {
		if !pod.Ready {
			if pod.Restarts > 0 {
				return "restarting"
			}
			return "degraded"
		}
		return "running"
	}
	if pod.Restarts > 0 && !pod.Ready {
		return "restarting"
	}
	return "unknown"
}

func isPodHealthUnhealthy(health string) bool {
	switch health {
	case "running":
		return false
	default:
		return true
	}
}

func mergeOverallHealth(current, next string) string {
	rank := map[string]int{
		"healthy": 0, "unknown": 1, "degraded": 2, "restarting": 3,
		"pending": 4, "failed": 5, "crash_loop": 6, "oom_killed": 6,
	}
	if rank[next] > rank[current] {
		return next
	}
	return current
}

func summarizeWorkloadHealth(pods []workloadPodView) workloadHealthSummary {
	sum := workloadHealthSummary{Overall: "healthy"}
	for _, pod := range pods {
		sum.TotalRestarts += pod.Restarts
		switch pod.Health {
		case "running":
			sum.PodsRunning++
		case "crash_loop":
			sum.PodsCrashLoop++
			sum.PodsUnhealthy++
			sum.Overall = mergeOverallHealth(sum.Overall, "crash_loop")
		case "oom_killed":
			sum.PodsOOM++
			sum.PodsUnhealthy++
			sum.Overall = mergeOverallHealth(sum.Overall, "oom_killed")
		case "restarting", "degraded", "failed", "pending", "unknown":
			if pod.Health != "unknown" {
				sum.PodsUnhealthy++
			}
			sum.Overall = mergeOverallHealth(sum.Overall, pod.Health)
		}
	}
	if len(pods) == 0 {
		sum.Overall = "unknown"
	}
	return sum
}

func classifyDeploymentHealth(dep rancher.ResourceRow, pods []workloadPodView) string {
	health := "running"
	for _, pod := range pods {
		if pod.Health == "crash_loop" || pod.Health == "oom_killed" || pod.Health == "failed" {
			return pod.Health
		}
		if isPodHealthUnhealthy(pod.Health) {
			health = pod.Health
		}
	}
	st := strings.ToLower(strings.TrimSpace(dep.Status))
	if strings.Contains(st, "progress") || strings.Contains(st, "unavailable") {
		return mergeOverallHealth(health, "restarting")
	}
	if health == "running" && st != "" && st != "active" && !strings.Contains(st, "available") {
		return "degraded"
	}
	return health
}

func (h *Handler) projectWorkloadNamespace(p projectRow, env string) string {
	return h.projectNamespace(p, env)
}

func (h *Handler) canRestartProjectWorkload(r *http.Request, p projectRow, env string) bool {
	ns := h.projectWorkloadNamespace(p, env)
	if ns == "" {
		return false
	}
	u, ok := auth.UserFromContext(r.Context())
	if !ok || !auth.CanWriteK8s(u.Role) {
		return false
	}
	scope, err := h.accessScope(r.Context(), u)
	if err != nil {
		return false
	}
	return namespaceWritable(scope, u, ns)
}

func (h *Handler) GetProjectWorkloadHealth(w http.ResponseWriter, r *http.Request) {
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
	if env != "dev" && env != "prod" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "environment phải là dev hoặc prod"})
		return
	}
	ns := h.projectWorkloadNamespace(p, env)
	if ns == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "namespace chưa cấu hình"})
		return
	}
	if _, ok := h.guardK8sRead(w, r, "pods", ns); !ok {
		return
	}

	clusterID := clusterQuery(r)
	podsList, err := h.rancher.ListK8s(r.Context(), clusterID, "pods", ns, 1, 100)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	depsList, _ := h.rancher.ListK8s(r.Context(), clusterID, "deployments", ns, 1, 100)

	podViews := make([]workloadPodView, 0, len(podsList.Items))
	for _, pod := range podsList.Items {
		podViews = append(podViews, workloadPodView{
			Name:                  pod.Name,
			Status:                pod.Status,
			Health:                classifyPodHealth(pod),
			Restarts:              pod.Restarts,
			LastTerminationReason: pod.LastTerminationReason,
			Ready:                 pod.Ready,
			Node:                  pod.Node,
			Images:                pod.Images,
		})
	}

	depViews := make([]workloadDeployView, 0, len(depsList.Items))
	for _, dep := range depsList.Items {
		depViews = append(depViews, workloadDeployView{
			Name:     dep.Name,
			Replicas: dep.Replicas,
			Status:   dep.Status,
			Health:   classifyDeploymentHealth(dep, podViews),
		})
	}

	events, _ := h.rancher.ListNamespaceEvents(r.Context(), clusterID, ns, "", 80)
	eventLines := buildWorkloadEventLines(events)

	writeJSON(w, http.StatusOK, workloadHealthResponse{
		Environment: env,
		Namespace:   ns,
		Summary:     summarizeWorkloadHealth(podViews),
		Deployments: depViews,
		Pods:        podViews,
		Events:      eventLines,
		CanRestart:  h.canRestartProjectWorkload(r, p, env),
	})
}

func buildWorkloadEventLines(events []rancher.ResourceRow) []string {
	if len(events) == 0 {
		return nil
	}
	lines := make([]string, 0, 8)
	for _, ev := range events {
		if !isInterestingK8sEvent(ev) {
			continue
		}
		line := strings.TrimSpace(ev.Reason)
		if ev.Object != "" {
			line = ev.Object + ": " + line
		}
		if ev.Message != "" {
			line += " — " + ev.Message
		}
		if line == "" {
			continue
		}
		lines = append(lines, line)
		if len(lines) >= 8 {
			break
		}
	}
	return lines
}

func (h *Handler) RestartProjectWorkload(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	p, ok := h.requireProjectAccess(w, r, slug)
	if !ok {
		return
	}
	if !h.rancherRequired(w, r) {
		return
	}

	var body struct {
		Environment string `json:"environment"`
		Deployment  string `json:"deployment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	env := strings.TrimSpace(body.Environment)
	if env == "" {
		env = "dev"
	}
	if env != "dev" && env != "prod" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "environment phải là dev hoặc prod"})
		return
	}
	deployName := strings.TrimSpace(body.Deployment)
	if deployName == "" {
		deployName = "app"
	}

	ns := h.projectWorkloadNamespace(p, env)
	if ns == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "namespace chưa cấu hình"})
		return
	}
	if _, ok := h.guardK8sRead(w, r, "deployments", ns); !ok {
		return
	}
	if _, ok := h.guardK8sWrite(w, r, ns); !ok {
		return
	}

	clusterID := clusterQuery(r)
	if err := h.rancher.RolloutRestartDeployment(r.Context(), clusterID, ns, deployName); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	u, _ := auth.UserFromContext(r.Context())
	auditAction(r.Context(), h, r, "workload.restart", slug, map[string]any{
		"environment": env,
		"namespace":   ns,
		"deployment":  deployName,
		"by":          u.Email,
	})
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "Đang restart deployment " + deployName + " — pod mới sẽ lên trong vài giây",
	})
}
