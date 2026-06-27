package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/Thien2026/k8s/services/portal-api/internal/rancher"
	"github.com/go-chi/chi/v5"
)

func (h *Handler) JoinInfo(w http.ResponseWriter, r *http.Request) {
	info := rancher.BuildJoinInfo(h.joinConfig())
	if h.rancher != nil && h.rancher.Enabled() {
		cid := r.URL.Query().Get("cluster_id")
		info.NodeCount = h.rancher.JoinNodeCount(r.Context(), cid)
	}
	writeJSON(w, http.StatusOK, info)
}

func (h *Handler) JoinScript(w http.ResponseWriter, r *http.Request) {
	if h.cfg.JoinGateSecret == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "JOIN_GATE_SECRET chưa cấu hình — chạy bootstrap 01b + 08",
		})
		return
	}
	gate := r.Header.Get("X-Join-Gate")
	if gate == "" {
		var body struct {
			Gate string `json:"gate"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gate = body.Gate
	}
	if gate != h.cfg.JoinGateSecret {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": "PIN join không đúng",
		})
		return
	}
	jc := h.joinConfig()
	resp, err := jc.JoinScript("")
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) joinConfig() rancher.JoinConfig {
	return rancher.JoinConfig{
		ServerIP:    h.cfg.RKE2ServerIP,
		ServerURL:   h.cfg.RKE2ServerURL,
		ServerToken: h.cfg.RKE2ServerToken,
		GateSecret:  h.cfg.JoinGateSecret,
	}
}

func clusterQuery(r *http.Request) string {
	return r.URL.Query().Get("cluster_id")
}

func (h *Handler) GetK8sDetail(w http.ResponseWriter, r *http.Request) {
	if !h.rancherRequired(w, r) {
		return
	}
	key := chi.URLParam(r, "resource")
	name := chi.URLParam(r, "name")
	ns := r.URL.Query().Get("namespace")

	if _, ok := h.guardK8sRead(w, r, key, ns); !ok {
		return
	}

	raw, err := h.rancher.GetK8sResource(r.Context(), clusterQuery(r), key, ns, name)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
}

func (h *Handler) GetK8sYAML(w http.ResponseWriter, r *http.Request) {
	if !h.rancherRequired(w, r) {
		return
	}
	key := chi.URLParam(r, "resource")
	name := chi.URLParam(r, "name")
	ns := r.URL.Query().Get("namespace")

	if _, ok := h.guardK8sRead(w, r, key, ns); !ok {
		return
	}

	yaml, err := h.rancher.GetK8sYAML(r.Context(), clusterQuery(r), key, ns, name)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"yaml": yaml})
}

func (h *Handler) GetPodLogs(w http.ResponseWriter, r *http.Request) {
	if !h.rancherRequired(w, r) {
		return
	}
	name := chi.URLParam(r, "name")
	ns := r.URL.Query().Get("namespace")
	container := r.URL.Query().Get("container")
	tail, _ := strconv.Atoi(r.URL.Query().Get("tail"))

	if _, ok := h.guardK8sRead(w, r, "pods", ns); !ok {
		return
	}

	logs, err := h.rancher.GetPodLogs(r.Context(), clusterQuery(r), ns, name, container, tail)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"logs": logs})
}

func (h *Handler) DeleteK8sResource(w http.ResponseWriter, r *http.Request) {
	if !h.rancherRequired(w, r) {
		return
	}
	key := chi.URLParam(r, "resource")
	name := chi.URLParam(r, "name")
	ns := r.URL.Query().Get("namespace")

	if _, ok := h.guardK8sRead(w, r, key, ns); !ok {
		return
	}
	if _, ok := h.guardK8sWrite(w, r, ns); !ok {
		return
	}

	if err := h.rancher.DeleteK8sResource(r.Context(), clusterQuery(r), key, ns, name); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handler) ScaleDeployment(w http.ResponseWriter, r *http.Request) {
	if !h.rancherRequired(w, r) {
		return
	}
	name := chi.URLParam(r, "name")
	ns := r.URL.Query().Get("namespace")
	var body struct {
		Replicas int `json:"replicas"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err != io.EOF {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	if body.Replicas < 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "replicas must be >= 0"})
		return
	}
	if _, ok := h.guardK8sRead(w, r, "deployments", ns); !ok {
		return
	}
	if _, ok := h.guardK8sWrite(w, r, ns); !ok {
		return
	}
	if err := h.rancher.ScaleDeployment(r.Context(), clusterQuery(r), ns, name, body.Replicas); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "scaled"})
}

func (h *Handler) ListNamespaces(w http.ResponseWriter, r *http.Request) {
	if !h.rancherRequired(w, r) {
		return
	}
	scope, err := h.accessScopeFromRequest(r)
	if err != nil {
		writeAccessDenied(w)
		return
	}
	if scope.All {
		list, err := h.rancher.ListK8s(r.Context(), clusterQuery(r), "namespaces", "", 1, 500)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		names := make([]string, 0, len(list.Items))
		for _, item := range list.Items {
			names = append(names, item.Name)
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": names})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": scope.ReadNamespaces})
}
