package handler

import (
	"context"
	"fmt"
	"strings"

	"github.com/Thien2026/k8s/services/portal-api/internal/deploy"
	"github.com/Thien2026/k8s/services/portal-api/internal/platformcontract"
)

func (h *Handler) validateProjectServicesLayout(ctx context.Context, userID int64, owner, ghRepo, branch, layout string, services []projectServiceRow) error {
	layout = deploy.NormalizeLayout(layout)
	if layout == deploy.LayoutMulti {
		if len(services) < 2 {
			return fmt.Errorf("Multi-service cần ít nhất 2 service (vd. api + web)")
		}
		names := map[string]bool{}
		publicCount := 0
		for _, s := range services {
			name := strings.TrimSpace(s.Name)
			if name == "" {
				return fmt.Errorf("Tên service không được rỗng")
			}
			if names[name] {
				return fmt.Errorf("Tên service trùng: %s", name)
			}
			names[name] = true
			if serviceRowExposeIngress(s) {
				publicCount++
			}
		}
		if publicCount == 0 {
			return fmt.Errorf("Cần ít nhất 1 service public (Ingress) — internal-only dùng expose_ingress=false")
		}
		branch = strings.TrimSpace(branch)
		if branch == "" {
			branch = "main"
		}
		if err := h.validateMultiServicePaths(ctx, userID, owner, ghRepo, branch, services); err != nil {
			return err
		}
	}
	return nil
}

func (h *Handler) persistProjectServices(ctx context.Context, projectID int64, layout string, services []projectServiceRow) error {
	layout = deploy.NormalizeLayout(layout)
	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `UPDATE projects SET layout=$1 WHERE id=$2`, layout, projectID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM project_services WHERE project_id=$1`, projectID); err != nil {
		return err
	}
	if layout == deploy.LayoutMulti {
		for i, s := range services {
			svc := deploy.NormalizeServiceDef(deploy.ServiceDef{
				Name:           s.Name,
				DisplayName:    s.DisplayName,
				BuildContext:   s.BuildContext,
				BuildMode:      s.BuildMode,
				Stack:          s.Stack,
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
				   container_port, health_path, ingress_path, expose_ingress, stack, sort_order, updated_at)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,now())`,
				projectID, svc.Name, svc.DisplayName, svc.BuildContext, svc.BuildMode, svc.DockerfilePath,
				svc.ContainerPort, svc.HealthPath, svc.IngressPath, svc.ExposeIngress, deploy.NormalizeStack(svc.Stack), svc.SortOrder); err != nil {
				return err
			}
		}
	}
	return tx.Commit(ctx)
}

func (h *Handler) applyServicesContractFromRepo(ctx context.Context, userID int64, p projectRow, owner, ghRepo, branch string) (int, string, error) {
	repo := projectRepoRow{GitHubOwner: owner, GitHubRepo: ghRepo, Branch: branch}
	f, found, err := h.loadServicesContractFromRepo(ctx, userID, repo, branch)
	if !found {
		return 0, "", fmt.Errorf("không tìm thấy %s trên branch %s", platformcontract.ServicesContractPath, branch)
	}
	if err != nil {
		return 0, "", err
	}
	if f.Layout != deploy.LayoutMulti {
		return 0, "", fmt.Errorf("services.yaml layout=%s — chỉ áp dụng layout multi (≥2 service)", f.Layout)
	}
	rows := h.contractToProjectServiceRows(f)
	if err := h.validateMultiServicePaths(ctx, userID, owner, ghRepo, branch, rows); err != nil {
		return 0, "", err
	}
	if err := h.persistProjectServices(ctx, p.ID, deploy.LayoutMulti, rows); err != nil {
		return 0, "", err
	}
	subMode, _ := h.resolveGitSubmodules(ctx, userID, repo, branch, &f)
	_, _ = h.db.Exec(ctx, `UPDATE project_repos SET git_submodules=$1, updated_at=now() WHERE project_id=$2`, subMode, p.ID)
	_ = h.enrichProjectServiceBuildModes(ctx, userID, p.ID, repo, branch)
	_, _ = h.ensureBackFrontConventions(ctx, p.ID)
	return len(rows), subMode, nil
}

func (h *Handler) invalidateProjectWorkflow(ctx context.Context, projectID int64) {
	_, _ = h.db.Exec(ctx, `UPDATE project_repos SET workflow_synced_at=NULL, auto_deploy_enabled=false WHERE project_id=$1`, projectID)
}
