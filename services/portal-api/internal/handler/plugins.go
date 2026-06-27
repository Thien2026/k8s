package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Thien2026/k8s/services/portal-api/internal/auth"
	"github.com/Thien2026/k8s/services/portal-api/internal/plugins"
	"github.com/go-chi/chi/v5"
)

func (h *Handler) pluginEnabled(ctx context.Context, name string) bool {
	ok, err := h.plugins.Enabled(ctx, name)
	return err == nil && ok
}

func (h *Handler) rancherPluginRequired(w http.ResponseWriter, r *http.Request) bool {
	return h.rancherRequired(w, r)
}

func (h *Handler) harborPluginEnabled(ctx context.Context) bool {
	return h.pluginEnabled(ctx, plugins.Harbor)
}

func (h *Handler) enrichPluginStatus(r *http.Request, list []plugins.Plugin) []plugins.Plugin {
	installMeta := map[string]struct {
		chart   string
		script  string
		prereq  string
	}{
		plugins.Rancher: {
			chart:  "2.14.2",
			script: "bootstrap/addons/install-rancher.sh",
			prereq: "Core bootstrap xong — ./bootstrap/run.sh đến bước 08",
		},
		plugins.Harbor: {
			chart:  "1.19.1",
			script: "bootstrap/addons/install-harbor.sh",
			prereq: "Core 08 xong — Ingress + cert-manager OK",
		},
	}

	for i := range list {
		switch list[i].Name {
		case plugins.Rancher:
			if h.rancher != nil && h.rancher.Enabled() {
				list[i].Ready = true
				list[i].ReadyHint = "RANCHER_TOKEN đã cấu hình"
			} else if list[i].Enabled {
				list[i].ReadyHint = "Đã bật — chạy lệnh cài bên dưới trên VPS (tmux)"
			} else {
				list[i].ReadyHint = "Bật addon → chạy script cài trên VPS"
			}
		case plugins.Harbor:
			if h.harbor != nil && h.harbor.Enabled() {
				list[i].Ready = true
				list[i].ReadyHint = "HARBOR_URL + admin password OK"
			} else if list[i].Enabled {
				list[i].ReadyHint = "Đã bật — chạy lệnh cài bên dưới trên VPS (tmux)"
			} else {
				list[i].ReadyHint = "Tùy chọn — GHCR mặc định nếu không cần on-prem"
			}
		case plugins.GHCR:
			list[i].Ready = list[i].Enabled
			list[i].ReadyHint = "Sẵn sàng — cấu hình GitHub Actions push ghcr.io"
		case plugins.Console:
			list[i].Ready = true
			list[i].ReadyHint = "Core luôn bật"
		}

		if meta, ok := installMeta[list[i].Name]; ok && !list[i].Core {
			list[i].ChartVersion = meta.chart
			list[i].PrereqNote = meta.prereq
			list[i].CheckCmd = "./bootstrap/addons/run.sh check " + list[i].Name
			list[i].InstallCmd = "tmux new -A -s k8s-addon 'cd ~/k8s && ./bootstrap/addons/run.sh " + list[i].Name + "'"
			list[i].NeedsBootstrap = list[i].Enabled && !list[i].Ready
		}
	}
	return list
}

func (h *Handler) AdminListPlugins(w http.ResponseWriter, r *http.Request) {
	list, err := h.plugins.List(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	list = h.enrichPluginStatus(r, list)
	writeJSON(w, http.StatusOK, map[string]any{"items": list})
}

func (h *Handler) AdminPatchPlugin(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	var body struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Enabled == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "enabled (bool) bắt buộc"})
		return
	}
	if err := h.plugins.SetEnabled(r.Context(), name, *body.Enabled); err != nil {
		if errors.Is(err, plugins.ErrCannotDisableCore) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "plugin không tồn tại"})
		return
	}
	actor := auth.MustUser(r.Context())
	auditAction(r.Context(), h, r, "plugin.update", name, map[string]any{"enabled": *body.Enabled, "by": actor.Email})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
