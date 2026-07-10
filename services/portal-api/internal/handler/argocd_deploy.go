package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Thien2026/k8s/services/portal-api/internal/deploy"
	"github.com/Thien2026/k8s/services/portal-api/internal/gitops"
)

func (h *Handler) deployPullSecretName(p projectRow) string {
	if strings.EqualFold(strings.TrimSpace(p.RegistryProvider), "harbor") {
		return deploy.HarborPullSecretName
	}
	return deploy.GHCRPullSecretName
}

func (h *Handler) ensureDeployPullSecret(ctx context.Context, p projectRow, params deploy.Params, clusterID string) (string, error) {
	if h.rancher == nil || !h.rancher.Enabled() {
		return "", fmt.Errorf("rancher chưa sẵn sàng")
	}
	if err := h.rancher.EnsureNamespace(ctx, clusterID, params.Namespace); err != nil {
		return "", err
	}
	if p.RegistryProvider == "harbor" {
		host := harborHostFromURL(h.cfg.HarborURL)
		user, pass, err := h.ensureHarborCIRobot(ctx, p)
		if err != nil {
			return "", fmt.Errorf("harbor pull secret: %w", err)
		}
		secretJSON, err := deploy.HarborPullSecret(params.Namespace, host, user, pass)
		if err != nil {
			return "", err
		}
		if err := h.rancher.ApplyNamespacedObject(ctx, clusterID, "/api/v1/secrets", params.Namespace, secretJSON); err != nil {
			return "", fmt.Errorf("harbor pull secret: %w", err)
		}
		return deploy.HarborPullSecretName, nil
	}
	user, tok, err := h.ensureGHCRCredentials(ctx, p)
	if err != nil {
		return "", fmt.Errorf("ghcr pull secret: %w", err)
	}
	secretJSON, err := deploy.GHCRPullSecret(params.Namespace, user, tok)
	if err != nil {
		return "", err
	}
	if err := h.rancher.ApplyNamespacedObject(ctx, clusterID, "/api/v1/secrets", params.Namespace, secretJSON); err != nil {
		return "", fmt.Errorf("ghcr pull secret: %w", err)
	}
	return deploy.GHCRPullSecretName, nil
}

