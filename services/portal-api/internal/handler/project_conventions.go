package handler

import (
	"context"
	"fmt"
	"net/http"

	"github.com/Thien2026/k8s/services/portal-api/internal/auth"
	"github.com/Thien2026/k8s/services/portal-api/internal/deploy"
	"github.com/go-chi/chi/v5"
)

type backFrontConventionSeed struct {
	Key         string `json:"key"`
	Value       string `json:"value"`
	Scope       string `json:"scope"`
	Environment string `json:"environment"`
	Created     bool   `json:"created"`
}

func (h *Handler) backFrontConventionsPayload(ctx context.Context, projectID int64) map[string]any {
	layout := h.getProjectLayout(ctx, projectID)
	if layout != deploy.LayoutMulti {
		return map[string]any{
			"layout":  layout,
			"enabled": false,
			"summary": "Chuẩn back/front chỉ áp dụng khi layout Backend + Frontend.",
		}
	}
	return map[string]any{
		"layout":          layout,
		"enabled":         true,
		"api_base_path":   deploy.ConventionAPIBasePath,
		"public_pattern":  "https://{domain}" + deploy.ConventionAPIBasePath + "/...",
		"dev_local_hint":  "Dev máy: frontend proxy /api → backend — không hardcode port trong code prod",
		"build_contract":  deploy.DefaultMultiBuildContract(),
		"runtime_contract": deploy.DefaultMultiRuntimeContract(),
		"recommended_build": map[string]string{
			deploy.ConventionViteAPIBaseKey: deploy.ConventionAPIBasePath,
		},
		"recommended_runtime": map[string]string{
			deploy.ConventionAPIRoutePrefixKey: deploy.ConventionAPIBasePath,
		},
	}
}

func (h *Handler) ensureBackFrontConventions(ctx context.Context, projectID int64) ([]backFrontConventionSeed, error) {
	if h.getProjectLayout(ctx, projectID) != deploy.LayoutMulti {
		return nil, nil
	}
	var seeds []backFrontConventionSeed
	for key, val := range deploy.DefaultBuildEnvSeed() {
		created, err := h.upsertProjectEnvVar(ctx, projectID, "dev", "build", key, val, false)
		if err != nil {
			return seeds, fmt.Errorf("seed build %s: %w", key, err)
		}
		seeds = append(seeds, backFrontConventionSeed{
			Key: key, Value: val, Scope: "build", Environment: "dev", Created: created,
		})
	}
	for key, val := range deploy.DefaultRuntimeEnvSeed() {
		for _, env := range []string{"dev", "prod"} {
			created, err := h.upsertProjectEnvVar(ctx, projectID, env, "runtime", key, val, false)
			if err != nil {
				return seeds, fmt.Errorf("seed runtime %s/%s: %w", env, key, err)
			}
			seeds = append(seeds, backFrontConventionSeed{
				Key: key, Value: val, Scope: "runtime", Environment: env, Created: created,
			})
		}
	}
	return seeds, nil
}

// upsertProjectEnvVar — chỉ insert nếu key chưa tồn tại (không ghi đè dev đã cấu hình).
func (h *Handler) upsertProjectEnvVar(ctx context.Context, projectID int64, env, scope, key, value string, isSecret bool) (created bool, err error) {
	var exists bool
	err = h.db.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM project_env_vars
			WHERE project_id=$1 AND environment=$2 AND scope=$3 AND key=$4
		)`, projectID, env, scope, key).Scan(&exists)
	if err != nil {
		return false, err
	}
	if exists {
		return false, nil
	}
	_, err = h.db.Exec(ctx, `
		INSERT INTO project_env_vars (project_id, environment, key, value, is_secret, scope)
		VALUES ($1,$2,$3,$4,$5,$6)`,
		projectID, env, key, value, isSecret, scope)
	return err == nil, err
}

func (h *Handler) smokePathsForProject(ctx context.Context, projectID int64, repo projectRepoRow) []string {
	layout := h.getProjectLayout(ctx, projectID)
	if layout != deploy.LayoutMulti {
		return deploy.SmokeCheckPaths(deploy.LayoutSingle, nil)
	}
	services, _ := h.loadDeployServices(ctx, projectID, repo)
	return deploy.SmokeCheckPaths(layout, services)
}

func (h *Handler) GetProjectConventions(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	p, ok := h.requireProjectAccess(w, r, slug)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, h.backFrontConventionsPayload(r.Context(), p.ID))
}

func (h *Handler) ApplyProjectConventions(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	p, ok := h.requireProjectAccess(w, r, slug)
	if !ok {
		return
	}
	u, _ := auth.UserFromContext(r.Context())
	if !auth.CanWriteK8s(u.Role) {
		writeAccessDenied(w)
		return
	}
	if h.getProjectLayout(r.Context(), p.ID) != deploy.LayoutMulti {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Chỉ áp dụng khi project ở layout Backend + Frontend",
		})
		return
	}
	seeds, err := h.ensureBackFrontConventions(r.Context(), p.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"seeds":  seeds,
		"hint":   "Kiểm tra tab Cấu hình app — sync workflow GitHub nếu vừa thêm biến build.",
	})
}
