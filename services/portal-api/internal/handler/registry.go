package handler

import (
	"net/http"

	"github.com/Thien2026/k8s/services/portal-api/internal/registry"
)

func (h *Handler) ListRegistryProviders(w http.ResponseWriter, r *http.Request) {
	list, err := h.registry.ListProviders(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list, "default": registry.GHCR})
}