func (h *Handler) pushGitOpsOverlayForDeploy(ctx context.Context, p projectRow, params deploy.Params, imageTag string) error {
	g := h.loadGitOpsConfig(ctx)
	if !g.Configured || strings.TrimSpace(g.PushToken) == "" {
		return fmt.Errorf("GitOps chưa cấu hình push token")
	}
	ref, err := gitops.ParseRepoURL(g.RepoURL)
	if err != nil {
		return err
	}
	env := strings.ToLower(strings.TrimSpace(params.Environment))
	if env == "" {
		env = "dev"
	}
	overlayPath := gitops.OverlayPath(g.BasePath, params.ProjectSlug, env)
	patchPath := gitops.OverlayPatchPath(g.BasePath, params.ProjectSlug, env, gitops.PullSecretPatchFileName())

	client := gitops.NewClient()
	if err := h.ensureGitOpsOverlayScaffolded(ctx, p, params); err != nil {
		return err
	}
	msg := fmt.Sprintf("chore(gitops): %s %s -> %s", params.ProjectSlug, env, shortImageTag(imageTag))
	if err := gitops.SyncBaseManifests(ctx, client, g.PushToken, ref, g.BasePath, p.Slug, g.RepoBranch, params, msg); err != nil {
		return fmt.Errorf("gitops base: %w", err)
	}
	content, err := client.GetFile(ctx, g.PushToken, ref, overlayPath, g.RepoBranch)
	if err != nil {
		return fmt.Errorf("đọc %s: %w", overlayPath, err)
	}
	imageNames := make([]string, 0, len(params.EffectiveServices()))
	for _, svc := range params.EffectiveServices() {
		imageNames = append(imageNames, params.ImageRepositoryFor(svc))
	}
	updated, err := gitops.RewriteOverlayImagesSection(content, imageNames, imageTag)
	if err != nil {
		return err
	}
	secretName := h.deployPullSecretName(p)
	updated, err = gitops.EnsureOverlayPullSecret(updated, secretName)
	if err != nil {
		return err
	}
	hasEnv := false
	if vars, _ := h.envVarsMap(ctx, p.ID, params.Environment); len(vars) > 0 {
		hasEnv = true
		updated, err = gitops.EnsureOverlayEnvFrom(updated, deploy.AppEnvSecretName)
		if err != nil {
			return err
		}
	}
	updated = gitops.ConsolidateOverlayKustomization(updated)
	if err := client.PutFile(ctx, g.PushToken, ref, overlayPath, g.RepoBranch, msg, updated); err != nil {
		return err
	}
	// GitHub cần vài giây để ArgoCD đọc commit mới.
	time.Sleep(2 * time.Second)
	if patchYAML := gitops.PullSecretPatchYAML(secretName); patchYAML != "" {
		if err := client.PutFile(ctx, g.PushToken, ref, patchPath, g.RepoBranch, msg+" (pull secret)", patchYAML); err != nil {
			return err
		}
	}
	if hasEnv {
		envPatchPath := gitops.OverlayPatchPath(g.BasePath, params.ProjectSlug, env, gitops.EnvFromPatchFileName())
		if patchYAML := gitops.EnvFromPatchYAML(deploy.AppEnvSecretName); patchYAML != "" {
			if err := client.PutFile(ctx, g.PushToken, ref, envPatchPath, g.RepoBranch, msg+" (env)", patchYAML); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *Handler) ensureGitOpsOverlayScaffolded(ctx context.Context, p projectRow, params deploy.Params) error {
	g := h.loadGitOpsConfig(ctx)
	if !g.Configured || strings.TrimSpace(g.PushToken) == "" {
		return fmt.Errorf("GitOps chưa cấu hình push token")
	}
	ref, err := gitops.ParseRepoURL(g.RepoURL)
	if err != nil {
		return err
	}
	env := strings.ToLower(strings.TrimSpace(params.Environment))
	if env == "" {
		env = "dev"
	}
	overlayPath := gitops.OverlayPath(g.BasePath, p.Slug, env)
	client := gitops.NewClient()
	ok, err := client.FileExists(ctx, g.PushToken, ref, overlayPath, g.RepoBranch)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	repo, err := h.getProjectRepo(ctx, p.ID)
	if err != nil {
		return err
	}
	h.enrichProjectRegistry(ctx, &p)
	devParams := h.buildDeployParams(ctx, p, repo, "dev", "latest", false)
	prodParams := h.buildDeployParams(ctx, p, repo, "prod", "latest", false)
	devParams.ImagePullSecret = h.deployPullSecretName(p)
	prodParams.ImagePullSecret = h.deployPullSecretName(p)
	files, err := gitops.BuildFiles(gitops.ScaffoldInput{
		Slug:       p.Slug,
		BasePath:   g.BasePath,
		DevParams:  devParams,
		ProdParams: prodParams,
	})
	if err != nil {
		return fmt.Errorf("scaffold GitOps: %w", err)
	}
	msg := fmt.Sprintf("chore(gitops): scaffold %s overlay %s", p.Slug, env)
	for path, content := range files {
		exists, err := client.FileExists(ctx, g.PushToken, ref, path, g.RepoBranch)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if err := client.PutFile(ctx, g.PushToken, ref, path, g.RepoBranch, msg+" "+path, content); err != nil {
			return fmt.Errorf("push %s: %w", path, err)
		}
	}
	return nil
}

func (h *Handler) triggerArgoApplicationSync(ctx context.Context, appName string) error {
	if h.rancher == nil || !h.rancher.Enabled() {
		return fmt.Errorf("rancher chưa sẵn sàng")
	}
	g := h.loadGitOpsConfig(ctx)
	revision := strings.TrimSpace(g.RepoBranch)
	if revision == "" {
		revision = "main"
	}
	ns := strings.TrimSpace(h.cfg.ArgoCDNamespace)
	if ns == "" {
		ns = "argocd"
	}
	payload := map[string]any{
		"operation": map[string]any{
			"initiatedBy": map[string]any{"username": "platform"},
			"sync": map[string]any{
				"revision": revision,
				"prune":    true,
			},
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 2 * time.Second)
		}
		if err := h.rancher.MergePatchNamespacedObject(ctx, "", "/apis/argoproj.io/v1alpha1/applications", ns, appName, raw); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	return lastErr
}

func (h *Handler) applyArgoDeploy(ctx context.Context, p projectRow, repo projectRepoRow, params deploy.Params, imageTag, clusterID string, deployID int64) (appName, appURL string, err error) {
	if err := h.validateDeployImagesMatchLayout(ctx, p, repo, params, imageTag); err != nil {
		return "", "", err
	}
	pullSecret, err := h.ensureDeployPullSecret(ctx, p, params, clusterID)
	if err != nil {
		return "", "", err
	}
	params.ImagePullSecret = pullSecret
	if err := h.attachAppEnvToDeploy(ctx, p, params.Environment, clusterID); err != nil {
		return "", "", fmt.Errorf("app env: %w", err)
	}
	if params.IsMultiService() {
		h.cleanupLegacySingleApp(ctx, clusterID, params.Namespace)
	}
	if err := h.pushGitOpsOverlayForDeploy(ctx, p, params, imageTag); err != nil {
		return "", "", fmt.Errorf("gitops overlay: %w", err)
	}
	appName, appURL, err = h.ensureArgoApplication(ctx, p, params.Environment, imageTag)
	if err != nil {
		return appName, appURL, err
	}
	if syncErr := h.triggerArgoApplicationSync(ctx, appName); syncErr != nil {
		if deployID > 0 {
			_, _ = h.db.Exec(ctx, `
				UPDATE project_deployments SET deploy_status='running', runtime_status='running',
					runtime_detail=$1, updated_at=now()
				WHERE id=$2`, "ArgoCD sync queued (retry): "+syncErr.Error(), deployID)
		}
		return appName, appURL, nil
	}
	if deployID > 0 {
		_, _ = h.db.Exec(ctx, `
			UPDATE project_deployments SET deploy_status='running', runtime_status='running',
				runtime_detail=$1, updated_at=now()
			WHERE id=$2`, "ArgoCD sync triggered: "+appName, deployID)
	}
	return appName, appURL, nil
}

func (h *Handler) finishArgoDeployRuntimeSync(ctx context.Context, deployID int64, p projectRow, params deploy.Params, imageTag string) {
	deadline := time.Now().Add(12 * time.Minute)
	drow := deploymentRowFromParams(params)
	drow.ID = deployID
	drow.ImageTag = imageTag
	drow.Environment = params.Environment
	drow.DeployStatus = "success"
	drow.RuntimeStatus = "running"
	drow.Status = "in_progress"
	appName := h.argoAppName(p.Slug, params.Environment)
	syncRetries := 0

	for time.Now().Before(deadline) {
		if h.isSupersededByNewerDeploy(ctx, p.ID, params.Environment, deployID) {
			return
		}
		if syncRetries < 2 {
			if st, err := h.argoApplicationStatus(ctx, appName); err == nil {
				syncSt := strings.ToLower(strings.TrimSpace(st.SyncStatus))
				if syncSt == "outofsync" || syncSt == "unknown" || syncSt == "" {
					syncRetries++
					_ = h.triggerArgoApplicationSync(ctx, appName)
					time.Sleep(3 * time.Second)
				}
			}
		}
		v := h.assessRuntimeHealth(ctx, p, &drow)
		if v.Status == "failed" {
			h.markDeploymentFailed(ctx, deployID, "runtime", v.ErrorMsg)
			return
		}
		if v.Status == "success" {
			if live, detail := h.deploymentTrafficLive(ctx, p, params.Environment, imageTag); !live {
				_, _ = h.db.Exec(ctx, `
					UPDATE project_deployments SET deploy_status='success', runtime_status='running', runtime_detail=$1,
						status='in_progress', updated_at=now() WHERE id=$2`, detail, deployID)
				time.Sleep(5 * time.Second)
				continue
			}
			h.markDeploymentSuccess(ctx, deployID, v.Detail)
			return
		}
		_, _ = h.db.Exec(ctx, `
			UPDATE project_deployments SET deploy_status='success', runtime_status=$1, runtime_detail=$2,
				status='in_progress', updated_at=now() WHERE id=$3`, v.Status, v.Detail, deployID)
		time.Sleep(5 * time.Second)
	}
	h.markDeploymentFailed(ctx, deployID, "runtime", "ArgoCD chưa Synced/Healthy trong thời gian chờ — kiểm tra Argo CD Application")
}
