package handler

import (
	"net/http"

	"github.com/Thien2026/k8s/services/portal-api/internal/deploy"
	"github.com/go-chi/chi/v5"
)

// GetProjectEnvSyncStatus GET /projects/{slug}/env/sync-status?environment=dev
func (h *Handler) GetProjectEnvSyncStatus(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	p, ok := h.requireProjectAccess(w, r, slug)
	if !ok {
		return
	}
	env, ok := validProjectEnv(r.URL.Query().Get("environment"))
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "environment phải là dev hoặc prod"})
		return
	}
	cid := r.URL.Query().Get("cluster_id")
	ns := h.projectNamespace(p, env)

	console, err := h.envVarsMap(r.Context(), p.ID, env)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	resp := map[string]any{
		"environment":   env,
		"namespace":     ns,
		"console_keys":  len(console),
		"synced":        false,
		"cluster_found": false,
	}

	if h.rancher == nil || !h.rancher.Enabled() {
		resp["detail"] = "Rancher chưa sẵn sàng — không so được với cluster"
		writeJSON(w, http.StatusOK, resp)
		return
	}

	cluster, found, err := h.rancher.GetOpaqueSecretData(r.Context(), cid, ns, deploy.AppEnvSecretName)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	resp["cluster_found"] = found

	if len(console) == 0 && !found {
		resp["synced"] = true
		resp["detail"] = "Không có biến runtime — cluster cũng không có secret"
		writeJSON(w, http.StatusOK, resp)
		return
	}
	if len(console) > 0 && !found {
		resp["detail"] = "Console có biến nhưng cluster chưa có Secret app-env — bấm Đồng bộ cluster"
		writeJSON(w, http.StatusOK, resp)
		return
	}

	var missingOnCluster, extraOnCluster, valueMismatch []string
	for k, v := range console {
		cv, ok := cluster[k]
		if !ok {
			missingOnCluster = append(missingOnCluster, k)
			continue
		}
		if cv != v {
			valueMismatch = append(valueMismatch, k)
		}
	}
	for k := range cluster {
		if _, ok := console[k]; !ok {
			extraOnCluster = append(extraOnCluster, k)
		}
	}

	resp["missing_on_cluster"] = missingOnCluster
	resp["extra_on_cluster"] = extraOnCluster
	resp["value_mismatch"] = valueMismatch
	resp["synced"] = len(missingOnCluster) == 0 && len(valueMismatch) == 0 && len(extraOnCluster) == 0

	if resp["synced"].(bool) {
		resp["detail"] = "Secret app-env khớp Console"
	} else if len(missingOnCluster) > 0 || len(valueMismatch) > 0 {
		resp["detail"] = "Cluster chưa khớp Console — bấm Đồng bộ cluster & restart pod"
	} else {
		resp["detail"] = "Cluster có key thừa so với Console"
	}
	writeJSON(w, http.StatusOK, resp)
}
