package handler

import (
	"encoding/json"
	"net/http"
	"strings"
)

func (h *Handler) DeployEventHook(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.Header.Get("X-Platform-Deploy-Token"))
	if token == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "thiếu deploy token"})
		return
	}
	var body struct {
		Event       string `json:"event"`
		ImageTag    string `json:"image_tag"`
		Environment string `json:"environment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "payload không hợp lệ"})
		return
	}
	p, err := h.getProjectByDeployToken(r.Context(), token)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "token không hợp lệ"})
		return
	}
	env := strings.ToLower(strings.TrimSpace(body.Environment))
	if env == "" {
		env = "dev"
	}
	tag := strings.TrimSpace(body.ImageTag)
	switch strings.TrimSpace(body.Event) {
	case "build_started":
		h.markDeploymentBuildStarted(r.Context(), p.ID, env, tag)
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "event không hỗ trợ"})
	}
}
