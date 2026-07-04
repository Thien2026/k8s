package handler

import (
	"context"
	"fmt"
	"strings"

	"github.com/Thien2026/k8s/services/portal-api/internal/deploy"
)

// cleanupLegacySingleApp — xóa workload single-app cũ khi chuyển sang multi-service.
func (h *Handler) cleanupLegacySingleApp(ctx context.Context, clusterID, namespace string) {
	if h.rancher == nil || !h.rancher.Enabled() {
		return
	}
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		return
	}
	_ = h.rancher.DeleteNamespacedObject(ctx, clusterID, "/apis/apps/v1/deployments", namespace, "app")
	_ = h.rancher.DeleteNamespacedObject(ctx, clusterID, "/api/v1/services", namespace, "app")
}

func harborProjectName(p projectRow) string {
	name := strings.TrimSpace(p.HarborProject)
	if name == "" {
		name = strings.TrimSpace(p.Slug)
	}
	return name
}

// requireMultiServiceImages — chặn deploy khi workflow single-app vẫn chỉ build image app.
func (h *Handler) requireMultiServiceImages(ctx context.Context, p projectRow, repo projectRepoRow, params deploy.Params, tag string) error {
	if !params.IsMultiService() {
		return nil
	}
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return nil
	}
	svcs := params.EffectiveServices()
	if len(svcs) < 2 {
		return nil
	}
	if p.RegistryProvider != "harbor" || h.harbor == nil || !h.harbor.Enabled() {
		return nil
	}
	projectName := harborProjectName(p)
	var missing []string
	for _, svc := range svcs {
		name := strings.TrimSpace(svc.Name)
		if name == "" {
			continue
		}
		ok, err := h.harbor.ArtifactExists(ctx, projectName, name, tag)
		if err != nil {
			return fmt.Errorf("kiểm tra image %s: %w", name, err)
		}
		if !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	branch := strings.TrimSpace(repo.Branch)
	if branch == "" {
		branch = "main"
	}
	appOnly, _ := h.harbor.ArtifactExists(ctx, projectName, "app", tag)
	if appOnly {
		return fmt.Errorf(
			"registry chỉ có image app:%s — Console đang multi-service (cần %s). Bấm 「Lưu & đồng bộ GitHub」 trên branch %s rồi chạy build lại",
			shortImageTag(tag),
			strings.Join(missing, ", "),
			branch,
		)
	}
	return fmt.Errorf(
		"thiếu image %s (tag %s) trong Harbor — đồng bộ workflow GitHub (branch %s) rồi build lại",
		strings.Join(missing, ", "),
		shortImageTag(tag),
		branch,
	)
}

// validateRollbackImages — rollback: báo rõ nếu tag single-app vs layout multi.
func (h *Handler) validateRollbackImages(ctx context.Context, p projectRow, params deploy.Params, tag string) error {
	if !params.IsMultiService() {
		return nil
	}
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return nil
	}
	if p.RegistryProvider != "harbor" || h.harbor == nil || !h.harbor.Enabled() {
		return nil
	}
	projectName := harborProjectName(p)
	svcs := params.EffectiveServices()
	var missing []string
	for _, svc := range svcs {
		name := strings.TrimSpace(svc.Name)
		if name == "" {
			continue
		}
		ok, err := h.harbor.ArtifactExists(ctx, projectName, name, tag)
		if err != nil {
			return fmt.Errorf("kiểm tra image %s: %w", name, err)
		}
		if !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	appOk, _ := h.harbor.ArtifactExists(ctx, projectName, "app", tag)
	if appOk {
		return fmt.Errorf(
			"tag %s là bản single-app (chỉ có image app) — không rollback được khi layout multi-service; chọn tag đã build api+web",
			shortImageTag(tag),
		)
	}
	return fmt.Errorf(
		"thiếu image %s (tag %s) trên Harbor — có thể đã bị xóa hoặc chưa từng build multi-service",
		strings.Join(missing, ", "),
		shortImageTag(tag),
	)
}

