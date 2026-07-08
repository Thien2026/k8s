package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Thien2026/k8s/services/portal-api/internal/auth"
	"github.com/Thien2026/k8s/services/portal-api/internal/deploy"
	"github.com/Thien2026/k8s/services/portal-api/internal/plugins"
	"github.com/Thien2026/k8s/services/portal-api/internal/rancher"
	"github.com/go-chi/chi/v5"
)

func (h *Handler) projectDeployParams(r *http.Request, p projectRow, envOverride, tagOverride string) (deploy.Params, string, error) {
	env := strings.ToLower(strings.TrimSpace(envOverride))
	if env == "" {
		env = strings.ToLower(strings.TrimSpace(r.URL.Query().Get("environment")))
	}
	if env == "" {
		env = "dev"
	}

	repo, _ := h.getProjectRepo(r.Context(), p.ID)
	if u, ok := auth.UserFromContext(r.Context()); ok {
		_ = h.resolveBuildMode(r.Context(), u.ID, p.ID, &repo)
	}

	tag := strings.TrimSpace(tagOverride)
	if tag == "" {
		tag = strings.TrimSpace(r.URL.Query().Get("image_tag"))
	}

	params := h.buildDeployParams(r.Context(), p, repo, env, tag, repo.AutoDeployEnabled)
	return params, env, nil
}

func (h *Handler) GetProjectDeployPlan(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	p, ok := h.requireProjectAccess(w, r, slug)
	if !ok {
		return
	}
	h.enrichProjectRegistry(r.Context(), &p)
	params, _, err := h.projectDeployParams(r, p, "", "")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	rancherOn, _ := h.plugins.Enabled(r.Context(), plugins.Rancher)
	rancherReady := rancherOn && h.rancher != nil && h.rancher.Enabled()
	registryReady := p.Registry.Ready
	u, _ := auth.UserFromContext(r.Context())
	canApply := rancherReady && auth.CanWriteK8s(u.Role)

	plan, err := deploy.BuildPlan(params, rancherReady, registryReady, canApply)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, plan)
}

