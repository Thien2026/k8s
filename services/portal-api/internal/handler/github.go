package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"

	"github.com/Thien2026/k8s/services/portal-api/internal/auth"
	gh "github.com/Thien2026/k8s/services/portal-api/internal/github"
	"github.com/go-chi/chi/v5"
)

func (h *Handler) githubClient() *gh.Client {
	return gh.NewClient(h.cfg.GitHubClientID, h.cfg.GitHubClientSecret, h.cfg.GitHubRedirectURI)
}

func (h *Handler) saveGitHubToken(ctx context.Context, userID int64, u gh.User, token, scope string) error {
	_, err := h.db.Exec(ctx, `
		INSERT INTO user_github_tokens (user_id, github_user_id, github_login, access_token, token_scope, updated_at)
		VALUES ($1, $2, $3, $4, $5, now())
		ON CONFLICT (user_id) DO UPDATE SET
			github_user_id = EXCLUDED.github_user_id,
			github_login = EXCLUDED.github_login,
			access_token = EXCLUDED.access_token,
			token_scope = EXCLUDED.token_scope,
			updated_at = now()`,
		userID, u.ID, u.Login, token, scope)
	return err
}

func (h *Handler) getGitHubToken(ctx context.Context, userID int64) (string, string, error) {
	var token, login string
	err := h.db.QueryRow(ctx,
		`SELECT access_token, github_login FROM user_github_tokens WHERE user_id=$1`, userID).Scan(&token, &login)
	return token, login, err
}

func (h *Handler) GitHubStatus(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	client := h.githubClient()
	out := map[string]any{
		"enabled":   client.Enabled(),
		"connected": false,
	}
	if client.Enabled() {
		if _, login, err := h.getGitHubToken(r.Context(), u.ID); err == nil && login != "" {
			out["connected"] = true
			out["login"] = login
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) GitHubOAuthStart(w http.ResponseWriter, r *http.Request) {
	client := h.githubClient()
	if !client.Enabled() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "GitHub OAuth chưa cấu hình — thêm GITHUB_CLIENT_ID/SECRET trên VPS",
		})
		return
	}
	state, _ := randomHex(16)
	ret := strings.TrimSpace(r.URL.Query().Get("return"))
	if ret == "" {
		ret = "/#/my-projects"
	}
	popup := strings.TrimSpace(r.URL.Query().Get("popup")) == "1"
	http.SetCookie(w, &http.Cookie{
		Name:     "gh_oauth_state",
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   600,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "gh_oauth_return",
		Value:    ret,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   600,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "gh_oauth_popup",
		Value:    map[bool]string{true: "1", false: "0"}[popup],
		Path:     "/",
		HttpOnly: true,
		Secure:   h.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   600,
	})
	http.Redirect(w, r, client.AuthorizeURL(state), http.StatusFound)
}

func (h *Handler) GitHubOAuthCallback(w http.ResponseWriter, r *http.Request) {
	client := h.githubClient()
	stateCookie, _ := r.Cookie("gh_oauth_state")
	returnCookie, _ := r.Cookie("gh_oauth_return")
	popupCookie, _ := r.Cookie("gh_oauth_popup")
	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")
	ret := "/#/my-projects"
	popup := popupCookie != nil && popupCookie.Value == "1"
	if returnCookie != nil && returnCookie.Value != "" {
		ret = returnCookie.Value
	}
	if stateCookie == nil || stateCookie.Value == "" || stateCookie.Value != state || code == "" {
		h.finishGitHubOAuth(w, r, popup, ret, "error")
		return
	}

	rt, err := r.Cookie(auth.CookieRefresh)
	if err != nil || rt.Value == "" {
		h.finishGitHubOAuth(w, r, popup, ret, "login_required")
		return
	}
	userID, err := h.auth.GetSessionUserID(r.Context(), auth.HashToken(rt.Value))
	if err != nil {
		h.finishGitHubOAuth(w, r, popup, ret, "login_required")
		return
	}

	tr, err := client.ExchangeCode(r.Context(), code)
	if err != nil {
		h.finishGitHubOAuth(w, r, popup, ret, "error")
		return
	}
	ghUser, err := client.GetUser(r.Context(), tr.AccessToken)
	if err != nil {
		h.finishGitHubOAuth(w, r, popup, ret, "error")
		return
	}
	_ = h.saveGitHubToken(r.Context(), userID, ghUser, tr.AccessToken, tr.Scope)
	h.finishGitHubOAuth(w, r, popup, ret, "connected")
}

func (h *Handler) finishGitHubOAuth(w http.ResponseWriter, r *http.Request, popup bool, ret, status string) {
	for _, name := range []string{"gh_oauth_state", "gh_oauth_return", "gh_oauth_popup"} {
		http.SetCookie(w, &http.Cookie{
			Name:     name,
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			Secure:   h.cfg.CookieSecure,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   -1,
		})
	}
	if popup {
		target := ret + "?github=" + status
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, `<!doctype html><html><body><script>
const payload = { type: "github_oauth", status: %q, target: %q };
try {
  if (window.opener && !window.opener.closed) window.opener.postMessage(payload, window.location.origin);
} catch (_) {}
window.close();
setTimeout(function(){ window.location.href = payload.target; }, 300);
</script></body></html>`, status, target)
		return
	}
	http.Redirect(w, r, ret+"?github="+status, http.StatusFound)
}

func (h *Handler) GitHubListRepos(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	token, _, err := h.getGitHubToken(r.Context(), u.ID)
	if err != nil || token == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Chưa kết nối GitHub — bấm Kết nối GitHub trước"})
		return
	}
	page := 1
	if p := r.URL.Query().Get("page"); p != "" {
		fmt.Sscanf(p, "%d", &page)
	}
	list, err := h.githubClient().ListRepos(r.Context(), token, page)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list})
}

