package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/Thien2026/k8s/services/portal-api/internal/auth"
	"github.com/Thien2026/k8s/services/portal-api/internal/deploy"
	"github.com/Thien2026/k8s/services/portal-api/internal/platformcontract"
	"github.com/go-chi/chi/v5"
)

type envContractContext struct {
	Build    *platformcontract.File
	Runtime  *platformcontract.File
	DockerARGs []string
}

func (h *Handler) loadEnvContracts(ctx context.Context, userID int64, repo projectRepoRow) envContractContext {
	out := envContractContext{}
	if strings.TrimSpace(repo.GitHubOwner) == "" || strings.TrimSpace(repo.GitHubRepo) == "" {
		return out
	}
	ghToken, _, err := h.getGitHubToken(ctx, userID)
	if err != nil || ghToken == "" {
		return out
	}
	branch := strings.TrimSpace(repo.Branch)
	if branch == "" {
		branch = "main"
	}
	client := h.githubClient()
	if raw, ok, err := client.GetFileContent(ctx, ghToken, repo.GitHubOwner, repo.GitHubRepo, platformcontract.BuildContractPath, branch); err == nil && ok {
		if f, err := platformcontract.Parse(raw); err == nil {
			out.Build = &f
		}
	}
	if raw, ok, err := client.GetFileContent(ctx, ghToken, repo.GitHubOwner, repo.GitHubRepo, platformcontract.RuntimeContractPath, branch); err == nil && ok {
		if f, err := platformcontract.Parse(raw); err == nil {
			out.Runtime = &f
		}
	}
	dfPath := strings.TrimSpace(repo.DockerfilePath)
	if dfPath == "" {
		dfPath = "Dockerfile"
	}
	if deploy.NormalizeBuildMode(repo.BuildMode) != "buildpack" {
		if raw, ok, err := client.GetFileContent(ctx, ghToken, repo.GitHubOwner, repo.GitHubRepo, dfPath, branch); err == nil && ok {
			out.DockerARGs = platformcontract.ParseDockerfileARGs(raw)
		}
	}
	return out
}

func contractToJSON(f *platformcontract.File) string {
	if f == nil {
		return ""
	}
	b, err := json.Marshal(f)
	if err != nil {
		return ""
	}
	return string(b)
}

func contractFromJSON(raw string) *platformcontract.File {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var f platformcontract.File
	if err := json.Unmarshal([]byte(raw), &f); err != nil {
		return nil
	}
	if f.Vars == nil {
		f.Vars = map[string]platformcontract.VarSpec{}
	}
	return &f
}

func (h *Handler) persistEnvContracts(ctx context.Context, projectID int64, c envContractContext) error {
	_, err := h.db.Exec(ctx, `
		UPDATE project_repos SET env_contract_build=$1, env_contract_runtime=$2, updated_at=now()
		WHERE project_id=$3`,
		contractToJSON(c.Build), contractToJSON(c.Runtime), projectID)
	return err
}

func contractDefaultValue(key string, p projectRow) string {
	switch key {
	case deploy.ConventionViteAPIBaseKey, deploy.ConventionNextAPIBaseKey:
		return deploy.ConventionAPIBasePath
	case deploy.ConventionAPIRoutePrefixKey:
		return deploy.ConventionAPIBasePath
	case "BUILD_LABEL":
		return p.Slug
	case "APP_GREETING":
		if strings.TrimSpace(p.Name) != "" {
			return "Hello from " + p.Name
		}
		return "Hello from " + p.Slug
	default:
		return p.Slug
	}
}

