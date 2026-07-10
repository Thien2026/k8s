package handler

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/Thien2026/k8s/services/portal-api/internal/auth"
	"github.com/Thien2026/k8s/services/portal-api/internal/deploy"
	"github.com/Thien2026/k8s/services/portal-api/internal/plugins"
	"github.com/go-chi/chi/v5"
)

// DeployHook nhận webhook từ GitHub Actions sau khi build image.
func (h *Handler) DeployHook(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.Header.Get("X-Platform-Deploy-Token"))
	if token == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "thiếu deploy token"})
		return
	}
	var body struct {
		ImageTag    string `json:"image_tag"`
		Environment string `json:"environment"`
		ClusterID   string `json:"cluster_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "payload không hợp lệ"})
		return
	}
	tag := strings.TrimSpace(body.ImageTag)
	if tag == "" {
		tag = "latest"
	}
	env := strings.ToLower(strings.TrimSpace(body.Environment))
	if env == "" {
		env = "dev"
	}

	p, err := h.getProjectByDeployToken(r.Context(), token)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "token không hợp lệ"})
		return
	}

	repo, _ := h.getProjectRepo(r.Context(), p.ID)
	if err := h.requireWorkflowReady(r.Context(), p.ID, repo); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	if !repo.AutoDeployEnabled {
		skipReason := "auto_deploy tắt — image đã build, không deploy lên cluster"
		h.markDeploymentDeploySkipped(r.Context(), p.ID, env, tag, skipReason)
		writeJSON(w, http.StatusOK, map[string]any{
			"status":      "skipped",
			"reason":      skipReason,
			"image_tag":   tag,
			"environment": env,
			"project":     p.Slug,
		})
		return
	}

	rancherOn, _ := h.plugins.Enabled(r.Context(), plugins.Rancher)
	if !rancherOn || h.rancher == nil || !h.rancher.Enabled() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Rancher chưa sẵn sàng"})
		return
	}

	h.enrichProjectRegistry(r.Context(), &p)
	if err := h.requireDeployEnvReady(r.Context(), p, env); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}

	pSlug := p.Slug
	clusterID := body.ClusterID
	deployCtx := context.WithoutCancel(r.Context())
	go func() {
		result, err := h.applyProjectDeploy(deployCtx, p, env, tag, clusterID, true, false)
		if err != nil {
			log.Printf("deploy hook async project=%s env=%s tag=%s: %v", pSlug, env, tag, err)
			return
		}
		_ = result
	}()

	writeJSON(w, http.StatusAccepted, map[string]any{
		"status":      "accepted",
		"message":     "Deploy đã nhận — đang đồng bộ GitOps và cluster",
		"image_tag":   tag,
		"environment": env,
		"project":     p.Slug,
	})
}

func (h *Handler) ProjectGitHubSetup(w http.ResponseWriter, r *http.Request) {
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
	ghToken, _, err := h.getGitHubToken(r.Context(), u.ID)
	if err != nil || ghToken == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Chưa kết nối GitHub"})
		return
	}

	var body struct {
		Owner             string              `json:"owner"`
		Repo              string              `json:"repo"`
		Branch            string              `json:"branch"`
		Environment       string              `json:"environment"`
		Layout            string              `json:"layout,omitempty"`
		Services          []projectServiceRow `json:"services,omitempty"`
		ApplyRepoContract bool                `json:"apply_repo_contract,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "payload không hợp lệ"})
		return
	}
	owner := strings.TrimSpace(body.Owner)
	repo := strings.TrimSpace(body.Repo)
	if owner == "" || repo == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "owner và repo bắt buộc"})
		return
	}
	branch := strings.TrimSpace(body.Branch)
	if branch == "" {
		branch = "main"
	}
	env := strings.ToLower(strings.TrimSpace(body.Environment))
	if env == "" {
		env = "dev"
	}
	if env != "dev" && env != "prod" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "environment phải là dev hoặc prod"})
		return
	}

	layoutFromBody := strings.TrimSpace(body.Layout) != ""
	if body.ApplyRepoContract {
		if _, _, err := h.applyServicesContractFromRepo(r.Context(), u.ID, p, owner, repo, branch); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
	} else if layoutFromBody {
		layout := deploy.NormalizeLayout(body.Layout)
		if err := h.validateProjectServicesLayout(r.Context(), u.ID, owner, repo, branch, layout, body.Services); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if err := h.persistProjectServices(r.Context(), p.ID, layout, body.Services); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		h.invalidateProjectWorkflow(r.Context(), p.ID)
		if layout == deploy.LayoutMulti {
			_, _ = h.ensureBackFrontConventions(r.Context(), p.ID)
		}
	}

	if h.getProjectLayout(r.Context(), p.ID) == deploy.LayoutMulti {
		svcRows, err := h.listProjectServices(r.Context(), p.ID)
		if err == nil && len(svcRows) >= 2 {
			if err := h.validateMultiServicePaths(r.Context(), u.ID, owner, repo, branch, svcRows); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
		}
	}

	deployToken, err := h.ensureDeployHookToken(r.Context(), p.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	gitURL := "https://github.com/" + owner + "/" + repo
	_, err = h.db.Exec(r.Context(), `
		UPDATE project_repos SET
			git_url=$1, branch=$2, github_owner=$3, github_repo=$4,
			deploy_environment=$5, updated_at=now()
		WHERE project_id=$6`,
		gitURL, branch, owner, repo, env, p.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	repoRow, _ := h.getProjectRepo(r.Context(), p.ID)
	if !body.ApplyRepoContract && !layoutFromBody {
		h.maybeApplyServicesContractOnSetup(r.Context(), u.ID, p, repoRow, branch)
	}
	h.syncGitSubmodulesFromContract(r.Context(), u.ID, p.ID, repoRow, branch)
	repoRow, _ = h.getProjectRepo(r.Context(), p.ID)
	h.enrichProjectRegistry(r.Context(), &p)
	if err := h.resolveBuildMode(r.Context(), u.ID, p.ID, &repoRow); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "không quét được repo: " + err.Error()})
		return
	}
	if err := h.syncEnvContractsFromGitHub(r.Context(), u.ID, p); err != nil {
		log.Printf("github setup env contracts failed project=%s err=%v", p.Slug, err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "không đồng bộ contract env: " + err.Error()})
		return
	}
	repoRow, _ = h.getProjectRepo(r.Context(), p.ID)
	params := h.buildDeployParams(r.Context(), p, repoRow, env, "", true)
	wf := deploy.GitHubWorkflow(params)

	client := h.githubClient()
	if err := client.PutWorkflowFile(r.Context(), ghToken, owner, repo, wf.Filename,
		"chore(platform): sync deploy workflow for "+p.Slug, wf.Content, branch); err != nil {
		log.Printf("github setup workflow failed project=%s repo=%s/%s branch=%s err=%v", p.Slug, owner, repo, branch, err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "không push workflow: " + err.Error()})
		return
	}
	if err := h.syncProjectServicesYAMLToRepo(r.Context(), ghToken, owner, repo, branch, p.ID, repoRow); err != nil {
		log.Printf("github setup services.yaml failed project=%s err=%v", p.Slug, err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "không push services.yaml: " + err.Error()})
		return
	}
	deploySecret := deploy.DeployTokenSecretName(p.Slug)
	if err := client.SetActionsSecret(r.Context(), ghToken, owner, repo, deploySecret, deployToken); err != nil {
		log.Printf("github setup secret failed project=%s repo=%s/%s err=%v", p.Slug, owner, repo, err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "không tạo secret: " + err.Error()})
		return
	}

	secretsProvisioned := []string{deploySecret}
	gitOpsCfg := h.loadGitOpsConfig(r.Context())
	if gitOpsCfg.PushToken != "" && gitOpsCfg.RepoURL != "" {
		gitOpsSecret := deploy.GitOpsTokenSecretName()
		if err := client.SetActionsSecret(r.Context(), ghToken, owner, repo, gitOpsSecret, gitOpsCfg.PushToken); err != nil {
			log.Printf("github gitops token secret failed project=%s err=%v", p.Slug, err)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "không tạo " + gitOpsSecret + ": " + err.Error()})
			return
		}
		secretsProvisioned = append(secretsProvisioned, gitOpsSecret)
	}
	if p.RegistryProvider == "harbor" {
		harborUser, harborPass, err := h.ensureHarborCIRobot(r.Context(), p)
		if err != nil {
			log.Printf("harbor ci robot failed project=%s err=%v", p.Slug, err)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "không tạo Harbor CI robot: " + err.Error()})
			return
		}
		harborUserSecret := deploy.HarborUsernameSecretName(p.Slug)
		harborPassSecret := deploy.HarborPasswordSecretName(p.Slug)
		if err := client.SetActionsSecret(r.Context(), ghToken, owner, repo, harborUserSecret, harborUser); err != nil {
			log.Printf("github harbor username secret failed project=%s err=%v", p.Slug, err)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "không tạo " + harborUserSecret + ": " + err.Error()})
			return
		}
		if err := client.SetActionsSecret(r.Context(), ghToken, owner, repo, harborPassSecret, harborPass); err != nil {
			log.Printf("github harbor password secret failed project=%s err=%v", p.Slug, err)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "không tạo " + harborPassSecret + ": " + err.Error()})
			return
		}
		secretsProvisioned = append(secretsProvisioned, harborUserSecret, harborPassSecret)
	} else {
		if _, _, err := h.ensureGHCRCredentials(r.Context(), p); err != nil {
			log.Printf("ghcr pull creds failed project=%s err=%v", p.Slug, err)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
	}

	dispatchErr := client.DispatchWorkflow(r.Context(), ghToken, owner, repo, wf.Filename, branch)
	if dispatchErr != nil {
		log.Printf("github dispatch workflow failed project=%s repo=%s/%s branch=%s err=%v", p.Slug, owner, repo, branch, dispatchErr)
	}

	repoRow, _ = h.getProjectRepo(r.Context(), p.ID)
	h.markWorkflowSynced(r.Context(), p.ID, repoRow)

	resp := map[string]any{
		"status":                   "ok",
		"git_url":                  gitURL,
		"layout":                   h.getProjectLayout(r.Context(), p.ID),
		"build_mode":               repoRow.BuildMode,
		"build_mode_detected_path": repoRow.BuildModeDetectedPath,
		"workflow_file":            wf.Filename,
		"workflow_url":             "https://github.com/" + owner + "/" + repo + "/actions",
		"auto_deploy":              true,
		"deploy_environment":       env,
		"secrets_provisioned":      secretsProvisioned,
		"workflow_dispatched":      dispatchErr == nil,
		"dev_action_required":      false,
	}
	if dispatchErr != nil {
		resp["dispatch_warning"] = "Workflow đã push và secrets đã cấp — build sẽ chạy khi push code hoặc trigger thủ công trên GitHub Actions."
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) buildDeployParams(ctx context.Context, p projectRow, repo projectRepoRow, env, imageTag string, includeHook bool) deploy.Params {
	harborHost := ""
	if p.RegistryProvider == "harbor" && h.cfg.HarborURL != "" {
		harborHost = harborHostFromURL(h.cfg.HarborURL)
	}
	deployEnv := strings.ToLower(strings.TrimSpace(env))
	if deployEnv != "dev" && deployEnv != "prod" {
		deployEnv = ""
	}
	if deployEnv == "" {
		deployEnv = strings.ToLower(strings.TrimSpace(repo.DeployEnvironment))
	}
	if deployEnv == "" {
		deployEnv = "dev"
	}
	ns := p.NamespaceDev
	if deployEnv == "prod" {
		ns = p.NamespaceProd
	}
	params := deploy.Params{
		ProjectSlug:      p.Slug,
		ProjectName:      p.Name,
		Namespace:        ns,
		Environment:      deployEnv,
		RegistryProvider: p.RegistryProvider,
		Registry:         p.Registry,
		GitURL:           repo.GitURL,
		Branch:           repo.Branch,
		GitSubmodules:    repo.GitSubmodules,
		BuildMode:        repo.BuildMode,
		DockerfilePath:   repo.DockerfilePath,
		BuildContext:     repo.BuildContext,
		ImageTag:         imageTag,
		HarborHost:       harborHost,
	}
	gitOps := h.loadGitOpsConfig(ctx)
	params.GitOpsRepoURL = gitOps.RepoURL
	params.GitOpsRepoBranch = gitOps.RepoBranch
	params.GitOpsBasePath = gitOps.BasePath
	services, layout := h.loadDeployServices(ctx, p.ID, repo)
	params.Layout = layout
	params.Services = services
	if includeHook {
		params.DeployHookURL = h.platformDeployHookURL()
		params.DeployEnvironment = deployEnv
	}
	params.DeployTokenSecret = deploy.DeployTokenSecretName(p.Slug)
	params.GitOpsTokenSecret = deploy.GitOpsTokenSecretName()
	params.HarborUserSecret = deploy.HarborUsernameSecretName(p.Slug)
	params.HarborPassSecret = deploy.HarborPasswordSecretName(p.Slug)
	if includeHook && ctx != nil {
		if args, err := h.loadBuildArgs(ctx, p.ID, deployEnv, p.Slug); err == nil {
			params.BuildArgs = args
		}
	}
	return params
}

func harborHostFromURL(raw string) string {
	u, err := url.Parse(strings.TrimRight(raw, "/"))
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Host
}
