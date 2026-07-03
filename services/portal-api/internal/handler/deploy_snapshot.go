package handler

import (
	"context"
	"encoding/json"
	"fmt"
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
	for _, d := range deploy.DefaultMultiServices {
		if _, ok := byName[d.Name]; !ok {
			byName[d.Name] = d
		}
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
		out = append(out, defaultServiceDefFromSnap(name, snap.DisplayName))
	}
	return out
}

func defaultServiceDefFromSnap(name, displayName string) deploy.ServiceDef {
	def := deploy.ServiceDef{
		Name:          name,
		DisplayName:   displayName,
		ContainerPort: 8080,
		HealthPath:    "/health",
		IngressPath:   "/",
		ExposeIngress: true,
	}
	switch name {
	case "api":
		def.IngressPath = deploy.ConventionAPIBasePath
		def.HealthPath = "/health"
	case "web":
		def.IngressPath = "/"
		def.HealthPath = "/"
	}
	return deploy.NormalizeServiceDef(def)
}

// deployParamsForHealthCheck — runtime/smoke theo profile bản deploy (snapshot), không phải Console hiện tại.
func (h *Handler) deployParamsForHealthCheck(ctx context.Context, p projectRow, repo projectRepoRow, env, tag string, d *deploymentRow) deploy.Params {
	base := h.buildDeployParams(ctx, p, repo, env, tag, false)
	if d == nil || strings.TrimSpace(d.DeployLayout) == "" {
		return base
	}
	layout := deploy.NormalizeLayout(d.DeployLayout)
	if layout == deploy.LayoutMulti && len(d.DeployServices) > 0 {
		base.Layout = layout
		base.Services = snapServicesToDefs(d.DeployServices, base.EffectiveServices())
		return base
	}
	if layout == deploy.LayoutSingle {
		base.Layout = deploy.LayoutSingle
		base.Services = nil
		return base
	}
	return base
}

func (h *Handler) smokePathsForDeployment(ctx context.Context, projectID int64, repo projectRepoRow, d *deploymentRow) []string {
	if d != nil && strings.TrimSpace(d.DeployLayout) != "" {
		layout := deploy.NormalizeLayout(d.DeployLayout)
		if layout != deploy.LayoutMulti {
			return deploy.SmokeCheckPaths(deploy.LayoutSingle, nil)
		}
		fallback, _ := h.loadDeployServices(ctx, projectID, repo)
		return deploy.SmokeCheckPaths(layout, snapServicesToDefs(d.DeployServices, fallback))
	}
	return h.smokePathsForProject(ctx, projectID, repo)
}

func deploymentRowFromParams(params deploy.Params) deploymentRow {
	d := deploymentRow{DeployLayout: deploy.NormalizeLayout(params.Layout)}
	for _, s := range params.EffectiveServices() {
		d.DeployServices = append(d.DeployServices, deployServiceSnap{
			Name:        s.Name,
			DisplayName: s.DisplayName,
		})
	}
	return d
}

