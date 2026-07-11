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
	Stack          string `json:"stack,omitempty"`
	DockerfilePath string `json:"dockerfile_path"`
	ContainerPort  int    `json:"container_port"`
	HealthPath     string `json:"health_path"`
	IngressPath    string `json:"ingress_path"`
	ExposeIngress  *bool  `json:"expose_ingress,omitempty"`
	SortOrder      int    `json:"sort_order"`
	ResourcesMode  string `json:"resources_mode,omitempty"`
	CPURequest     string `json:"cpu_request,omitempty"`
	MemoryRequest  string `json:"memory_request,omitempty"`
	CPULimit       string `json:"cpu_limit,omitempty"`
	MemoryLimit    string `json:"memory_limit,omitempty"`
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
		       dockerfile_path, container_port, health_path, ingress_path, expose_ingress,
		       COALESCE(stack,''), sort_order,
		       COALESCE(resources_mode,'platform'), COALESCE(cpu_request,''), COALESCE(memory_request,''),
		       COALESCE(cpu_limit,''), COALESCE(memory_limit,'')
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
			&s.DockerfilePath, &s.ContainerPort, &s.HealthPath, &s.IngressPath, &expose, &s.Stack, &s.SortOrder,
			&s.ResourcesMode, &s.CPURequest, &s.MemoryRequest, &s.CPULimit, &s.MemoryLimit); err != nil {
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

func serviceRowToServiceDef(r projectServiceRow) deploy.ServiceDef {
	svc := deploy.ServiceDef{
		Name:           r.Name,
		DisplayName:    r.DisplayName,
		BuildContext:   r.BuildContext,
		BuildMode:      r.BuildMode,
		Stack:          r.Stack,
		DockerfilePath: r.DockerfilePath,
		ContainerPort:  r.ContainerPort,
		HealthPath:     r.HealthPath,
		IngressPath:    r.IngressPath,
		ExposeIngress:  serviceRowExposeIngress(r),
		SortOrder:      r.SortOrder,
		ResourcesMode:  r.ResourcesMode,
		CPURequest:     r.CPURequest,
		MemoryRequest:  r.MemoryRequest,
		CPULimit:       r.CPULimit,
		MemoryLimit:    r.MemoryLimit,
	}
	return deploy.NormalizeServiceDef(svc)
}

func (h *Handler) loadDeployServices(ctx context.Context, projectID int64, repo projectRepoRow) ([]deploy.ServiceDef, string) {
	layout := h.getProjectLayout(ctx, projectID)
	if layout != deploy.LayoutMulti {
		res, _ := h.loadRepoAppResources(ctx, projectID)
		return []deploy.ServiceDef{serviceRowToServiceDef(projectServiceRow{
			Name:           "app",
			DisplayName:    "App",
			BuildContext:   repo.BuildContext,
			BuildMode:      repo.BuildMode,
			DockerfilePath: repo.DockerfilePath,
			ContainerPort:  8080,
			HealthPath:     "/health",
			IngressPath:    "/",
			ResourcesMode:  res.ResourcesMode,
			CPURequest:     res.CPURequest,
			MemoryRequest:  res.MemoryRequest,
			CPULimit:       res.CPULimit,
			MemoryLimit:    res.MemoryLimit,
		})}, deploy.LayoutSingle
	}
	rows, err := h.listProjectServices(ctx, projectID)
	if err != nil || len(rows) == 0 {
		res, _ := h.loadRepoAppResources(ctx, projectID)
		return []deploy.ServiceDef{serviceRowToServiceDef(projectServiceRow{
			Name:           "app",
			DisplayName:    "App",
			BuildContext:   repo.BuildContext,
			BuildMode:      repo.BuildMode,
			DockerfilePath: repo.DockerfilePath,
			ContainerPort:  8080,
			HealthPath:     "/health",
			IngressPath:    "/",
			ResourcesMode:  res.ResourcesMode,
			CPURequest:     res.CPURequest,
			MemoryRequest:  res.MemoryRequest,
			CPULimit:       res.CPULimit,
			MemoryLimit:    res.MemoryLimit,
		})}, deploy.LayoutSingle
	}
	out := make([]deploy.ServiceDef, 0, len(rows))
	for _, r := range rows {
		out = append(out, serviceRowToServiceDef(r))
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
		if res, err := h.loadRepoAppResources(r.Context(), p.ID); err == nil {
			items[0].ResourcesMode = res.ResourcesMode
			items[0].CPURequest = res.CPURequest
			items[0].MemoryRequest = res.MemoryRequest
			items[0].CPULimit = res.CPULimit
			items[0].MemoryLimit = res.MemoryLimit
		}
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
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "payload không hợp lệ: " + err.Error()})
		return
	}
	layout := deploy.NormalizeLayout(body.Layout)
	repoRow, _ := h.getProjectRepo(r.Context(), p.ID)
	validateBranch := strings.TrimSpace(body.Branch)
	if validateBranch == "" {
		validateBranch = strings.TrimSpace(repoRow.Branch)
	}
	if validateBranch == "" {
		validateBranch = "main"
	}
	owner := strings.TrimSpace(repoRow.GitHubOwner)
	ghRepo := strings.TrimSpace(repoRow.GitHubRepo)
	if err := h.validateProjectServicesLayout(r.Context(), u.ID, owner, ghRepo, validateBranch, layout, body.Services); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := validateProjectServiceResources(body.Services); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := h.persistProjectServices(r.Context(), p.ID, layout, body.Services); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	h.invalidateProjectWorkflow(r.Context(), p.ID)
	var conventionSeeds []backFrontConventionSeed
	if layout == deploy.LayoutMulti {
		conventionSeeds, _ = h.ensureBackFrontConventions(r.Context(), p.ID)
	}
	resp := map[string]any{
		"status":          "ok",
		"layout":          layout,
		"workflow_stale":  true,
		"hint":            "Cấu hình đã lưu — bấm 「Lưu & đồng bộ GitHub」 để push workflow khớp layout.",
	}
	if len(conventionSeeds) > 0 {
		resp["convention_seeds"] = conventionSeeds
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
			return fmt.Errorf("không kiểm tra được thư mục %q trên branch %q: %w", ctxPath, branch, err)
		}
		if !ok {
			return fmt.Errorf("thư mục %q không có trên branch %q — chọn branch có backend/frontend (vd. multi-service) hoặc sửa thư mục", ctxPath, branch)
		}
		df := strings.TrimSpace(s.DockerfilePath)
		if deploy.NormalizeBuildMode(s.BuildMode) == "dockerfile" && df != "" {
			ok, err := client.RepoPathExists(ctx, token, owner, ghRepo, df, branch)
			if err != nil {
				return fmt.Errorf("không kiểm tra được Dockerfile %q trên branch %q: %w", df, branch, err)
			}
			if !ok {
				return fmt.Errorf("Dockerfile %q không có trên branch %q (service %s) — đổi build_mode sang buildpack hoặc sửa đường dẫn", df, branch, s.Name)
			}
		}
	}
	return nil
}
