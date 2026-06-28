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
	Found      bool                                   `json:"found"`
	Path       string                                 `json:"path,omitempty"`
	Layout     string                                 `json:"layout,omitempty"`
	Services   []deploy.ServiceDef                    `json:"services,omitempty"`
	InSync     bool                                   `json:"in_sync"`
	ParseError string                                 `json:"parse_error,omitempty"`
	Issues     []platformcontract.ServicesDetectIssue `json:"issues,omitempty"`
	Branch     string                                 `json:"branch,omitempty"`
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
	rows := h.contractToProjectServiceRows(f)
	if err := h.validateMultiServicePaths(r.Context(), u.ID, repo.GitHubOwner, repo.GitHubRepo, branch, rows); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	ctx := r.Context()
	tx, err := h.db.Begin(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `UPDATE projects SET layout=$1 WHERE id=$2`, deploy.LayoutMulti, p.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if _, err := tx.Exec(ctx, `DELETE FROM project_services WHERE project_id=$1`, p.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	for i, s := range rows {
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
	if err := tx.Commit(ctx); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	_, _ = h.db.Exec(ctx, `UPDATE project_repos SET workflow_synced_at=NULL WHERE project_id=$1`, p.ID)
	conventionSeeds, _ := h.ensureBackFrontConventions(ctx, p.ID)
	resp := map[string]any{
		"status":   "ok",
		"layout":   deploy.LayoutMulti,
		"services": len(rows),
		"source":   platformcontract.ServicesContractPath,
		"hint":     "Đã áp dụng từ repo — sync lại workflow GitHub.",
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
			BuildMode: s.BuildMode, DockerfilePath: s.DockerfilePath, ContainerPort: s.ContainerPort,
			HealthPath: s.HealthPath, IngressPath: s.IngressPath, ExposeIngress: serviceRowExposeIngress(s),
			SortOrder: s.SortOrder,
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
			return
		}
	}
	_ = tx.Commit(ctx)
	_, _ = h.ensureBackFrontConventions(ctx, p.ID)
}
