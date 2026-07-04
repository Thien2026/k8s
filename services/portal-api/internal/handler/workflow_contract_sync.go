package handler

import (
	"context"
	"fmt"

	"github.com/Thien2026/k8s/services/portal-api/internal/deploy"
	"github.com/Thien2026/k8s/services/portal-api/internal/platformcontract"
)

func (h *Handler) syncProjectServicesYAMLToRepo(ctx context.Context, token, owner, repo, branch string, projectID int64, repoRow projectRepoRow) error {
	layout := h.getProjectLayout(ctx, projectID)
	if layout != deploy.LayoutMulti {
		return nil
	}
	rows, err := h.listProjectServices(ctx, projectID)
	if err != nil || len(rows) < 2 {
		return nil
	}
	specs := make([]platformcontract.ServiceSpec, 0, len(rows))
	for _, r := range rows {
		expose := serviceRowExposeIngress(r)
		spec := platformcontract.ServiceSpec{
			Name:           r.Name,
			DisplayName:    r.DisplayName,
			Path:           r.BuildContext,
			BuildMode:      r.BuildMode,
			Stack:          r.Stack,
			DockerfilePath: r.DockerfilePath,
			Port:           r.ContainerPort,
			Health:         r.HealthPath,
			Ingress:        r.IngressPath,
		}
		if !expose {
			f := false
			spec.Expose = &f
		}
		specs = append(specs, spec)
	}
	f := platformcontract.ServicesFileFromDefs(deploy.LayoutMulti, specs, repoRow.GitSubmodules)
	content, err := platformcontract.RenderServicesYAML(f)
	if err != nil {
		return err
	}
	path := platformcontract.ServicesContractPath
	msg := fmt.Sprintf("chore(platform): sync %s for multi-service", path)
	return h.githubClient().PutWorkflowFile(ctx, token, owner, repo, path, msg, content, branch)
}
