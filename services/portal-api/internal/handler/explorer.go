package handler

import (
	"net/http"
	"strconv"

	"github.com/Thien2026/k8s/services/portal-api/internal/rancher"
	"github.com/go-chi/chi/v5"
)

func (h *Handler) ExplorerMenu(w http.ResponseWriter, _ *http.Request) {
	type item struct {
		Key     string `json:"key"`
		Label   string `json:"label"`
		Section string `json:"section"`
		Group   string `json:"group"`
		Type    string `json:"type"`
	}
	menu := []item{
		{Key: "overview", Label: "Cluster Dashboard", Section: "platform", Group: "Platform", Type: "page"},
		{Key: "clusters", Label: "Clusters", Section: "infra", Group: "Cluster", Type: "rancher"},
		{Key: "projects", Label: "Projects", Section: "infra", Group: "Cluster", Type: "rancher"},
	}
	for _, r := range rancher.K8sResources {
		sec := r.Section
		if sec == "" {
			sec = "infra"
		}
		menu = append(menu, item{Key: r.Key, Label: r.Label, Section: sec, Group: r.Group, Type: "k8s"})
	}
	writeJSON(w, http.StatusOK, menu)
}

func (h *Handler) rancherRequired(w http.ResponseWriter) bool {
	if h.rancher == nil || !h.rancher.Enabled() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "Rancher chưa cấu hình — thêm RANCHER_TOKEN vào config/rancher.env",
		})
		return false
	}
	return true
}

func parsePageLimit(r *http.Request) (page, limit int) {
	page, _ = strconv.Atoi(r.URL.Query().Get("page"))
	limit, _ = strconv.Atoi(r.URL.Query().Get("limit"))
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 50
	}
	return page, limit
}

func (h *Handler) ClusterDashboard(w http.ResponseWriter, r *http.Request) {
	if !h.rancherRequired(w) {
		return
	}
	dash, err := h.rancher.ClusterDashboard(r.Context())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, dash)
}

func (h *Handler) ListK8sResource(w http.ResponseWriter, r *http.Request) {
	if !h.rancherRequired(w) {
		return
	}
	key := chi.URLParam(r, "resource")
	ns := r.URL.Query().Get("namespace")
	page, limit := parsePageLimit(r)

	list, err := h.rancher.ListK8s(r.Context(), key, ns, page, limit)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *Handler) ListRancherClusters(w http.ResponseWriter, r *http.Request) {
	if !h.rancherRequired(w) {
		return
	}
	list, err := h.rancher.ListClusters(r.Context())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	if list == nil {
		list = []rancher.ClusterRow{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"total": len(list), "items": list})
}

func (h *Handler) ListRancherProjects(w http.ResponseWriter, r *http.Request) {
	if !h.rancherRequired(w) {
		return
	}
	list, err := h.rancher.ListProjects(r.Context())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	if list == nil {
		list = []rancher.ProjectRow{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"total": len(list), "items": list})
}
