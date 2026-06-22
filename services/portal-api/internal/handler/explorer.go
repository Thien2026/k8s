package handler

import (
	"net/http"

	"github.com/Thien2026/k8s/services/portal-api/internal/rancher"
	"github.com/go-chi/chi/v5"
)

func (h *Handler) ExplorerMenu(w http.ResponseWriter, _ *http.Request) {
	type item struct {
		Key   string `json:"key"`
		Label string `json:"label"`
		Group string `json:"group"`
		Type  string `json:"type"` // k8s | rancher
	}
	menu := []item{
		{Key: "overview", Label: "Tổng quan", Group: "Cluster", Type: "page"},
		{Key: "clusters", Label: "Clusters", Group: "Cluster", Type: "rancher"},
		{Key: "projects", Label: "Projects", Group: "Cluster", Type: "rancher"},
	}
	for _, r := range rancher.K8sResources {
		menu = append(menu, item{Key: r.Key, Label: r.Label, Group: r.Group, Type: "k8s"})
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

func (h *Handler) ListK8sResource(w http.ResponseWriter, r *http.Request) {
	if !h.rancherRequired(w) {
		return
	}
	key := chi.URLParam(r, "resource")
	ns := r.URL.Query().Get("namespace")

	list, err := h.rancher.ListK8s(r.Context(), key, ns)
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
