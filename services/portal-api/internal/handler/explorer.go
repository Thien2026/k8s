package handler

import (
	"net/http"
	"strconv"

	"github.com/Thien2026/k8s/services/portal-api/internal/auth"
	"github.com/Thien2026/k8s/services/portal-api/internal/plugins"
	"github.com/Thien2026/k8s/services/portal-api/internal/rancher"
	"github.com/go-chi/chi/v5"
)

func (h *Handler) ExplorerMenu(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	type item struct {
		Key     string `json:"key"`
		Label   string `json:"label"`
		Section string `json:"section"`
		Group   string `json:"group"`
		Type    string `json:"type"`
	}
	var menu []item
	if auth.CanViewInfra(u.Role) {
		menu = append(menu, item{Key: "overview", Label: "Cluster Dashboard", Section: "platform", Group: "Platform", Type: "page"})
	} else {
		menu = append(menu, item{Key: "my-projects", Label: "Dự án của tôi", Section: "platform", Group: "Platform", Type: "page"})
	}
	if auth.CanViewAudit(u.Role) {
		menu = append(menu, item{Key: "audit", Label: "Audit Log", Section: "platform", Group: "Platform", Type: "page"})
	}
	if auth.CanManageUsers(u.Role) {
		menu = append(menu, item{Key: "users", Label: "Quản lý user", Section: "platform", Group: "Platform", Type: "page"})
	}
	if auth.CanViewAllProjects(u.Role) {
		menu = append(menu, item{Key: "platform-projects", Label: "Quản lý Projects", Section: "platform", Group: "Platform", Type: "page"})
	}
	if u.Role == auth.RoleAdmin || u.Role == auth.RoleTechLead {
		menu = append(menu, item{Key: "addons", Label: "Addons", Section: "platform", Group: "Platform", Type: "page"})
	}
	if u.Role == auth.RoleAdmin {
		menu = append(menu,
			item{Key: "policy", Label: "Platform Policy", Section: "platform", Group: "Platform", Type: "page"},
			item{Key: "gitops", Label: "GitOps", Section: "platform", Group: "Platform", Type: "page"},
		)
	}

	rancherOn, _ := h.plugins.Enabled(r.Context(), plugins.Rancher)
	if auth.CanViewInfra(u.Role) && rancherOn {
		menu = append(menu, item{
			Key: "k8s-ops", Label: "Sổ lệnh K8s", Section: "infra", Group: "Vận hành", Type: "page",
		})
		menu = append(menu,
			item{Key: "clusters", Label: "Clusters", Section: "infra", Group: "Cluster", Type: "rancher"},
			item{Key: "projects", Label: "Projects", Section: "infra", Group: "Cluster", Type: "rancher"},
		)
		if auth.CanJoinWorker(u.Role) {
			menu = append(menu, item{Key: "add-worker", Label: "Thêm worker", Section: "infra", Group: "Cluster", Type: "page"})
		}
		for _, res := range rancher.K8sResources {
			sec := res.Section
			if sec == "" {
				sec = "infra"
			}
			menu = append(menu, item{Key: res.Key, Label: res.Label, Section: sec, Group: res.Group, Type: "k8s"})
		}
	} else if rancherOn {
		menu = append(menu, item{
			Key: "k8s-ops", Label: "Sổ lệnh K8s", Section: "workspace", Group: "Vận hành", Type: "page",
		})
		for _, key := range devK8sKeys {
			res, ok := rancher.K8sResourceByKey(key)
			if !ok {
				continue
			}
			menu = append(menu, item{
				Key: key, Label: res.Label, Section: "workspace", Group: res.Group, Type: "k8s",
			})
		}
	}
	writeJSON(w, http.StatusOK, menu)
}

func (h *Handler) rancherRequired(w http.ResponseWriter, r *http.Request) bool {
	if !h.pluginEnabled(r.Context(), plugins.Rancher) {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "Addon Rancher chưa bật — vào Addons hoặc chạy bootstrap/addons/install-rancher.sh",
		})
		return false
	}
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
	if !h.rancherRequired(w, r) {
		return
	}
	u, _ := auth.UserFromContext(r.Context())
	if !auth.CanViewInfra(u.Role) {
		writeAccessDenied(w)
		return
	}
	dash, err := h.rancher.ClusterDashboard(r.Context(), r.URL.Query().Get("cluster_id"))
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, dash)
}

func (h *Handler) ListK8sResource(w http.ResponseWriter, r *http.Request) {
	if !h.rancherRequired(w, r) {
		return
	}
	key := chi.URLParam(r, "resource")
	ns := r.URL.Query().Get("namespace")
	page, limit := parsePageLimit(r)

	scope, ok := h.guardK8sRead(w, r, key, ns)
	if !ok {
		return
	}

	list, err := h.rancher.ListK8s(r.Context(), r.URL.Query().Get("cluster_id"), key, ns, page, limit)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	if !scope.All {
		list = filterResourceList(scope, list)
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *Handler) ListRancherClusters(w http.ResponseWriter, r *http.Request) {
	if !h.rancherRequired(w, r) {
		return
	}
	u, _ := auth.UserFromContext(r.Context())
	if !auth.CanViewInfra(u.Role) {
		writeAccessDenied(w)
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
	if !h.rancherRequired(w, r) {
		return
	}
	u, _ := auth.UserFromContext(r.Context())
	if !auth.CanViewInfra(u.Role) {
		writeAccessDenied(w)
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
