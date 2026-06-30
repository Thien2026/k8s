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