// cleanupMultiServiceWorkloads — gỡ deployment/service multi cũ (rollback hoặc chuyển sang single).
func (h *Handler) cleanupMultiServiceWorkloads(ctx context.Context, clusterID, namespace string, services []deploy.ServiceDef) {
	if h.rancher == nil || !h.rancher.Enabled() {
		return
	}
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		return
	}
	for _, svc := range services {
		name := strings.TrimSpace(svc.Name)
		if name == "" || name == "app" {
			continue
		}
		_ = h.rancher.DeleteNamespacedObject(ctx, clusterID, "/apis/apps/v1/deployments", namespace, name)
		_ = h.rancher.DeleteNamespacedObject(ctx, clusterID, "/api/v1/services", namespace, name)
	}
}

// cleanupClusterMultiWorkloads — khi deploy single: xóa mọi deployment ≠ app còn sót trên cluster.
func (h *Handler) cleanupClusterMultiWorkloads(ctx context.Context, clusterID, namespace string) {
	if h.rancher == nil || !h.rancher.Enabled() {
		return
	}
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		return
	}
	list, err := h.rancher.ListK8s(ctx, clusterID, "deployments", namespace, 1, 100)
	if err != nil || len(list.Items) == 0 {
		h.cleanupMultiServiceWorkloads(ctx, clusterID, namespace, deploy.DefaultMultiServices)
		return
	}
	for _, item := range list.Items {
		name := strings.TrimSpace(item.Name)
		if name == "" || name == "app" {
			continue
		}
		_ = h.rancher.DeleteNamespacedObject(ctx, clusterID, "/apis/apps/v1/deployments", namespace, name)
		_ = h.rancher.DeleteNamespacedObject(ctx, clusterID, "/api/v1/services", namespace, name)
	}
}

func (h *Handler) validatePromoteLayoutMatch(ctx context.Context, projectID int64, tag string) error {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return nil
	}
	consoleLayout := h.getProjectLayout(ctx, projectID)
	var deployLayout string
	err := h.db.QueryRow(ctx, `
		SELECT COALESCE(deploy_layout,'') FROM project_deployments
		WHERE project_id=$1 AND environment='dev' AND image_tag=$2
		ORDER BY id DESC LIMIT 1`, projectID, tag).Scan(&deployLayout)
	if err != nil || strings.TrimSpace(deployLayout) == "" {
		return nil
	}
	if deploy.NormalizeLayout(deployLayout) == deploy.NormalizeLayout(consoleLayout) {
		return nil
	}
	return fmt.Errorf(
		"tag %s là bản 「%s」 — Console hiện 「%s」. Chỉ promote bản dev cùng kiểu chạy (tag single mới sau khi đổi kiểu)",
		shortImageTag(tag),
		layoutUserLabel(deployLayout),
		layoutUserLabel(consoleLayout),
	)
}

func (h *Handler) harborHasArtifact(ctx context.Context, p projectRow, repoName, tag string) bool {
	if p.RegistryProvider != "harbor" || h.harbor == nil || !h.harbor.Enabled() {
		return false
	}
	ok, err := h.harbor.ArtifactExists(ctx, harborProjectName(p), repoName, tag)
	return err == nil && ok
}

// resolveRollbackDeployParams — tag single-app (chỉ image app) rollback về layout 1 deployment dù Console đang multi.
func (h *Handler) resolveRollbackDeployParams(ctx context.Context, p projectRow, params deploy.Params, tag string) (deploy.Params, []deploy.ServiceDef, bool) {
	tag = strings.TrimSpace(tag)
	origServices := append([]deploy.ServiceDef(nil), params.EffectiveServices()...)
	if !params.IsMultiService() || tag == "" {
		return params, origServices, false
	}
	allMulti := true
	for _, svc := range origServices {
		if !h.harborHasArtifact(ctx, p, svc.Name, tag) {
			allMulti = false
			break
		}
	}
	if allMulti {
		return params, origServices, false
	}
	if !h.harborHasArtifact(ctx, p, "app", tag) {
		return params, origServices, false
	}
	sp := params
	sp.Layout = deploy.LayoutSingle
	sp.Services = nil
	return sp, origServices, true
}
