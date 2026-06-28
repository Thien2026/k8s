package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/Thien2026/k8s/services/portal-api/internal/auth"
	"github.com/Thien2026/k8s/services/portal-api/internal/deploy"
	"github.com/go-chi/chi/v5"
)

type projectServiceRow struct {
	Name           string `json:"name"`
	DisplayName    string `json:"display_name,omitempty"`
	BuildContext   string `json:"build_context"`
	BuildMode      string `json:"build_mode"`
	DockerfilePath string `json:"dockerfile_path"`
	ContainerPort  int    `json:"container_port"`
	HealthPath     string `json:"health_path"`
	IngressPath    string `json:"ingress_path"`
	ExposeIngress  *bool  `json:"expose_ingress,omitempty"`
	SortOrder      int    `json:"sort_order"`
}

func serviceRowExposeIngress(r projectServiceRow) bool {
	if r.ExposeIngress != nil {
		return *r.ExposeIngress
	}
	return !deploy.IsInternalIngressMarker(r.IngressPath)
}

func (h *Handler) getProjectLayout(ctx context.Context, projectID int64) string {
	var layout string
	_ = h.db.QueryRow(ctx, `SELECT COALESCE(layout,'single') FROM projects WHERE id=$1`, projectID).Scan(&layout)
	return deploy.NormalizeLayout(layout)
}

