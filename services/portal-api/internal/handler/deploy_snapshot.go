package handler

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/Thien2026/k8s/services/portal-api/internal/deploy"
)

type deployServiceSnap struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name,omitempty"`
}

type deploySnapshot struct {
	Layout   string              `json:"layout"`
	Branch   string              `json:"branch,omitempty"`
	Services []deployServiceSnap `json:"services"`
	Images   map[string]string   `json:"images,omitempty"`
}

type deployProfileView struct {
	Layout       string   `json:"layout"`
	Services     []string `json:"services"`
	Branch       string   `json:"branch,omitempty"`
	ProfileLabel string   `json:"profile_label"`
	ImageTag     string   `json:"image_tag,omitempty"`
}

func deployProfileLabel(layout string, services []deployServiceSnap) string {
	layout = deploy.NormalizeLayout(layout)
	names := make([]string, 0, len(services))
	for _, s := range services {
		n := strings.TrimSpace(s.Name)
		if n != "" {
			names = append(names, n)
		}
	}
	if layout == deploy.LayoutMulti {
		if len(names) == 0 {
			return "multi"
		}
		return "multi · " + strings.Join(names, "+")
	}
	if len(names) > 0 {
		return "single · " + names[0]
	}
	return "single · app"
}

func profileViewFromSnapshot(snap deploySnapshot, imageTag string) deployProfileView {
	services := make([]string, 0, len(snap.Services))
	for _, s := range snap.Services {
		if n := strings.TrimSpace(s.Name); n != "" {
			services = append(services, n)
		}
	}
	layout := deploy.NormalizeLayout(snap.Layout)
	if layout == "" {
		layout = deploy.LayoutSingle
	}
	return deployProfileView{
		Layout:       layout,
		Services:     services,
		Branch:       strings.TrimSpace(snap.Branch),
		ProfileLabel: deployProfileLabel(layout, snap.Services),
		ImageTag:     strings.TrimSpace(imageTag),
	}
}

func (h *Handler) buildDeploySnapshot(p projectRow, repo projectRepoRow, params deploy.Params) deploySnapshot {
	svcs := params.EffectiveServices()
	out := deploySnapshot{
		Layout: deploy.NormalizeLayout(params.Layout),
		Branch: strings.TrimSpace(repo.Branch),
		Images: map[string]string{},
	}
	if out.Layout == "" {
		if len(svcs) > 1 {
			out.Layout = deploy.LayoutMulti
		} else {
			out.Layout = deploy.LayoutSingle
		}
	}
	for _, s := range svcs {
		name := strings.TrimSpace(s.Name)
		if name == "" {
			continue
		}
		out.Services = append(out.Services, deployServiceSnap{
			Name:        name,
			DisplayName: strings.TrimSpace(s.DisplayName),
		})
		out.Images[name] = params.ImageRefFor(s)
	}
	return out
}

func (h *Handler) saveDeploymentSnapshot(ctx context.Context, deployID int64, snap deploySnapshot) {
	if deployID <= 0 {
		return
	}
	svcJSON, _ := json.Marshal(snap.Services)
	imgJSON, _ := json.Marshal(snap.Images)
	if snap.Images == nil {
		imgJSON = []byte("{}")
	}
	_, _ = h.db.Exec(ctx, `
		UPDATE project_deployments SET
			deploy_layout=$1, git_branch=$2, deploy_services=$3, deploy_images=$4, updated_at=now()
		WHERE id=$5`,
		deploy.NormalizeLayout(snap.Layout), strings.TrimSpace(snap.Branch), svcJSON, imgJSON, deployID)
}

func (h *Handler) latestDeploySnapshotForTag(ctx context.Context, projectID int64, env, tag string) (deploySnapshot, bool) {
	env = strings.ToLower(strings.TrimSpace(env))
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return deploySnapshot{}, false
	}
	var layout, branch string
	var svcRaw, imgRaw []byte
	err := h.db.QueryRow(ctx, `
		SELECT COALESCE(deploy_layout,''), COALESCE(git_branch,''),
			COALESCE(deploy_services,'[]'::jsonb), COALESCE(deploy_images,'{}'::jsonb)
		FROM project_deployments
		WHERE project_id=$1 AND environment=$2 AND image_tag=$3
			AND deploy_layout <> ''
		ORDER BY id DESC LIMIT 1`, projectID, env, tag).Scan(&layout, &branch, &svcRaw, &imgRaw)
	if err != nil || strings.TrimSpace(layout) == "" {
		return deploySnapshot{}, false
	}
	var services []deployServiceSnap
	_ = json.Unmarshal(svcRaw, &services)
	images := map[string]string{}
	_ = json.Unmarshal(imgRaw, &images)
	return deploySnapshot{
		Layout:   layout,
		Branch:   branch,
		Services: services,
		Images:   images,
	}, true
}

func (h *Handler) applySnapshotToParams(ctx context.Context, p projectRow, repo projectRepoRow, env, tag string, snap deploySnapshot, includeHook bool) deploy.Params {
	params := h.buildDeployParams(ctx, p, repo, env, tag, includeHook)
	params.Layout = deploy.NormalizeLayout(snap.Layout)
	params.ImageTag = tag
	if params.Layout == deploy.LayoutMulti && len(snap.Services) > 0 {
		params.Services = snapServicesToDefs(snap.Services, params.Services)
	} else {
		params.Services = nil
		params.Layout = deploy.LayoutSingle
	}
	return params
}

