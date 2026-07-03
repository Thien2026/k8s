package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Thien2026/k8s/services/portal-api/internal/auth"
	"github.com/Thien2026/k8s/services/portal-api/internal/deploy"
	"github.com/Thien2026/k8s/services/portal-api/internal/platformcontract"
	"github.com/go-chi/chi/v5"
)

type servicesContractDetect struct {
	Found               bool                                   `json:"found"`
	Path                string                                 `json:"path,omitempty"`
	Layout              string                                 `json:"layout,omitempty"`
	Services            []deploy.ServiceDef                    `json:"services,omitempty"`
	InSync              bool                                   `json:"in_sync"`
	GitSubmodules       string                                 `json:"git_submodules,omitempty"`
	GitSubmodulesInSync bool                                   `json:"git_submodules_in_sync,omitempty"`
	HasGitmodules       bool                                   `json:"has_gitmodules,omitempty"`
	ParseError          string                                 `json:"parse_error,omitempty"`
	Issues              []platformcontract.ServicesDetectIssue `json:"issues,omitempty"`
	Branch              string                                 `json:"branch,omitempty"`
}

func (h *Handler) loadServicesContractFromRepo(ctx context.Context, userID int64, repo projectRepoRow, branch string) (platformcontract.ServicesFile, bool, error) {
	owner := strings.TrimSpace(repo.GitHubOwner)
	ghRepo := strings.TrimSpace(repo.GitHubRepo)
	if owner == "" || ghRepo == "" {
		return platformcontract.ServicesFile{}, false, nil
	}
	token, _, err := h.getGitHubToken(ctx, userID)
	if err != nil || token == "" {
		return platformcontract.ServicesFile{}, false, nil
	}
	branch = strings.TrimSpace(branch)
	if branch == "" {
		branch = strings.TrimSpace(repo.Branch)
	}
	if branch == "" {
		branch = "main"
	}
	raw, ok, err := h.githubClient().GetFileContent(ctx, token, owner, ghRepo, platformcontract.ServicesContractPath, branch)
	if err != nil {
		return platformcontract.ServicesFile{}, false, err
	}
	if !ok {
		return platformcontract.ServicesFile{}, false, nil
	}
	f, err := platformcontract.ParseServices(raw)
	if err != nil {
		return platformcontract.ServicesFile{}, true, err
	}
	return f, true, nil
}

func (h *Handler) resolveGitSubmodules(ctx context.Context, userID int64, repo projectRepoRow, branch string, f *platformcontract.ServicesFile) (mode string, hasGitmodules bool) {
	if f != nil {
		mode = platformcontract.ResolveSubmodulesMode(f.Git.Submodules, f.Submodules)
	}
	owner := strings.TrimSpace(repo.GitHubOwner)
	ghRepo := strings.TrimSpace(repo.GitHubRepo)
	if owner == "" || ghRepo == "" {
		return mode, mode != ""
	}
	token, _, err := h.getGitHubToken(ctx, userID)
	if err != nil || token == "" {
		return mode, mode != ""
	}
	branch = strings.TrimSpace(branch)
	if branch == "" {
		branch = strings.TrimSpace(repo.Branch)
	}
	if branch == "" {
		branch = "main"
	}
	ok, err := h.githubClient().RepoPathExists(ctx, token, owner, ghRepo, ".gitmodules", branch)
	if err == nil && ok {
		hasGitmodules = true
		if mode == "" {
			mode = "recursive"
		}
	}
	return mode, hasGitmodules || mode != ""
}

func (h *Handler) detectServicesContract(ctx context.Context, userID int64, p projectRow, repo projectRepoRow, branch string) servicesContractDetect {
	out := servicesContractDetect{Branch: branch}
	f, found, err := h.loadServicesContractFromRepo(ctx, userID, repo, branch)
	if !found {
		return out
	}
	out.Found = true
	out.Path = platformcontract.ServicesContractPath
	if err != nil {
		out.ParseError = err.Error()
		return out
	}
	out.Layout = f.Layout
	if f.Layout == deploy.LayoutMulti {
		out.Services = deploy.ServiceDefsFromContract(f)
		dbSvcs, layout := h.loadDeployServices(ctx, p.ID, repo)
		out.InSync = layout == deploy.LayoutMulti && deploy.ServicesContractSameAsDB(f, dbSvcs)
	}
	subMode, hasGitmodules := h.resolveGitSubmodules(ctx, userID, repo, branch, &f)
	out.GitSubmodules = subMode
	out.HasGitmodules = hasGitmodules
	out.GitSubmodulesInSync = strings.TrimSpace(repo.GitSubmodules) == subMode
	return out
}

func (h *Handler) contractToProjectServiceRows(f platformcontract.ServicesFile) []projectServiceRow {
	svcs := deploy.ServiceDefsFromContract(f)
	rows := make([]projectServiceRow, 0, len(svcs))
	for _, s := range svcs {
		expose := s.ExposeIngress
		rows = append(rows, projectServiceRow{
			Name:           s.Name,
			DisplayName:    s.DisplayName,
			BuildContext:   s.BuildContext,
			BuildMode:      s.BuildMode,
			Stack:          s.Stack,
			DockerfilePath: s.DockerfilePath,
			ContainerPort:  s.ContainerPort,
			HealthPath:     s.HealthPath,
			IngressPath:    s.IngressPath,
			ExposeIngress:  &expose,
			SortOrder:      s.SortOrder,
		})
	}
	return rows
}