func (h *Handler) listProjectServices(ctx context.Context, projectID int64) ([]projectServiceRow, error) {
	rows, err := h.db.Query(ctx, `
		SELECT name, COALESCE(display_name,''), build_context, COALESCE(build_mode,'dockerfile'),
		       dockerfile_path, container_port, health_path, ingress_path, expose_ingress, sort_order
		FROM project_services
		WHERE project_id=$1 AND enabled=true
		ORDER BY sort_order, name`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []projectServiceRow
	for rows.Next() {
		var s projectServiceRow
		var expose bool
		if err := rows.Scan(&s.Name, &s.DisplayName, &s.BuildContext, &s.BuildMode,
			&s.DockerfilePath, &s.ContainerPort, &s.HealthPath, &s.IngressPath, &expose, &s.SortOrder); err != nil {
			return nil, err
		}
		s.ExposeIngress = &expose
		list = append(list, s)
	}
	if list == nil {
		list = []projectServiceRow{}
	}
	return list, rows.Err()
}

func (h *Handler) loadDeployServices(ctx context.Context, projectID int64, repo projectRepoRow) ([]deploy.ServiceDef, string) {
	layout := h.getProjectLayout(ctx, projectID)
	if layout != deploy.LayoutMulti {
		return nil, deploy.LayoutSingle
	}
	rows, err := h.listProjectServices(ctx, projectID)
	if err != nil || len(rows) == 0 {
		return nil, deploy.LayoutSingle
	}
	out := make([]deploy.ServiceDef, 0, len(rows))
	for _, r := range rows {
		out = append(out, deploy.ServiceDef{
			Name:           r.Name,
			DisplayName:    r.DisplayName,
			BuildContext:   r.BuildContext,
			BuildMode:      r.BuildMode,
			DockerfilePath: r.DockerfilePath,
			ContainerPort:  r.ContainerPort,
			HealthPath:     r.HealthPath,
			IngressPath:    r.IngressPath,
			ExposeIngress:  serviceRowExposeIngress(r),
			SortOrder:      r.SortOrder,
		})
	}
	return out, deploy.LayoutMulti
}

func (h *Handler) ingressRoutesForProject(ctx context.Context, projectID int64, repo projectRepoRow) []deploy.IngressRoute {
	services, layout := h.loadDeployServices(ctx, projectID, repo)
	if layout != deploy.LayoutMulti || len(services) == 0 {
		return nil
	}
	return deploy.IngressRoutesFromServices(services)
}

func (h *Handler) GetProjectServices(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	p, ok := h.requireProjectAccess(w, r, slug)
	if !ok {
		return
	}
	repo, _ := h.getProjectRepo(r.Context(), p.ID)
	layout := h.getProjectLayout(r.Context(), p.ID)
	items, err := h.listProjectServices(r.Context(), p.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if layout == deploy.LayoutSingle {
		items = []projectServiceRow{{
			Name:           "app",
			DisplayName:    "App",
			BuildContext:   repo.BuildContext,
			BuildMode:      repo.BuildMode,
			DockerfilePath: repo.DockerfilePath,
			ContainerPort:  8080,
			HealthPath:     "/health",
			IngressPath:    "/",
		}}
	}
	u, _ := auth.UserFromContext(r.Context())
	contract := h.detectServicesContract(r.Context(), u.ID, p, repo, repo.Branch)
	writeJSON(w, http.StatusOK, map[string]any{
		"layout":        layout,
		"items":         items,
		"template":      deploy.DefaultMultiServices,
		"conventions":   h.backFrontConventionsPayload(r.Context(), p.ID),
		"repo_contract": contract,
	})
}

func (h *Handler) PutProjectServices(w http.ResponseWriter, r *http.Request) {
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
	var body struct {
		Layout   string              `json:"layout"`
		Services []projectServiceRow `json:"services"`
		Branch   string              `json:"branch,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "payload không hợp lệ"})
		return
	}
	layout := deploy.NormalizeLayout(body.Layout)
	if layout == deploy.LayoutMulti {
		if len(body.Services) < 2 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Multi-service cần ít nhất 2 service (vd. api + web)"})
			return
		}
		names := map[string]bool{}
		publicCount := 0
		for _, s := range body.Services {
			name := strings.TrimSpace(s.Name)
			if name == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Tên service không được rỗng"})
				return
			}
			if names[name] {
			 writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Tên service trùng: " + name})
				return
			}
			names[name] = true
			if serviceRowExposeIngress(s) {
				publicCount++
			}
		}
		if publicCount == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Cần ít nhất 1 service public (Ingress) — internal-only dùng expose_ingress=false"})
			return
		}
	}

	if layout == deploy.LayoutMulti {
		repoRow, _ := h.getProjectRepo(r.Context(), p.ID)
		validateBranch := strings.TrimSpace(body.Branch)
		if validateBranch == "" {
			validateBranch = strings.TrimSpace(repoRow.Branch)
		}
		if validateBranch == "" {
			validateBranch = "main"
		}
		if err := h.validateMultiServicePaths(r.Context(), u.ID, repoRow.GitHubOwner, repoRow.GitHubRepo, validateBranch, body.Services); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
	}

	ctx := r.Context()
	tx, err := h.db.Begin(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `UPDATE projects SET layout=$1 WHERE id=$2`, layout, p.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if _, err := tx.Exec(ctx, `DELETE FROM project_services WHERE project_id=$1`, p.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if layout == deploy.LayoutMulti {
		for i, s := range body.Services {
			svc := deploy.NormalizeServiceDef(deploy.ServiceDef{
				Name:           s.Name,
				DisplayName:    s.DisplayName,
				BuildContext:   s.BuildContext,
				BuildMode:      s.BuildMode,
				DockerfilePath: s.DockerfilePath,
				ContainerPort:  s.ContainerPort,
				HealthPath:     s.HealthPath,
				IngressPath:    s.IngressPath,
				ExposeIngress:  serviceRowExposeIngress(s),
				SortOrder:      s.SortOrder,
			})
			if svc.SortOrder == 0 && i > 0 {
				svc.SortOrder = i
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO project_services
				  (project_id, name, display_name, build_context, build_mode, dockerfile_path,
				   container_port, health_path, ingress_path, expose_ingress, sort_order, updated_at)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,now())`,
				p.ID, svc.Name, svc.DisplayName, svc.BuildContext, svc.BuildMode, svc.DockerfilePath,
				svc.ContainerPort, svc.HealthPath, svc.IngressPath, svc.ExposeIngress, svc.SortOrder); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	_, _ = h.db.Exec(ctx, `UPDATE project_repos SET workflow_synced_at=NULL WHERE project_id=$1`, p.ID)
	var conventionSeeds []backFrontConventionSeed
	if layout == deploy.LayoutMulti {
		conventionSeeds, _ = h.ensureBackFrontConventions(ctx, p.ID)
	}
	resp := map[string]any{
		"status": "ok",
		"layout": layout,
		"hint":   "Sync lại workflow GitHub để áp dụng cấu hình multi-service.",
	}
	if len(conventionSeeds) > 0 {
		resp["convention_seeds"] = conventionSeeds
		resp["hint"] = "Đã gợi ý env chuẩn back/front (VITE_API_BASE=/api). Sync workflow GitHub và kiểm tra tab Cấu hình app."
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) validateMultiServicePaths(ctx context.Context, userID int64, owner, ghRepo, branch string, services []projectServiceRow) error {
	owner = strings.TrimSpace(owner)
	ghRepo = strings.TrimSpace(ghRepo)
	if owner == "" || ghRepo == "" {
		return nil
	}
	token, _, err := h.getGitHubToken(ctx, userID)
	if err != nil || token == "" {
		return nil
	}
	branch = strings.TrimSpace(branch)
	if branch == "" {
		branch = "main"
	}
	client := h.githubClient()
	for _, s := range services {
		ctxPath := strings.TrimSpace(s.BuildContext)
		if ctxPath == "" {
			ctxPath = "."
		}
		ok, err := client.RepoPathExists(ctx, token, owner, ghRepo, ctxPath, branch)
		if err != nil {
			return fmt.Errorf("không kiểm tra được GitHub: %w", err)
		}
		if !ok {
			return fmt.Errorf("thư mục %q không có trên branch %q — chọn branch có backend/frontend (vd. multi-service) hoặc sửa thư mục", ctxPath, branch)
		}
		df := strings.TrimSpace(s.DockerfilePath)
		if deploy.NormalizeBuildMode(s.BuildMode) == "dockerfile" && df != "" {
			ok, err := client.RepoPathExists(ctx, token, owner, ghRepo, df, branch)
			if err != nil {
				return fmt.Errorf("không kiểm tra được Dockerfile: %w", err)
			}
			if !ok {
				return fmt.Errorf("Dockerfile %q không có trên branch %q", df, branch)
			}
		}
	}
	return nil
}
