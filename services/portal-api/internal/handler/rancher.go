package handler

import "net/http"

func (h *Handler) ClusterSummary(w http.ResponseWriter, r *http.Request) {
	if h.rancher == nil || !h.rancher.Enabled() {
		writeJSON(w, http.StatusOK, map[string]any{
			"connected": false,
			"message":   "Rancher chưa cấu hình — thêm RANCHER_TOKEN vào config/rancher.env rồi redeploy portal-api",
		})
		return
	}

	sum, err := h.rancher.ClusterSummary(r.Context())
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"connected": false,
			"error":     err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"connected": sum.Connected,
		"total":     sum.Total,
		"ready":     sum.Ready,
		"not_ready": sum.NotReady,
		"nodes":     sum.Nodes,
	})
}