func (h *Handler) DetectProjectServicesContract(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	p, ok := h.requireProjectAccess(w, r, slug)
	if !ok {
		return
	}
	u, _ := auth.UserFromContext(r.Context())
	repo, _ := h.getProjectRepo(r.Context(), p.ID)
	branch := strings.TrimSpace(r.URL.Query().Get("branch"))
	if branch == "" {
		branch = strings.TrimSpace(repo.Branch)
	}
	writeJSON(w, http.StatusOK, h.detectServicesContract(r.Context(), u.ID, p, repo, branch))
}

func (h *Handler) SyncProjectServicesFromRepo(w http.ResponseWriter, r *http.Request) {
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
		Branch string `json:"branch,omitempty"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	repo, _ := h.getProjectRepo(r.Context(), p.ID)
	branch := strings.TrimSpace(body.Branch)
	if branch == "" {
		branch = strings.TrimSpace(repo.Branch)
	}
	f, found, err := h.loadServicesContractFromRepo(r.Context(), u.ID, repo, branch)
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "Không tìm thấy " + platformcontract.ServicesContractPath + " trên branch " + branch,
		})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if f.Layout != deploy.LayoutMulti {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "services.yaml layout=" + f.Layout + " — sync contract chỉ áp dụng layout multi (≥2 service)",
		})
		return
	}
	owner := strings.TrimSpace(repo.GitHubOwner)
	ghRepo := strings.TrimSpace(repo.GitHubRepo)
	if owner == "" || ghRepo == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Chưa cấu hình repo GitHub — kết nối repo trước"})
		return
	}
	n, subMode, err := h.applyServicesContractFromRepo(r.Context(), u.ID, p, owner, ghRepo, branch)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	h.invalidateProjectWorkflow(r.Context(), p.ID)
	conventionSeeds, _ := h.ensureBackFrontConventions(r.Context(), p.ID)
	resp := map[string]any{
		"status":          "ok",
		"layout":          deploy.LayoutMulti,
		"services":        n,
		"git_submodules":  subMode,
		"source":          platformcontract.ServicesContractPath,
		"workflow_stale":  true,
		"hint":            "Đã áp dụng từ repo — bấm 「Lưu & đồng bộ GitHub」 để push workflow.",
	}
	if len(conventionSeeds) > 0 {
		resp["convention_seeds"] = conventionSeeds
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) maybeApplyServicesContractOnSetup(ctx context.Context, userID int64, p projectRow, repo projectRepoRow, branch string) {
	existing, err := h.listProjectServices(ctx, p.ID)
	if err != nil || len(existing) > 0 {
		return
	}
	f, found, err := h.loadServicesContractFromRepo(ctx, userID, repo, branch)
	if !found || err != nil || f.Layout != deploy.LayoutMulti {
		return
	}
	rows := h.contractToProjectServiceRows(f)
	if err := h.validateMultiServicePaths(ctx, userID, repo.GitHubOwner, repo.GitHubRepo, branch, rows); err != nil {
		return
	}
	tx, err := h.db.Begin(ctx)
	if err != nil {
		return
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE projects SET layout=$1 WHERE id=$2`, deploy.LayoutMulti, p.ID); err != nil {
		return
	}
	for i, s := range rows {
		svc := deploy.NormalizeServiceDef(deploy.ServiceDef{
			Name: s.Name, DisplayName: s.DisplayName, BuildContext: s.BuildContext,
			BuildMode: s.BuildMode, Stack: s.Stack, DockerfilePath: s.DockerfilePath, ContainerPort: s.ContainerPort,
			HealthPath: s.HealthPath, IngressPath: s.IngressPath, ExposeIngress: serviceRowExposeIngress(s),
			SortOrder: s.SortOrder,
		})
		if svc.SortOrder == 0 && i > 0 {
			svc.SortOrder = i
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO project_services
			  (project_id, name, display_name, build_context, build_mode, dockerfile_path,
			   container_port, health_path, ingress_path, expose_ingress, stack, sort_order, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,now())`,
			p.ID, svc.Name, svc.DisplayName, svc.BuildContext, svc.BuildMode, svc.DockerfilePath,
			svc.ContainerPort, svc.HealthPath, svc.IngressPath, svc.ExposeIngress, deploy.NormalizeStack(svc.Stack), svc.SortOrder); err != nil {
			return
		}
	}
	subMode, _ := h.resolveGitSubmodules(ctx, userID, repo, branch, &f)
	_, _ = tx.Exec(ctx, `UPDATE project_repos SET git_submodules=$1, updated_at=now() WHERE project_id=$2`, subMode, p.ID)
	_ = tx.Commit(ctx)
	_ = h.enrichProjectServiceBuildModes(ctx, userID, p.ID, repo, branch)
	_, _ = h.ensureBackFrontConventions(ctx, p.ID)
}

func (h *Handler) syncGitSubmodulesFromContract(ctx context.Context, userID, projectID int64, repo projectRepoRow, branch string) {
	var file *platformcontract.ServicesFile
	f, found, err := h.loadServicesContractFromRepo(ctx, userID, repo, branch)
	if found && err == nil {
		file = &f
	}
	subMode, _ := h.resolveGitSubmodules(ctx, userID, repo, branch, file)
	_, _ = h.db.Exec(ctx, `UPDATE project_repos SET git_submodules=$1, updated_at=now() WHERE project_id=$2`, subMode, projectID)
}
