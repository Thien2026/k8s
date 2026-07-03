package github

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

const apiBase = "https://api.github.com"

type Client struct {
	clientID     string
	clientSecret string
	redirectURI  string
	http         *http.Client
}

func NewClient(clientID, clientSecret, redirectURI string) *Client {
	return &Client{
		clientID:     clientID,
		clientSecret: clientSecret,
		redirectURI:  redirectURI,
		http:         &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) Enabled() bool {
	return c.clientID != "" && c.clientSecret != "" && c.redirectURI != ""
}

type TokenResponse struct {
	AccessToken string `json:"access_token"`
	Scope       string `json:"scope"`
	TokenType   string `json:"token_type"`
}

type User struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
}

type Repo struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	FullName      string `json:"full_name"`
	Owner         string `json:"owner"`
	Private       bool   `json:"private"`
	DefaultBranch string `json:"default_branch"`
	HTMLURL       string `json:"html_url"`
}

type repoRaw struct {
	ID            int64 `json:"id"`
	Name          string
	FullName      string `json:"full_name"`
	Owner         struct {
		Login string `json:"login"`
	} `json:"owner"`
	Private       bool   `json:"private"`
	DefaultBranch string `json:"default_branch"`
	HTMLURL       string `json:"html_url"`
}

func (c *Client) AuthorizeURL(state string) string {
	q := url.Values{}
	q.Set("client_id", c.clientID)
	q.Set("redirect_uri", c.redirectURI)
	q.Set("scope", "read:user repo workflow")
	q.Set("state", state)
	return "https://github.com/login/oauth/authorize?" + q.Encode()
}