// ingressRoutesForParams — route Ingress theo profile deploy thực tế (rollback snapshot), không phải Console.
func ingressRoutesForParams(params deploy.Params) []deploy.IngressRoute {
	if deploy.NormalizeLayout(params.Layout) == deploy.LayoutMulti {
		routes := deploy.IngressRoutesFromServices(params.EffectiveServices())
		if len(routes) == 0 {
			routes = deploy.IngressRoutesFromServices(deploy.DefaultMultiServices)
		}
		return routes
	}
	return []deploy.IngressRoute{{
		Path:        "/",
		PathType:    "Prefix",
		ServiceName: "app",
		ServicePort: 80,
	}}
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
		if h.serviceHasReadyPods(ctx, p, env, name) {
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

func layoutUserLabel(layout string) string {
	if deploy.NormalizeLayout(layout) == deploy.LayoutMulti {
		return "Web + API riêng"
	}
	return "Một website"
}

// validateRollbackLayoutAllowed — rollback chỉ trong cùng kiểu chạy (single/multi).
func (h *Handler) validateRollbackLayoutAllowed(ctx context.Context, p projectRow, env, tag string) error {
	snap, ok := h.latestDeploySnapshotForTag(ctx, p.ID, env, tag)
	if !ok {
		return nil
	}
	target := deploy.NormalizeLayout(snap.Layout)
	if target == "" {
		return nil
	}
	cluster := h.clusterRuntimeProfile(ctx, p, env)
	current := deploy.NormalizeLayout(cluster.Layout)
	if current == "" {
		current = deploy.NormalizeLayout(p.Layout)
	}
	if current == "" {
		return nil
	}
	if target == current {
		return nil
	}
	return fmt.Errorf(
		"Không thể deploy lại bản này: bản chạy kiểu «%s» nhưng site hiện đang kiểu «%s». Chọn bản cùng kiểu hoặc deploy bản mới",
		layoutUserLabel(target), layoutUserLabel(current),
	)
}

// validateProjectLayoutMatchesRepo — chặn deploy khi Console layout ≠ services.yaml trên repo.
func (h *Handler) validateProjectLayoutMatchesRepo(ctx context.Context, userID int64, p projectRow, repo projectRepoRow) error {
	det := h.detectServicesContract(ctx, userID, p, repo, repo.Branch)
	if !det.Found || det.Layout == "" {
		return nil
	}
	projectLayout := deploy.NormalizeLayout(p.Layout)
	if projectLayout == "" {
		projectLayout = deploy.LayoutSingle
	}
	contractLayout := deploy.NormalizeLayout(det.Layout)
	if projectLayout == contractLayout {
		return nil
	}
	return fmt.Errorf(
		"Repo Git cần kiểu «%s» nhưng project đang chọn «%s». Vào tab Services → đổi kiểu cho khớp repo",
		layoutUserLabel(contractLayout), layoutUserLabel(projectLayout),
	)
}

// reconcileIngressToClusterProfile — tự sửa Ingress theo deploy đang chạy hoặc workload thực tế trên cluster.
func (h *Handler) reconcileIngressToClusterProfile(ctx context.Context, p projectRow, env, clusterID string) {
	if !h.domainSyncer().Ready() {
		return
	}
	if row, ok := h.newestActiveDeploymentRow(ctx, p.ID, env); ok {
		params := h.deployParamsFromRow(ctx, p, row)
		if params.Layout != "" {
			_ = h.syncProjectDomainsForEnv(ctx, p, env, clusterID, &params, 0)
			return
		}
	}
	prof := h.clusterRuntimeProfile(ctx, p, env)
	if prof.Layout == "" {
		return
	}
	repo, _ := h.getProjectRepo(ctx, p.ID)
	console := h.consoleDeployProfile(ctx, p.ID, repo)
	if console.Layout == prof.Layout {
		return
	}
	params := deploy.Params{Layout: prof.Layout, ImageTag: prof.ImageTag}
	if prof.Layout == deploy.LayoutMulti {
		for _, name := range prof.Services {
			params.Services = append(params.Services, deploy.ServiceDef{Name: name})
		}
	}
	_ = h.syncProjectDomainsForEnv(ctx, p, env, clusterID, &params, 0)
}

func (h *Handler) newestActiveDeploymentRow(ctx context.Context, projectID int64, env string) (deploymentRow, bool) {
	env = strings.ToLower(strings.TrimSpace(env))
	if env == "" {
		env = "dev"
	}
	rows, err := h.listProjectDeployments(ctx, projectID, env, 12)
	if err != nil || len(rows) == 0 {
		return deploymentRow{}, false
	}
	for i := range rows {
		if deploymentIsOngoingDeploy(rows[i]) {
			return rows[i], true
		}
	}
	return deploymentRow{}, false
}

func (h *Handler) deployParamsFromRow(ctx context.Context, p projectRow, d deploymentRow) deploy.Params {
	repo, _ := h.getProjectRepo(ctx, p.ID)
	env := strings.ToLower(strings.TrimSpace(d.Environment))
	if env == "" {
		env = "dev"
	}
	params := h.buildDeployParams(ctx, p, repo, env, d.ImageTag, false)
	layout := deploy.NormalizeLayout(d.DeployLayout)
	if layout != "" {
		params.Layout = layout
	}
	if len(d.DeployServices) > 0 {
		params.Services = snapServicesToDefs(d.DeployServices, params.EffectiveServices())
	}
	return params
}
