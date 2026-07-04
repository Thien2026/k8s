package handler

import (
	"context"
	"fmt"
	"strings"

	"github.com/Thien2026/k8s/services/portal-api/internal/deploy"
)

func (h *Handler) currentWorkflowProfile(ctx context.Context, projectID int64, repo projectRepoRow) (layout, services string) {
	layout = h.getProjectLayout(ctx, projectID)
	svcs, loadedLayout := h.loadDeployServices(ctx, projectID, repo)
	if loadedLayout != "" {
		layout = loadedLayout
	}
	return deploy.WorkflowProfileKey(layout, svcs)
}

func (h *Handler) workflowStaleReason(ctx context.Context, projectID int64, repo projectRepoRow) string {
	if strings.TrimSpace(repo.WorkflowSyncedAt) == "" {
		return "Workflow chưa đồng bộ — bấm 「Lưu & đồng bộ GitHub」 trên tab Deploy"
	}
	storedLayout := strings.TrimSpace(repo.WorkflowSyncLayout)
	if storedLayout == "" {
		return "Workflow GitHub chưa ghi nhận kiểu chạy — bấm 「Lưu & đồng bộ GitHub」 lại"
	}
	curLayout, curServices := h.currentWorkflowProfile(ctx, projectID, repo)
	svcs, _ := h.loadDeployServices(ctx, projectID, repo)
	if deploy.WorkflowProfileMatches(storedLayout, repo.WorkflowSyncServices, curLayout, svcs) {
		return ""
	}
	return fmt.Sprintf(
		"Workflow GitHub lệch Console (đã sync: %s · hiện tại: %s) — bấm 「Lưu & đồng bộ GitHub」 trước khi push",
		workflowProfileLabel(storedLayout, repo.WorkflowSyncServices),
		workflowProfileLabel(curLayout, curServices),
	)
}

func workflowProfileLabel(layout, services string) string {
	layout = deploy.NormalizeLayout(layout)
	if layout != deploy.LayoutMulti {
		return "Một website"
	}
	if strings.TrimSpace(services) == "" {
		return "Web + API riêng"
	}
	return "multi · " + strings.ReplaceAll(services, ",", "+")
}

func (h *Handler) requireWorkflowReady(ctx context.Context, projectID int64, repo projectRepoRow) error {
	if reason := h.workflowStaleReason(ctx, projectID, repo); reason != "" {
		return fmt.Errorf("%s", reason)
	}
	return nil
}

func (h *Handler) markWorkflowSynced(ctx context.Context, projectID int64, repo projectRepoRow) {
	layout, services := h.currentWorkflowProfile(ctx, projectID, repo)
	_, _ = h.db.Exec(ctx, `
		UPDATE project_repos SET
			workflow_synced_at=now(),
			auto_deploy_enabled=true,
			workflow_sync_layout=$1,
			workflow_sync_services=$2,
			updated_at=now()
		WHERE project_id=$3`, layout, services, projectID)
}

func (h *Handler) enrichRepoWorkflowStatus(ctx context.Context, projectID int64, repo *projectRepoRow) {
	if repo == nil {
		return
	}
	if reason := h.workflowStaleReason(ctx, projectID, *repo); reason != "" {
		repo.WorkflowStale = true
		repo.WorkflowStaleReason = reason
	}
}
