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
	"github.com/Thien2026/k8s/services/portal-api/internal/platformcontract"
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
	} else if err := h.requireMultiServiceImages(ctx, p, repo, params, imageTag); err != nil {
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
	if rollbackFromMulti {
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

func (h *Handler) PromoteProjectDeploy(w http.ResponseWriter, r *http.Request) {
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
	if u.Role != auth.RoleAdmin && u.Role != auth.RoleTechLead {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Chỉ admin/tech_lead được promote lên prod"})
		return
	}
	var body struct {
		ImageTag string `json:"image_tag"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "payload không hợp lệ"})
		return
	}
	tag := strings.TrimSpace(body.ImageTag)
	if tag == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "image_tag bắt buộc"})
		return
	}
	var devStatus string
	err := h.db.QueryRow(r.Context(), `
		SELECT status FROM project_deployments
		WHERE project_id=$1 AND environment='dev' AND image_tag=$2
		ORDER BY id DESC LIMIT 1`, p.ID, tag).Scan(&devStatus)
	if err != nil || devStatus != "success" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Chỉ promote bản đã deploy dev thành công (status=success)",
		})
		return
	}
	if !h.ensurePromoteReady(w, r, p) {
		return
	}
	rancherOn, _ := h.plugins.Enabled(r.Context(), plugins.Rancher)
	if !rancherOn || h.rancher == nil || !h.rancher.Enabled() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Rancher chưa sẵn sàng"})
		return
	}
	h.enrichProjectRegistry(r.Context(), &p)
	result, err := h.applyProjectDeploy(r.Context(), p, "prod", tag, "", true, false)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	result["promoted_from"] = "dev"
	result["image_tag"] = tag
	writeJSON(w, http.StatusOK, result)
}

type promoteReadinessItem struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	OK     bool   `json:"ok"`
	Level  string `json:"level,omitempty"` // required | warn
	Group  string `json:"group,omitempty"` // dev_image | prod
	Detail string `json:"detail,omitempty"`
	Tab    string `json:"tab,omitempty"`
}

func (h *Handler) GetPromoteReadiness(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	p, ok := h.requireProjectAccess(w, r, slug)
	if !ok {
		return
	}
	items, ready := h.promoteReadiness(r.Context(), p)
	latestTag := h.latestSuccessfulDeployTag(r.Context(), p.ID, "dev")
	resp := map[string]any{
		"ready":               ready,
		"items":               items,
		"latest_success_tag":  latestTag,
		"can_promote":         latestTag != "",
	}
	if br := h.projectEnvReadinessPayload(r.Context(), p, "dev", "build"); br != nil {
		resp["build_readiness"] = br
	}
	if rr := h.projectEnvReadinessPayload(r.Context(), p, "prod", "runtime"); rr != nil {
		resp["runtime_readiness"] = rr
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) promoteReadiness(ctx context.Context, p projectRow) ([]promoteReadinessItem, bool) {
	var items []promoteReadinessItem
	allOK := true

	markRequired := func(it *promoteReadinessItem) {
		it.Level = "required"
		if !it.OK {
			allOK = false
		}
	}
	markWarn := func(it *promoteReadinessItem) {
		it.Level = "warn"
	}

	nsProd := strings.TrimSpace(p.NamespaceProd)
	nsItem := promoteReadinessItem{
		ID:    "namespace_prod",
		Label: "Namespace prod",
		OK:    nsProd != "",
		Group: "prod",
		Tab:   "settings",
	}
	if nsItem.OK {
		nsItem.Detail = nsProd
		if nsProd == strings.TrimSpace(p.NamespaceDev) {
			nsItem.OK = false
			nsItem.Detail = nsProd + " — trùng namespace dev (nên tách riêng)"
		}
	} else {
		nsItem.Detail = "Chưa cấu hình namespace prod trong Cài đặt project"
	}
	markRequired(&nsItem)
	items = append(items, nsItem)

	devEnv, _ := h.listProjectEnvVars(ctx, p.ID, "dev", "runtime")
	prodEnv, _ := h.listProjectEnvVars(ctx, p.ID, "prod", "runtime")
	devKeys := map[string]bool{}
	for _, row := range devEnv {
		devKeys[row.Key] = true
	}
	var missing []string
	for k := range devKeys {
		found := false
		for _, row := range prodEnv {
			if row.Key == k {
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, k)
		}
	}
	envItem := promoteReadinessItem{
		ID:    "env_vars",
		Label: "Env vars prod",
		Group: "prod",
		Tab:   "env",
	}
	if len(devKeys) == 0 {
		envItem.OK = true
		envItem.Detail = "Dev chưa có env — prod không bắt buộc (nên thêm nếu app cần)"
	} else if len(missing) == 0 {
		envItem.OK = true
		envItem.Detail = fmt.Sprintf("Đủ %d biến giống dev", len(devKeys))
	} else {
		envItem.OK = false
		envItem.Detail = "Thiếu trên prod: " + strings.Join(missing, ", ")
	}
	markRequired(&envItem)
	items = append(items, envItem)

	buildCheck, _ := h.evaluateBuildConfigCached(ctx, p, "dev")
	buildItem := promoteReadinessItem{
		ID:    "build_contract_dev",
		Label: "Build contract (dev)",
		Group: "dev_image",
		Tab:   "env",
	}
	if !buildCheck.ContractFound {
		buildItem.OK = true
		buildItem.Detail = "Chưa có .platform/build.yaml trên GitHub"
	} else if !buildCheck.Ready {
		buildItem.OK = false
		if msg := platformcontract.FormatCheckError(buildCheck); msg != "" {
			buildItem.Detail = msg
		} else {
			buildItem.Detail = "Cấu hình build dev chưa sẵn sàng"
		}
	} else {
		buildItem.OK = true
		buildItem.Detail = "Đủ theo .platform/build.yaml"
	}
	markRequired(&buildItem)
	items = append(items, buildItem)

	rtCheck, _ := h.evaluateRuntimeConfigCached(ctx, p, "prod")
	rtItem := promoteReadinessItem{
		ID:    "runtime_contract",
		Label: "Runtime contract (prod)",
		Group: "prod",
		Tab:   "env",
	}
	if !rtCheck.ContractFound {
		rtItem.OK = true
		rtItem.Detail = "Chưa có .platform/runtime.yaml trên GitHub"
	} else if !rtCheck.Ready {
		rtItem.OK = false
		if msg := platformcontract.FormatCheckError(rtCheck); msg != "" {
			rtItem.Detail = msg
		} else {
			rtItem.Detail = "Cấu hình runtime prod chưa sẵn sàng"
		}
	} else {
		rtItem.OK = true
		rtItem.Detail = "Đủ theo .platform/runtime.yaml"
	}
	markRequired(&rtItem)
	items = append(items, rtItem)

	domains, _ := h.listProjectDomains(ctx, p.ID)
	var prodHosts []string
	for _, d := range domains {
		if strings.EqualFold(d.Environment, "prod") {
			prodHosts = append(prodHosts, d.Hostname)
		}
	}
	domItem := promoteReadinessItem{
		ID:    "domain_prod",
		Label: "Domain prod",
		Group: "prod",
		Tab:   "domains",
	}
	if len(prodHosts) > 0 {
		domItem.OK = true
		domItem.Detail = strings.Join(prodHosts, ", ")
	} else {
		domItem.OK = false
		domItem.Detail = "Chưa có domain prod — thêm tại tab Domains"
	}
	markRequired(&domItem)
	items = append(items, domItem)

	h.enrichProjectRegistry(ctx, &p)
	if strings.EqualFold(strings.TrimSpace(p.RegistryProvider), "harbor") && h.harbor != nil && h.harbor.Enabled() {
		var devTag string
		_ = h.db.QueryRow(ctx, `
			SELECT image_tag FROM project_deployments
			WHERE project_id=$1 AND environment='dev' AND status='success'
			ORDER BY id DESC LIMIT 1`, p.ID).Scan(&devTag)
		scanItem := promoteReadinessItem{
			ID:    "harbor_scan",
			Label: "Bảo mật image (Trivy)",
			Group: "dev_image",
			Tab:   "deploy",
		}
		if devTag == "" {
			scanItem.OK = true
			scanItem.Detail = "Chưa có bản dev success để quét — promote vẫn được nếu đủ điều kiện khác"
		} else {
			projectName := strings.TrimSpace(p.HarborProject)
			if projectName == "" {
				projectName = p.Slug
			}
			ov, err := h.harbor.ArtifactScanOverview(ctx, projectName, "app", devTag)
			if err != nil || ov == nil {
				scanItem.OK = true
				scanItem.Detail = "Chưa đọc được kết quả quét — không chặn promote"
			} else {
				crit := ov.Severity["Critical"]
				high := ov.Severity["High"]
				scanItem.Detail = ov.Detail
				scanItem.OK = crit == 0
				if crit > 0 {
					scanItem.Detail += " — nên sửa base image trước promote (không chặn cứng)"
				} else if high > 0 {
					scanItem.Detail += " — có High, cân nhắc trước prod"
				}
			}
		}
		markWarn(&scanItem)
		items = append(items, scanItem)
	}

	return items, allOK
}

func (h *Handler) ensurePromoteReady(w http.ResponseWriter, r *http.Request, p projectRow) bool {
	items, ready := h.promoteReadiness(r.Context(), p)
	if ready {
		return true
	}
	var hints []string
	for _, it := range items {
		if !it.OK && it.Detail != "" {
			hints = append(hints, it.Label+": "+it.Detail)
		}
	}
	writeJSON(w, http.StatusBadRequest, map[string]any{
		"error": "Chưa đủ cấu hình prod — hoàn tất checklist trước khi promote",
		"items": items,
		"ready": false,
		"hints": hints,
	})
	return false
}

func (h *Handler) latestSuccessfulDeployTag(ctx context.Context, projectID int64, env string) string {
	env = strings.ToLower(strings.TrimSpace(env))
	if env == "" {
		env = "dev"
	}
	var tag string
	_ = h.db.QueryRow(ctx, `
		SELECT image_tag FROM project_deployments
		WHERE project_id=$1 AND environment=$2 AND status='success'
		ORDER BY id DESC LIMIT 1`, projectID, env).Scan(&tag)
	return strings.TrimSpace(tag)
}

func (h *Handler) RollbackProjectDeploy(w http.ResponseWriter, r *http.Request) {
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
		ImageTag    string `json:"image_tag"`
		Environment string `json:"environment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "payload không hợp lệ"})
		return
	}
	tag := strings.TrimSpace(body.ImageTag)
	env := strings.ToLower(strings.TrimSpace(body.Environment))
	if tag == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "image_tag bắt buộc"})
		return
	}
	if env == "" {
		env = "dev"
	}
	if env != "dev" && env != "prod" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "environment phải là dev hoặc prod"})
		return
	}
	if env == "prod" && u.Role != auth.RoleAdmin && u.Role != auth.RoleTechLead {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Chỉ admin/tech_lead được rollback prod"})
		return
	}
	if !h.hasSuccessfulDeployForTag(r.Context(), p.ID, env, tag) {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Chỉ rollback về bản đã từng deploy thành công trong lịch sử",
		})
		return
	}
	rancherOn, _ := h.plugins.Enabled(r.Context(), plugins.Rancher)
	if !rancherOn || h.rancher == nil || !h.rancher.Enabled() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Rancher chưa sẵn sàng"})
		return
	}
	h.enrichProjectRegistry(r.Context(), &p)
	if err := h.validateRollbackLayoutAllowed(r.Context(), p, env, tag); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	result, err := h.applyProjectDeploy(r.Context(), p, env, tag, "", true, true)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	result["rollback"] = true
	result["image_tag"] = tag
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) PatchProjectAutoDeploy(w http.ResponseWriter, r *http.Request) {
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
		Enabled *bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Enabled == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "enabled (boolean) bắt buộc"})
		return
	}
	repo, err := h.getProjectRepo(r.Context(), p.ID)
	if err != nil || repo.WorkflowSyncedAt == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Chưa kết nối GitHub / chưa sync workflow — kết nối repo trước",
		})
		return
	}
	_, err = h.db.Exec(r.Context(), `
		UPDATE project_repos SET auto_deploy_enabled=$1, updated_at=now() WHERE project_id=$2`,
		*body.Enabled, p.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":              "ok",
		"auto_deploy_enabled": *body.Enabled,
		"auto_deploy":         *body.Enabled,
	})
}
