package handler

import (
	"net/http"
	"strings"

	"github.com/Thien2026/k8s/services/portal-api/internal/auth"
	"github.com/Thien2026/k8s/services/portal-api/internal/plugins"
)

type infraLink struct {
	Key       string `json:"key"`
	Label     string `json:"label"`
	URL       string `json:"url"`
	LoginURL  string `json:"login_url,omitempty"`
	Username  string `json:"username,omitempty"`
	Password  string `json:"password,omitempty"`
	Enabled   bool   `json:"enabled"`
	Note      string `json:"note,omitempty"`
}

func (h *Handler) GetInfraLinks(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	if !auth.CanViewInfra(u.Role) {
		writeAccessDenied(w)
		return
	}

	var items []infraLink

	rancherOn, _ := h.plugins.Enabled(r.Context(), plugins.Rancher)
	if rancherOn && strings.TrimSpace(h.cfg.RancherURL) != "" {
		url := strings.TrimRight(strings.TrimSpace(h.cfg.RancherURL), "/")
		lnk := infraLink{
			Key:      "rancher",
			Label:    "Rancher",
			URL:      url,
			LoginURL: url + "/dashboard/auth/login",
			Username: "admin",
			Password: strings.TrimSpace(h.cfg.RancherAdminPassword),
			Enabled:  h.rancher != nil && h.rancher.Enabled(),
			Note:     "Quản lý cluster Kubernetes",
		}
		items = append(items, lnk)
	}

	harborOn, _ := h.plugins.Enabled(r.Context(), plugins.Harbor)
	if harborOn && strings.TrimSpace(h.cfg.HarborURL) != "" {
		url := strings.TrimRight(strings.TrimSpace(h.cfg.HarborURL), "/")
		user := strings.TrimSpace(h.cfg.HarborAdminUser)
		if user == "" {
			user = "admin"
		}
		lnk := infraLink{
			Key:      "harbor",
			Label:    "Harbor",
			URL:      url,
			LoginURL: url,
			Username: user,
			Password: strings.TrimSpace(h.cfg.HarborPassword),
			Enabled:  h.harbor != nil && h.harbor.Enabled(),
			Note:     "Container registry — push/pull image",
		}
		items = append(items, lnk)
	}

	if h.monitoringConfigured() {
		gURL := trimURL(h.cfg.GrafanaURL)
		user := strings.TrimSpace(h.cfg.GrafanaAdminUser)
		if user == "" {
			user = "admin"
		}
		monOn, _ := h.plugins.Enabled(r.Context(), plugins.Monitoring)
		lnk := infraLink{
			Key:      "grafana",
			Label:    "Grafana",
			URL:      gURL,
			LoginURL: gURL + "/login",
			Username: user,
			Password: strings.TrimSpace(h.cfg.GrafanaAdminPassword),
			Enabled:  monOn || h.monitoringConfigured(),
			Note:     "Metrics & dashboards — Prometheus + Alertmanager",
		}
		items = append(items, lnk)
	}

	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