// ensureContractEnvSeeds — gợi ý giá trị mặc định cho biến required chưa có trên Console (không ghi đè).
func (h *Handler) ensureContractEnvSeeds(ctx context.Context, p projectRow, env string, contract *platformcontract.File, scope string) error {
	if contract == nil {
		return nil
	}
	envs := []string{env}
	if scope == "runtime" {
		envs = []string{"dev", "prod"}
	}
	for _, e := range envs {
		for key, spec := range contract.Vars {
			if !spec.Required {
				continue
			}
			val := contractDefaultValue(key, p)
			if _, err := h.upsertProjectEnvVar(ctx, p.ID, e, scope, key, val, false); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *Handler) syncEnvContractsFromGitHub(ctx context.Context, userID int64, p projectRow) error {
	repo, err := h.getProjectRepo(ctx, p.ID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(repo.GitHubOwner) == "" {
		return nil
	}
	contracts := h.loadEnvContracts(ctx, userID, repo)
	if contracts.Build == nil && contracts.Runtime == nil {
		return nil
	}
	if err := h.persistEnvContracts(ctx, p.ID, contracts); err != nil {
		return err
	}
	env := strings.ToLower(strings.TrimSpace(repo.DeployEnvironment))
	if env == "" {
		env = "dev"
	}
	if contracts.Build != nil {
		if err := h.ensureContractEnvSeeds(ctx, p, env, contracts.Build, "build"); err != nil {
			return err
		}
	}
	if contracts.Runtime != nil {
		if err := h.ensureContractEnvSeeds(ctx, p, env, contracts.Runtime, "runtime"); err != nil {
			return err
		}
	}
	return nil
}

func (h *Handler) reloadEnvContractsIfNeeded(ctx context.Context, p projectRow) error {
	repo, err := h.getProjectRepo(ctx, p.ID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(repo.GitHubOwner) == "" {
		return nil
	}
	if strings.TrimSpace(repo.EnvContractBuild) != "" && strings.TrimSpace(repo.EnvContractRuntime) != "" {
		return nil
	}
	var ownerID int64
	_ = h.db.QueryRow(ctx, `SELECT COALESCE(owner_id, 0) FROM projects WHERE id=$1`, p.ID).Scan(&ownerID)
	if ownerID == 0 {
		return nil
	}
	return h.syncEnvContractsFromGitHub(ctx, ownerID, p)
}

func (h *Handler) effectiveBuildContract(ctx context.Context, projectID int64, repo projectRepoRow) *platformcontract.File {
	base := contractFromJSON(repo.EnvContractBuild)
	if h.getProjectLayout(ctx, projectID) == deploy.LayoutMulti {
		return platformcontract.MergeContracts(base, deploy.DefaultMultiBuildContract())
	}
	return base
}

func (h *Handler) effectiveRuntimeContract(ctx context.Context, projectID int64, repo projectRepoRow) *platformcontract.File {
	base := contractFromJSON(repo.EnvContractRuntime)
	if h.getProjectLayout(ctx, projectID) == deploy.LayoutMulti {
		return platformcontract.MergeContracts(base, deploy.DefaultMultiRuntimeContract())
	}
	return base
}

func (h *Handler) evaluateBuildConfigCached(ctx context.Context, p projectRow, env string) (platformcontract.CheckResult, error) {
	repo, err := h.getProjectRepo(ctx, p.ID)
	if err != nil {
		return platformcontract.CheckResult{}, err
	}
	console, err := h.consoleVarsForScope(ctx, p.ID, env, "build")
	if err != nil {
		return platformcontract.CheckResult{}, err
	}
	return platformcontract.CheckBuild(h.effectiveBuildContract(ctx, p.ID, repo), console, nil), nil
}

func (h *Handler) contractSuggestions(ctx context.Context, p projectRow, env, scope string) []map[string]any {
	repo, err := h.getProjectRepo(ctx, p.ID)
	if err != nil {
		return nil
	}
	var contract *platformcontract.File
	if scope == "build" {
		contract = h.effectiveBuildContract(ctx, p.ID, repo)
	} else {
		contract = h.effectiveRuntimeContract(ctx, p.ID, repo)
	}
	if contract == nil {
		return nil
	}
	console, _ := h.consoleVarsForScope(ctx, p.ID, env, scope)
	cm := map[string]bool{}
	for _, c := range console {
		cm[c.Key] = true
	}
	var suggestions []map[string]any
	for key, spec := range contract.Vars {
		if cm[key] {
			continue
		}
		suggestions = append(suggestions, map[string]any{
			"key":         key,
			"required":    spec.Required,
			"description": spec.Description,
		})
	}
	return suggestions
}

func (h *Handler) projectEnvReadinessPayload(ctx context.Context, p projectRow, env, scope string) map[string]any {
	var check platformcontract.CheckResult
	var err error
	if scope == "build" {
		check, err = h.evaluateBuildConfigCached(ctx, p, env)
	} else {
		check, err = h.evaluateRuntimeConfigCached(ctx, p, env)
	}
	if err != nil || !check.ContractFound {
		return nil
	}
	return map[string]any{
		"environment": env,
		"scope":       scope,
		"ready":       check.Ready,
		"contract":    check,
		"suggestions": h.contractSuggestions(ctx, p, env, scope),
	}
}

func (h *Handler) evaluateRuntimeConfigCached(ctx context.Context, p projectRow, env string) (platformcontract.CheckResult, error) {
	repo, err := h.getProjectRepo(ctx, p.ID)
	if err != nil {
		return platformcontract.CheckResult{}, err
	}
	console, err := h.consoleVarsForScope(ctx, p.ID, env, "runtime")
	if err != nil {
		return platformcontract.CheckResult{}, err
	}
	return platformcontract.CheckRuntime(h.effectiveRuntimeContract(ctx, p.ID, repo), console), nil
}

func (h *Handler) requireDeployEnvReady(ctx context.Context, p projectRow, env string) error {
	_ = h.reloadEnvContractsIfNeeded(ctx, p)
	repo, err := h.getProjectRepo(ctx, p.ID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(repo.GitHubOwner) != "" && strings.TrimSpace(repo.EnvContractBuild) == "" && h.getProjectLayout(ctx, p.ID) != deploy.LayoutMulti {
		return fmt.Errorf("chưa load contract build — Console → Cấu hình app → Đồng bộ workflow GitHub (cần sau khi có .platform/build.yaml)")
	}
	if strings.TrimSpace(repo.GitHubOwner) != "" && strings.TrimSpace(repo.EnvContractRuntime) == "" && h.getProjectLayout(ctx, p.ID) != deploy.LayoutMulti {
		return fmt.Errorf("chưa load contract runtime — Console → Cấu hình app → Đồng bộ workflow GitHub")
	}
	// Build env chỉ cần khi deploy dev (GitHub build). Promote prod tái dùng image dev.
	if env != "prod" {
		if check, err := h.evaluateBuildConfigCached(ctx, p, env); err != nil {
			return err
		} else if msg := platformcontract.FormatCheckError(check); msg != "" {
			return fmt.Errorf("cấu hình build chưa sẵn sàng: %s", msg)
		}
	}
	if check, err := h.evaluateRuntimeConfigCached(ctx, p, env); err != nil {
		return err
	} else if msg := platformcontract.FormatCheckError(check); msg != "" {
		return fmt.Errorf("cấu hình runtime chưa sẵn sàng: %s", msg)
	}
	return nil
}

func (h *Handler) consoleVarsForScope(ctx context.Context, projectID int64, env, scope string) ([]platformcontract.ConsoleVar, error) {
	rows, err := h.listProjectEnvVars(ctx, projectID, env, scope)
	if err != nil {
		return nil, err
	}
	out := make([]platformcontract.ConsoleVar, 0, len(rows))
	for _, r := range rows {
		val := r.Value
		if r.IsSecret {
			var plain string
			if err := h.db.QueryRow(ctx, `SELECT value FROM project_env_vars WHERE id=$1`, r.ID).Scan(&plain); err == nil {
				val = plain
			}
		}
		out = append(out, platformcontract.ConsoleVar{Key: r.Key, Value: val, IsSecret: r.IsSecret})
	}
	return out, nil
}

func (h *Handler) evaluateBuildConfig(ctx context.Context, userID int64, p projectRow, env string) (platformcontract.CheckResult, error) {
	repo, err := h.getProjectRepo(ctx, p.ID)
	if err != nil {
		return platformcontract.CheckResult{}, err
	}
	contracts := h.loadEnvContracts(ctx, userID, repo)
	console, err := h.consoleVarsForScope(ctx, p.ID, env, "build")
	if err != nil {
		return platformcontract.CheckResult{}, err
	}
	return platformcontract.CheckBuild(h.effectiveBuildContract(ctx, p.ID, repo), console, contracts.DockerARGs), nil
}

func (h *Handler) evaluateRuntimeConfig(ctx context.Context, userID int64, p projectRow, env string) (platformcontract.CheckResult, error) {
	repo, err := h.getProjectRepo(ctx, p.ID)
	if err != nil {
		return platformcontract.CheckResult{}, err
	}
	console, err := h.consoleVarsForScope(ctx, p.ID, env, "runtime")
	if err != nil {
		return platformcontract.CheckResult{}, err
	}
	return platformcontract.CheckRuntime(h.effectiveRuntimeContract(ctx, p.ID, repo), console), nil
}

func (h *Handler) requireBuildConfigReady(ctx context.Context, userID int64, p projectRow, env string) error {
	check, err := h.evaluateBuildConfig(ctx, userID, p, env)
	if err != nil {
		return err
	}
	if msg := platformcontract.FormatCheckError(check); msg != "" {
		return fmt.Errorf("cấu hình build chưa sẵn sàng: %s", msg)
	}
	return nil
}

func (h *Handler) requireRuntimeConfigReady(ctx context.Context, userID int64, p projectRow, env string) error {
	check, err := h.evaluateRuntimeConfig(ctx, userID, p, env)
	if err != nil {
		return err
	}
	if msg := platformcontract.FormatCheckError(check); msg != "" {
		return fmt.Errorf("cấu hình runtime chưa sẵn sàng: %s", msg)
	}
	return nil
}

func (h *Handler) contractForSave(ctx context.Context, userID int64, p projectRow, scope string) *platformcontract.File {
	repo, err := h.getProjectRepo(ctx, p.ID)
	if err != nil {
		return nil
	}
	if scope == "build" {
		return h.effectiveBuildContract(ctx, p.ID, repo)
	}
	return h.effectiveRuntimeContract(ctx, p.ID, repo)
}

// GetProjectEnvReadiness GET /projects/{slug}/env/readiness?environment=dev&scope=build|runtime
func (h *Handler) GetProjectEnvReadiness(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	p, ok := h.requireProjectAccess(w, r, slug)
	if !ok {
		return
	}
	u, _ := auth.UserFromContext(r.Context())
	env, ok := validProjectEnv(r.URL.Query().Get("environment"))
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "environment phải là dev hoặc prod"})
		return
	}
	scope := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("scope")))
	if scope == "" {
		scope = "build"
	}
	if scope != "build" && scope != "runtime" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "scope phải là build hoặc runtime"})
		return
	}

	var check platformcontract.CheckResult
	var err error
	if scope == "build" {
		check, err = h.evaluateBuildConfig(r.Context(), u.ID, p, env)
	} else {
		check, err = h.evaluateRuntimeConfig(r.Context(), u.ID, p, env)
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Gợi ý key từ contract chưa có trên Console.
	suggestions := []map[string]any{}
	repo, _ := h.getProjectRepo(r.Context(), p.ID)
	var contract *platformcontract.File
	if scope == "build" {
		contract = h.effectiveBuildContract(r.Context(), p.ID, repo)
	} else {
		contract = h.effectiveRuntimeContract(r.Context(), p.ID, repo)
	}
	if contract != nil {
		console, _ := h.consoleVarsForScope(r.Context(), p.ID, env, scope)
		cm := map[string]bool{}
		for _, c := range console {
			cm[c.Key] = true
		}
		for key, spec := range contract.Vars {
			if cm[key] {
				continue
			}
			suggestions = append(suggestions, map[string]any{
				"key":         key,
				"required":    spec.Required,
				"description": spec.Description,
			})
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"environment": env,
		"scope":       scope,
		"ready":       check.Ready,
		"contract":    check,
		"suggestions": suggestions,
	})
}

// DeployValidateConfigHook — GitHub Actions gọi trước khi build (chặn pipeline nếu thiếu env).
func (h *Handler) DeployValidateConfigHook(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.Header.Get("X-Platform-Deploy-Token"))
	if token == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "thiếu deploy token"})
		return
	}
	var body struct {
		Environment string `json:"environment"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
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
	if strings.TrimSpace(repo.WorkflowSyncedAt) == "" {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
			"error": "Workflow chưa đồng bộ với Console — bấm 「Lưu & đồng bộ GitHub」 trên tab Deploy trước khi build",
		})
		return
	}
	if err := h.requireDeployEnvReady(r.Context(), p, env); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