func (h *Handler) GitHubListBranches(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	token, _, err := h.getGitHubToken(r.Context(), u.ID)
	if err != nil || token == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Chưa kết nối GitHub — bấm Kết nối GitHub trước"})
		return
	}
	owner := strings.TrimSpace(chi.URLParam(r, "owner"))
	repo := strings.TrimSpace(chi.URLParam(r, "repo"))
	if owner == "" || repo == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "owner và repo bắt buộc"})
		return
	}
	list, err := h.githubClient().ListBranches(r.Context(), token, owner, repo)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list})
}

func (h *Handler) GitHubDisconnect(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	_, _ = h.db.Exec(r.Context(), `DELETE FROM user_github_tokens WHERE user_id=$1`, u.ID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (h *Handler) ensureDeployHookToken(ctx context.Context, projectID int64) (string, error) {
	var token string
	err := h.db.QueryRow(ctx,
		`SELECT deploy_hook_token FROM project_repos WHERE project_id=$1`, projectID).Scan(&token)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(token) != "" {
		return token, nil
	}
	token, err = randomHex(24)
	if err != nil {
		return "", err
	}
	_, err = h.db.Exec(ctx,
		`UPDATE project_repos SET deploy_hook_token=$1 WHERE project_id=$2`, token, projectID)
	return token, err
}

func (h *Handler) platformDeployHookURL() string {
	base := strings.TrimRight(h.cfg.PlatformPublicURL, "/")
	if base == "" {
		base = strings.TrimRight(h.cfg.CORSOrigin, "/")
	}
	return base + "/api/v1/hooks/deploy"
}

// ensureHarborCIRobot lấy hoặc tạo robot Harbor; credentials chỉ platform giữ, inject vào GitHub Secrets.
func (h *Handler) ensureHarborCIRobot(ctx context.Context, p projectRow) (username, secret string, err error) {
	if h.harbor == nil || !h.harbor.Enabled() {
		return "", "", fmt.Errorf("Harbor chưa sẵn sàng")
	}
	projectName := strings.TrimSpace(p.HarborProject)
	if projectName == "" {
		projectName = p.Slug
	}
	if err := h.harbor.EnsureProject(ctx, projectName); err != nil {
		return "", "", err
	}

	var storedName, storedSecret string
	_ = h.db.QueryRow(ctx,
		`SELECT COALESCE(harbor_robot_name,''), COALESCE(harbor_robot_secret,'') FROM projects WHERE id=$1`,
		p.ID).Scan(&storedName, &storedSecret)
	if strings.TrimSpace(storedName) != "" && strings.TrimSpace(storedSecret) != "" {
		return storedName, storedSecret, nil
	}

	username, secret, err = h.harbor.EnsureCIRobot(ctx, projectName)
	if err != nil {
		return "", "", err
	}
	_, err = h.db.Exec(ctx,
		`UPDATE projects SET harbor_robot_name=$1, harbor_robot_secret=$2, updated_at=now() WHERE id=$3`,
		username, secret, p.ID)
	return username, secret, err
}

func (h *Handler) ghcrPullConfigured(ctx context.Context, projectID int64) bool {
	var user, token string
	_ = h.db.QueryRow(ctx,
		`SELECT COALESCE(ghcr_pull_user,''), COALESCE(ghcr_pull_token,'') FROM projects WHERE id=$1`,
		projectID).Scan(&user, &token)
	if strings.TrimSpace(user) != "" && strings.TrimSpace(token) != "" {
		return true
	}
	return strings.TrimSpace(h.cfg.GHCRPullToken) != "" && strings.TrimSpace(h.cfg.GHCRPullUser) != ""
}

// ensureGHCRCredentials lấy hoặc lưu PAT pull ghcr.io cho K8s imagePullSecret.
func (h *Handler) ensureGHCRCredentials(ctx context.Context, p projectRow) (username, token string, err error) {
	var storedUser, storedToken string
	_ = h.db.QueryRow(ctx,
		`SELECT COALESCE(ghcr_pull_user,''), COALESCE(ghcr_pull_token,'') FROM projects WHERE id=$1`,
		p.ID).Scan(&storedUser, &storedToken)
	if strings.TrimSpace(storedUser) != "" && strings.TrimSpace(storedToken) != "" {
		return storedUser, storedToken, nil
	}

	user := strings.TrimSpace(h.cfg.GHCRPullUser)
	tok := strings.TrimSpace(h.cfg.GHCRPullToken)
	if user == "" || tok == "" {
		return "", "", fmt.Errorf("GHCR pull chưa cấu hình — đặt GHCR_PULL_TOKEN (+ GHCR_PULL_USER) trên VPS (PAT scope read:packages)")
	}

	_, err = h.db.Exec(ctx,
		`UPDATE projects SET ghcr_pull_user=$1, ghcr_pull_token=$2, updated_at=now() WHERE id=$3`,
		user, tok, p.ID)
	return user, tok, err
}

func (h *Handler) getProjectByDeployToken(ctx context.Context, token string) (projectRow, error) {
	var p projectRow
	err := h.db.QueryRow(ctx, `
		SELECT p.id, p.name, p.slug, COALESCE(p.description,''), p.namespace_dev, p.namespace_prod,
		       COALESCE(p.harbor_project,''), COALESCE(p.registry_provider,'ghcr')
		FROM projects p
		INNER JOIN project_repos r ON r.project_id = p.id
		WHERE r.deploy_hook_token = $1`, token).Scan(
		&p.ID, &p.Name, &p.Slug, &p.Description, &p.NamespaceDev, &p.NamespaceProd,
		&p.HarborProject, &p.RegistryProvider,
	)
	return p, err
}