func (c *Client) ExchangeCode(ctx context.Context, code string) (TokenResponse, error) {
	body := url.Values{}
	body.Set("client_id", c.clientID)
	body.Set("client_secret", c.clientSecret)
	body.Set("code", code)
	body.Set("redirect_uri", c.redirectURI)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://github.com/login/oauth/access_token", strings.NewReader(body.Encode()))
	if err != nil {
		return TokenResponse{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	res, err := c.http.Do(req)
	if err != nil {
		return TokenResponse{}, err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 400 {
		return TokenResponse{}, fmt.Errorf("oauth token %d: %s", res.StatusCode, string(raw))
	}
	var tr TokenResponse
	if err := json.Unmarshal(raw, &tr); err != nil {
		return TokenResponse{}, err
	}
	if tr.AccessToken == "" {
		return TokenResponse{}, fmt.Errorf("oauth không trả access_token")
	}
	return tr, nil
}

func (c *Client) api(ctx context.Context, token, method, path string, payload any) ([]byte, int, error) {
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, 0, err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, apiBase+path, body)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	return raw, res.StatusCode, nil
}

func (c *Client) GetUser(ctx context.Context, token string) (User, error) {
	raw, code, err := c.api(ctx, token, http.MethodGet, "/user", nil)
	if err != nil {
		return User{}, err
	}
	if code >= 400 {
		return User{}, fmt.Errorf("github user %d", code)
	}
	var u User
	if err := json.Unmarshal(raw, &u); err != nil {
		return User{}, err
	}
	return u, nil
}

func (c *Client) ListRepos(ctx context.Context, token string, page int) ([]Repo, error) {
	if page < 1 {
		page = 1
	}
	path := fmt.Sprintf("/user/repos?per_page=100&page=%d&sort=updated", page)
	raw, code, err := c.api(ctx, token, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	if code >= 400 {
		return nil, fmt.Errorf("github repos %d: %s", code, string(raw))
	}
	var list []repoRaw
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, err
	}
	out := make([]Repo, 0, len(list))
	for _, r := range list {
		out = append(out, Repo{
			ID:            r.ID,
			Name:          r.Name,
			FullName:      r.FullName,
			Owner:         r.Owner.Login,
			Private:       r.Private,
			DefaultBranch: r.DefaultBranch,
			HTMLURL:       r.HTMLURL,
		})
	}
	return out, nil
}

// Branch is a git branch on a repository.
type Branch struct {
	Name      string `json:"name"`
	IsDefault bool   `json:"is_default,omitempty"`
}

// ListBranches returns branch names for a repository (up to 100).
func (c *Client) ListBranches(ctx context.Context, token, owner, repo string) ([]Branch, error) {
	owner = strings.TrimSpace(owner)
	repo = strings.TrimSpace(repo)
	if owner == "" || repo == "" {
		return nil, fmt.Errorf("owner và repo bắt buộc")
	}
	defaultBranch := ""
	if info, code, err := c.api(ctx, token, http.MethodGet, fmt.Sprintf("/repos/%s/%s", owner, repo), nil); err == nil && code < 400 {
		var meta struct {
			DefaultBranch string `json:"default_branch"`
		}
		if json.Unmarshal(info, &meta) == nil {
			defaultBranch = strings.TrimSpace(meta.DefaultBranch)
		}
	}
	path := fmt.Sprintf("/repos/%s/%s/branches?per_page=100", owner, repo)
	raw, code, err := c.api(ctx, token, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	if code >= 400 {
		return nil, fmt.Errorf("github branches %d: %s", code, string(raw))
	}
	var list []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, err
	}
	out := make([]Branch, 0, len(list))
	for _, b := range list {
		name := strings.TrimSpace(b.Name)
		if name == "" {
			continue
		}
		out = append(out, Branch{Name: name, IsDefault: defaultBranch != "" && name == defaultBranch})
	}
	return out, nil
}

func (c *Client) contentsAPI(owner, repo, filePath string) string {
	parts := strings.Split(filePath, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return fmt.Sprintf("/repos/%s/%s/contents/%s", owner, repo, strings.Join(parts, "/"))
}

func (c *Client) fileSHA(ctx context.Context, token, owner, repo, path, ref string) (string, error) {
	p := c.contentsAPI(owner, repo, path)
	if ref = strings.TrimSpace(ref); ref != "" {
		p += "?ref=" + url.QueryEscape(ref)
	}
	raw, code, err := c.api(ctx, token, http.MethodGet, p, nil)
	if code == 404 {
		return "", nil
	}
	if err != nil || code >= 400 {
		return "", fmt.Errorf("get file %d", code)
	}
	var meta struct {
		SHA string `json:"sha"`
	}
	if json.Unmarshal(raw, &meta) != nil {
		return "", nil
	}
	return meta.SHA, nil
}

// GetFileContent đọc file text từ repo (ref = branch/tag/commit, rỗng = default branch).
func (c *Client) GetFileContent(ctx context.Context, token, owner, repo, path, ref string) (content string, found bool, err error) {
	p := c.contentsAPI(owner, repo, path)
	if ref = strings.TrimSpace(ref); ref != "" {
		p += "?ref=" + url.QueryEscape(ref)
	}
	raw, code, err := c.api(ctx, token, http.MethodGet, p, nil)
	if code == 404 {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if code >= 400 {
		return "", false, fmt.Errorf("get file %d: %s", code, string(raw))
	}
	var meta struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return "", false, err
	}
	if meta.Encoding != "base64" {
		return "", false, fmt.Errorf("encoding không hỗ trợ: %s", meta.Encoding)
	}
	dec, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(meta.Content, "\n", ""))
	if err != nil {
		return "", false, err
	}
	return string(dec), true, nil
}

// ContentEntry — mục trong thư mục GitHub (file hoặc dir).
type ContentEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Type string `json:"type"`
}

// ListRepoContents liệt kê file/thư mục con (path rỗng = root repo).
func (c *Client) ListRepoContents(ctx context.Context, token, owner, repo, path, ref string) ([]ContentEntry, error) {
	p := c.contentsAPI(owner, repo, strings.Trim(path, "/"))
	if ref = strings.TrimSpace(ref); ref != "" {
		p += "?ref=" + url.QueryEscape(ref)
	}
	raw, code, err := c.api(ctx, token, http.MethodGet, p, nil)
	if code == 404 {
		return nil, fmt.Errorf("không tìm thấy path %q trên branch", path)
	}
	if err != nil {
		return nil, err
	}
	if code >= 400 {
		return nil, fmt.Errorf("list contents %d: %s", code, string(raw))
	}
	var list []ContentEntry
	if err := json.Unmarshal(raw, &list); err == nil {
		return list, nil
	}
	var one ContentEntry
	if err := json.Unmarshal(raw, &one); err == nil && one.Path != "" {
		return []ContentEntry{one}, nil
	}
	return nil, fmt.Errorf("phản hồi GitHub không hợp lệ")
}

// RepoPathExists kiểm tra path tồn tại trên ref (branch/commit).
func (c *Client) RepoPathExists(ctx context.Context, token, owner, repo, path, ref string) (bool, error) {
	path = strings.Trim(path, "/")
	if path == "" || path == "." {
		return true, nil
	}
	p := c.contentsAPI(owner, repo, path)
	if ref = strings.TrimSpace(ref); ref != "" {
		p += "?ref=" + url.QueryEscape(ref)
	}
	_, code, err := c.api(ctx, token, http.MethodGet, p, nil)
	if code == 404 {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if code >= 400 {
		return false, fmt.Errorf("check path %d", code)
	}
	return true, nil
}

func (c *Client) PutWorkflowFile(ctx context.Context, token, owner, repo, path, message, content, branch string) error {
	branch = strings.TrimSpace(branch)
	sha, _ := c.fileSHA(ctx, token, owner, repo, path, branch)
	payload := map[string]string{
		"message": message,
		"content": base64.StdEncoding.EncodeToString([]byte(content)),
	}
	if sha != "" {
		payload["sha"] = sha
	}
	if branch != "" {
		payload["branch"] = branch
	}
	p := c.contentsAPI(owner, repo, path)
	raw, code, err := c.api(ctx, token, http.MethodPut, p, payload)
	if err != nil {
		return err
	}
	if code >= 400 {
		return fmt.Errorf("put workflow %d: %s", code, string(raw))
	}
	return nil
}

// DispatchWorkflow triggers workflow_dispatch on the given ref (branch/tag).
// workflowFile may be a full path (.github/workflows/foo.yml); GitHub API expects the file name only.
func (c *Client) DispatchWorkflow(ctx context.Context, token, owner, repo, workflowFile, ref string) error {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		ref = "main"
	}
	wfID := filepath.Base(strings.TrimSpace(workflowFile))
	if wfID == "" {
		return fmt.Errorf("workflow file name trống")
	}
	path := fmt.Sprintf("/repos/%s/%s/actions/workflows/%s/dispatches", owner, repo, url.PathEscape(wfID))
	body := map[string]string{"ref": ref}
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		if attempt > 0 {
			wait := time.Duration(attempt) * 2 * time.Second
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
		}
		raw, code, err := c.api(ctx, token, http.MethodPost, path, body)
		if err != nil {
			return err
		}
		if code < 400 {
			return nil
		}
		lastErr = fmt.Errorf("dispatch workflow %d: %s", code, string(raw))
		if code != 404 {
			return lastErr
		}
	}
	return lastErr
}

func (c *Client) SetActionsSecret(ctx context.Context, token, owner, repo, name, plaintext string) error {
	raw, code, err := c.api(ctx, token, http.MethodGet, fmt.Sprintf("/repos/%s/%s/actions/secrets/public-key", owner, repo), nil)
	if err != nil {
		return err
	}
	if code >= 400 {
		return fmt.Errorf("public-key %d: %s", code, string(raw))
	}
	var pk struct {
		KeyID string `json:"key_id"`
		Key   string `json:"key"`
	}
	if json.Unmarshal(raw, &pk) != nil || pk.Key == "" {
		return fmt.Errorf("public-key không hợp lệ")
	}
	enc, err := encryptSecret(plaintext, pk.Key)
	if err != nil {
		return err
	}
	body := map[string]string{
		"encrypted_value": enc,
		"key_id":          pk.KeyID,
	}
	path := fmt.Sprintf("/repos/%s/%s/actions/secrets/%s", owner, repo, name)
	raw, code, err = c.api(ctx, token, http.MethodPut, path, body)
	if err != nil {
		return err
	}
	if code >= 400 {
		return fmt.Errorf("set secret %d: %s", code, string(raw))
	}
	return nil
}