func snapServicesToDefs(snaps []deployServiceSnap, fallback []deploy.ServiceDef) []deploy.ServiceDef {
	byName := map[string]deploy.ServiceDef{}
	for _, s := range fallback {
		byName[strings.TrimSpace(s.Name)] = s
	}
	out := make([]deploy.ServiceDef, 0, len(snaps))
	for _, snap := range snaps {
		name := strings.TrimSpace(snap.Name)
		if name == "" {
			continue
		}
		if def, ok := byName[name]; ok {
			if snap.DisplayName != "" {
				def.DisplayName = snap.DisplayName
			}
			out = append(out, def)
			continue
		}
		out = append(out, deploy.ServiceDef{Name: name, DisplayName: snap.DisplayName, ContainerPort: 8080, HealthPath: "/health", IngressPath: "/"})
	}
	return out
}

func (h *Handler) resolveRollbackParams(ctx context.Context, p projectRow, repo projectRepoRow, env, tag string) (deploy.Params, []deploy.ServiceDef, bool) {
	base := h.buildDeployParams(ctx, p, repo, env, tag, false)
	if snap, ok := h.latestDeploySnapshotForTag(ctx, p.ID, env, tag); ok {
		params := h.applySnapshotToParams(ctx, p, repo, env, tag, snap, false)
		origServices := append([]deploy.ServiceDef(nil), base.EffectiveServices()...)
		rollbackFromMulti := params.Layout != deploy.LayoutMulti && deploy.NormalizeLayout(base.Layout) == deploy.LayoutMulti
		if params.Layout == deploy.LayoutMulti {
			return params, origServices, false
		}
		return params, origServices, rollbackFromMulti
	}
	return h.resolveRollbackDeployParams(ctx, p, base, tag)
}

func (d *deploymentRow) applySnapshotFields(layout, branch string, svcRaw, imgRaw []byte) {
	d.DeployLayout = deploy.NormalizeLayout(layout)
	d.GitBranch = strings.TrimSpace(branch)
	if len(svcRaw) > 0 {
		_ = json.Unmarshal(svcRaw, &d.DeployServices)
	}
	if len(imgRaw) > 0 {
		_ = json.Unmarshal(imgRaw, &d.DeployImages)
	}
	if d.DeployLayout != "" {
		d.DeployProfile = deployProfileLabel(d.DeployLayout, d.DeployServices)
	}
}

func (h *Handler) consoleDeployProfile(ctx context.Context, projectID int64, repo projectRepoRow) deployProfileView {
	services, layout := h.loadDeployServices(ctx, projectID, repo)
	snaps := make([]deployServiceSnap, 0, len(services))
	names := make([]string, 0, len(services))
	for _, s := range services {
		n := strings.TrimSpace(s.Name)
		if n == "" {
			continue
		}
		snaps = append(snaps, deployServiceSnap{Name: n, DisplayName: s.DisplayName})
		names = append(names, n)
	}
	layout = deploy.NormalizeLayout(layout)
	return deployProfileView{
		Layout:       layout,
		Services:     names,
		Branch:       strings.TrimSpace(repo.Branch),
		ProfileLabel: deployProfileLabel(layout, snaps),
	}
}

func (h *Handler) clusterRuntimeProfile(ctx context.Context, p projectRow, env string) deployProfileView {
	tag := strings.TrimSpace(h.clusterServingImageTag(ctx, p, env))
	repo, _ := h.getProjectRepo(ctx, p.ID)
	dbServices, _ := h.loadDeployServices(ctx, p.ID, repo)
	if h.clusterRunsSingleAppOnly(ctx, p, env, dbServices) {
		return deployProfileView{
			Layout:       deploy.LayoutSingle,
			Services:     []string{"app"},
			ProfileLabel: "single · app",
			ImageTag:     tag,
		}
	}
	var running []string
	for _, svc := range dbServices {
		name := strings.TrimSpace(svc.Name)
		if name == "" {
			continue
		}
		if h.serviceWorkloadExists(ctx, p, env, name) {
			running = append(running, name)
		}
	}
	if len(running) >= 2 {
		snaps := make([]deployServiceSnap, 0, len(running))
		for _, n := range running {
			snaps = append(snaps, deployServiceSnap{Name: n})
		}
		return deployProfileView{
			Layout:       deploy.LayoutMulti,
			Services:     running,
			ProfileLabel: deployProfileLabel(deploy.LayoutMulti, snaps),
			ImageTag:     tag,
		}
	}
	if len(running) == 1 {
		return deployProfileView{
			Layout:       deploy.LayoutSingle,
			Services:     running,
			ProfileLabel: deployProfileLabel(deploy.LayoutSingle, []deployServiceSnap{{Name: running[0]}}),
			ImageTag:     tag,
		}
	}
	if tag != "" {
		return deployProfileView{ProfileLabel: "tag " + shortImageTag(tag), ImageTag: tag}
	}
	return deployProfileView{ProfileLabel: "chưa deploy"}
}