func (h *Handler) ApplyProjectDeploy(w http.ResponseWriter, r *http.Request) {
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
		Environment string `json:"environment"`
		ImageTag    string `json:"image_tag"`
		ClusterID   string `json:"cluster_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "payload không hợp lệ"})
		return
	}

	rancherOn, _ := h.plugins.Enabled(r.Context(), plugins.Rancher)
	if !rancherOn || h.rancher == nil || !h.rancher.Enabled() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "Rancher chưa sẵn sàng — bật addon và cài cluster trước",
		})
		return
	}
	h.enrichProjectRegistry(r.Context(), &p)
	repo, _ := h.getProjectRepo(r.Context(), p.ID)
	if err := h.validateProjectLayoutMatchesRepo(r.Context(), u.ID, p, repo); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	if err := h.requireWorkflowReady(r.Context(), p.ID, repo); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	result, err := h.applyProjectDeploy(r.Context(), p, body.Environment, body.ImageTag, body.ClusterID, true, false)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) applyProjectDeploy(ctx context.Context, p projectRow, env, imageTag, clusterID string, asyncRuntime bool, rollback bool) (map[string]any, error) {
	if err := h.requireDeployEnvReady(ctx, p, env); err != nil {
		return nil, err
	}
	h.enrichProjectRegistry(ctx, &p)
	repo, _ := h.getProjectRepo(ctx, p.ID)
	params := h.buildDeployParams(ctx, p, repo, env, imageTag, false)
	var multiServices []deploy.ServiceDef
	var rollbackFromMulti bool
	if rollback {
		params, multiServices, rollbackFromMulti = h.resolveRollbackParams(ctx, p, repo, env, imageTag)
	}
	params.ForceRolloutRestart = rollback
	imageRef := deployImageRef(params)

	var deployID int64
	var err error
	if rollback {
		deployID, err = h.beginRollbackDeployment(ctx, p.ID, params.Environment, imageTag)
	} else {
		deployID, err = h.getOrCreateDeploymentID(ctx, p.ID, params.Environment, imageTag)
	}
	if err != nil {
		return nil, err
	}
	snap := h.buildDeploySnapshot(p, repo, params)
	if deployID > 0 {
		h.saveDeploymentSnapshot(ctx, deployID, snap)
	}
	if deployID > 0 {
		h.markDeploymentDeployPhase(ctx, deployID, imageRef)
	}
	if h.argoEnabled() {
		appName, appURL, err := h.ensureArgoApplication(ctx, p, params.Environment, imageTag)
		if err != nil {
			if deployID > 0 {
				h.markDeploymentFailed(ctx, deployID, "deploy", "ArgoCD apply: "+err.Error())
			}
			return nil, fmt.Errorf("argocd application: %w", err)
		}
		if deployID > 0 {
			_, _ = h.db.Exec(ctx, `
				UPDATE project_deployments SET deploy_status='running', runtime_status='pending',
					runtime_detail=$1, updated_at=now()
				WHERE id=$2`, "ArgoCD application queued: "+appName, deployID)
		}
		result := map[string]any{
			"status":          "accepted",
			"environment":     params.Environment,
			"namespace":       params.Namespace,
			"image":           deployImageRef(params),
			"deployments":     []string{params.PrimaryService().Name},
			"deployment":      params.PrimaryService().Name,
			"deployment_mode": "argocd",
			"argocd_app":      appName,
			"argocd_url":      appURL,
			"runtime":         "argocd-sync",
		}
		if deployID > 0 {
			result["deployment_id"] = deployID
		}
		return result, nil
	}

	if rollback {
		if _, ok := h.latestDeploySnapshotForTag(ctx, p.ID, params.Environment, imageTag); !ok {
			if err := h.validateRollbackImages(ctx, p, params, imageTag); err != nil {
				if deployID > 0 {
					h.markDeploymentFailed(ctx, deployID, "deploy", err.Error())
				}
				return nil, err
			}
		}
	} else if err := h.validateDeployImagesMatchLayout(ctx, p, repo, params, imageTag); err != nil {
		if deployID > 0 {
			h.markDeploymentFailed(ctx, deployID, "deploy", err.Error())
		}
		return nil, err
	}
	if rollback && !rollbackFromMulti {
		_ = h.syncProjectDomainsForEnv(ctx, p, params.Environment, clusterID, &params, 0)
	}
	if params.IsMultiService() {
		h.cleanupLegacySingleApp(ctx, clusterID, params.Namespace)
	}

	if err := h.rancher.EnsureNamespace(ctx, clusterID, params.Namespace); err != nil {
		if deployID > 0 {
			h.markDeploymentFailed(ctx, deployID, "deploy", err.Error())
		}
		return nil, fmt.Errorf("namespace %s: %w", params.Namespace, err)
	}

	if p.RegistryProvider == "harbor" {
		host := harborHostFromURL(h.cfg.HarborURL)
		user, pass, err := h.ensureHarborCIRobot(ctx, p)
		if err != nil {
			return nil, fmt.Errorf("harbor pull secret: %w", err)
		}
		secretJSON, err := deploy.HarborPullSecret(params.Namespace, host, user, pass)
		if err != nil {
			return nil, err
		}
		if err := h.rancher.ApplyNamespacedObject(ctx, clusterID, "/api/v1/secrets", params.Namespace, secretJSON); err != nil {
			if deployID > 0 {
				h.markDeploymentFailed(ctx, deployID, "deploy", err.Error())
			}
			return nil, fmt.Errorf("harbor pull secret: %w", err)
		}
		params.ImagePullSecret = deploy.HarborPullSecretName
	} else {
		user, tok, err := h.ensureGHCRCredentials(ctx, p)
		if err != nil {
			if deployID > 0 {
				h.markDeploymentFailed(ctx, deployID, "deploy", err.Error())
			}
			return nil, fmt.Errorf("ghcr pull secret: %w", err)
		}
		secretJSON, err := deploy.GHCRPullSecret(params.Namespace, user, tok)
		if err != nil {
			return nil, err
		}
		if err := h.rancher.ApplyNamespacedObject(ctx, clusterID, "/api/v1/secrets", params.Namespace, secretJSON); err != nil {
			if deployID > 0 {
				h.markDeploymentFailed(ctx, deployID, "deploy", err.Error())
			}
			return nil, fmt.Errorf("ghcr pull secret: %w", err)
		}
		params.ImagePullSecret = deploy.GHCRPullSecretName
	}
	if err := h.attachAppEnvToDeploy(ctx, p, params.Environment, clusterID); err != nil {
		if deployID > 0 {
			h.markDeploymentFailed(ctx, deployID, "deploy", err.Error())
		}
		return nil, fmt.Errorf("app env: %w", err)
	}
	vars, _ := h.envVarsMap(ctx, p.ID, params.Environment)
	if len(vars) > 0 {
		params.AppEnvFromSecret = deploy.AppEnvSecretName
	}
	manifests, err := deploy.K8sManifests(params)
	if err != nil {
		if deployID > 0 {
			h.markDeploymentFailed(ctx, deployID, "deploy", err.Error())
		}
		return nil, err
	}
	deployedNames := make([]string, 0, len(manifests))
	for _, manifest := range manifests {
		if err := h.rancher.ApplyDeploymentAndService(ctx, clusterID, params.Namespace, manifest.Deployment, manifest.Service); err != nil {
			if deployID > 0 {
				h.markDeploymentFailed(ctx, deployID, "deploy", err.Error())
			}
			return nil, err
		}
		if manifest.ServiceName != "" {
			deployedNames = append(deployedNames, manifest.ServiceName)
		}
	}
	if deployID > 0 {
		_, _ = h.db.Exec(ctx, `UPDATE project_deployments SET deploy_status='success', runtime_status='running', updated_at=now() WHERE id=$1`, deployID)
	}
	domainWarnings := h.syncProjectDomainsForEnv(ctx, p, params.Environment, clusterID, &params, deployID)
	if !params.IsMultiService() {
		h.cleanupClusterMultiWorkloads(ctx, clusterID, params.Namespace)
	} else if rollbackFromMulti {
		h.cleanupMultiServiceWorkloads(ctx, clusterID, params.Namespace, multiServices)
	}
	if deployID > 0 {
		if asyncRuntime {
			go h.finishDeployRuntime(deployID, p, params, imageTag, clusterID)
		} else {
			h.finishDeployRuntimeSync(ctx, deployID, p, params, imageTag, clusterID)
		}
	}
	result := map[string]any{
		"status":      "ok",
		"environment": params.Environment,
		"namespace":   params.Namespace,
		"image":       deployImageRef(params),
		"deployments": deployedNames,
	}
	if len(deployedNames) == 1 {
		result["deployment"] = deployedNames[0]
	} else if len(deployedNames) == 0 {
		result["deployment"] = params.PrimaryService().Name
	}
	if deployID > 0 {
		result["deployment_id"] = deployID
	}
	if asyncRuntime {
		result["status"] = "accepted"
		result["runtime"] = "background"
	}
	if len(domainWarnings) > 0 {
		result["domain_warnings"] = domainWarnings
	}
	return result, nil
}

func (h *Handler) finishDeployRuntime(deployID int64, p projectRow, params deploy.Params, imageTag, clusterID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	h.finishDeployRuntimeSync(ctx, deployID, p, params, imageTag, clusterID)
}

// isSupersededByNewerDeploy kiểm tra có deploy mới hơn deployID trong cùng project/env không.
func (h *Handler) isSupersededByNewerDeploy(ctx context.Context, projectID int64, env string, deployID int64) bool {
	var newerID int64
	_ = h.db.QueryRow(ctx, `
		SELECT id FROM project_deployments
		WHERE project_id=$1 AND environment=$2 AND id > $3
		LIMIT 1`, projectID, env, deployID).Scan(&newerID)
	return newerID > 0
}

func (h *Handler) finishDeployRuntimeSync(ctx context.Context, deployID int64, p projectRow, params deploy.Params, imageTag, clusterID string) {
	if deployID <= 0 || h.rancher == nil || !h.rancher.Enabled() {
		return
	}
	svcs := params.EffectiveServices()
	var lastRollout rancher.DeploymentRolloutStatus
	var waitErr error
	for _, svc := range svcs {
		rollout, err := h.rancher.WaitDeploymentReady(ctx, clusterID, params.Namespace, svc.Name, 120*time.Second)
		lastRollout = rollout
		if err != nil {
			waitErr = err
		}
		if rollout.IsFailed() {
			break
		}
	}
	// Nếu có deploy mới hơn đã được tạo trong khi goroutine này đang chờ pod → dừng lại,
	// không sync Ingress hay đánh dấu failed để tránh ghi đè trạng thái của deploy mới.
	if h.isSupersededByNewerDeploy(ctx, p.ID, params.Environment, deployID) {
		return
	}
	primary := params.PrimaryService()
	pods := h.matchedDeployPods(ctx, p, params.Environment, imageTag, primary.Name)
	podName := pickFirstPodName(pods)
	st, detail, errMsg := evaluateDeploymentRollout(lastRollout)
	if waitErr != nil {
		if st == "failed" {
			if errMsg == "" {
				errMsg = waitErr.Error()
			}
			if detail == "" {
				detail = errMsg
			}
		} else {
			st = "running"
			if detail == "" {
				detail = waitErr.Error()
			}
			errMsg = ""
		}
	}
	if podName != "" && st == "success" {
		detail = strings.TrimSpace(detail + " · pod " + podName)
	}
	if errMsg != "" || st == "failed" {
		msg := errMsg
		if msg == "" {
			msg = detail
		}
		if strings.Contains(msg, "ProgressDeadlineExceeded") {
			msg += " — Gợi ý: rollback về bản deploy thành công trước đó."
		}
		h.markDeploymentFailed(ctx, deployID, "runtime", msg)
		return
	}
	if st == "success" {
		drow := deploymentRowFromParams(params)
		drow.ID = deployID
		drow.ImageTag = imageTag
		drow.PodName = podName
		drow.Environment = params.Environment
		if fleetSt, fleetDetail, fleetErr := h.evaluateFleetRolloutForParams(ctx, p, params, imageTag); fleetSt == "failed" {
			msg := fleetErr
			if msg == "" {
				msg = fleetDetail
			}
			h.markDeploymentFailed(ctx, deployID, "runtime", msg)
			return
		} else if fleetSt == "running" {
			_, _ = h.db.Exec(ctx, `
				UPDATE project_deployments SET deploy_status='success', runtime_status='running', runtime_detail=$1, status='in_progress', updated_at=now()
				WHERE id=$2`, fleetDetail, deployID)
			return
		}
		v := h.assessRuntimeHealth(ctx, p, &drow)
		if v.Status == "failed" {
			h.markDeploymentFailed(ctx, deployID, "runtime", v.ErrorMsg)
		} else if v.Status == "success" {
			if live, detail := h.deploymentTrafficLive(ctx, p, params.Environment, imageTag); !live {
				_, _ = h.db.Exec(ctx, `
					UPDATE project_deployments SET deploy_status='success', runtime_status='running', runtime_detail=$1,
						status='in_progress', updated_at=now() WHERE id=$2`, detail, deployID)
				return
			}
			h.markDeploymentSuccess(ctx, deployID, v.Detail)
		} else {
			_, _ = h.db.Exec(ctx, `
				UPDATE project_deployments SET deploy_status='success', runtime_status=$1, runtime_detail=$2, updated_at=now()
				WHERE id=$3`, v.Status, v.Detail, deployID)
		}
		return
	}
	_, _ = h.db.Exec(ctx, `
		UPDATE project_deployments SET deploy_status='success', runtime_status=$1, runtime_detail=$2, updated_at=now()
		WHERE id=$3`, st, detail, deployID)
}

func deployImageRef(p deploy.Params) string {
	return p.ImageRef()
}
