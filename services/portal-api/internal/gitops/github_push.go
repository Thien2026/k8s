package gitops

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client pushes files to a GitHub repo using a PAT.
type Client struct {
	http *http.Client
}

func NewClient() *Client {
	return &Client{http: &http.Client{Timeout: 45 * time.Second}}
}

func (c *Client) RepoReachable(ctx context.Context, token string, ref RepoRef) error {
	_, code, err := c.api(ctx, token, http.MethodGet, fmt.Sprintf("/repos/%s/%s", ref.Owner, ref.Name), nil)
	if err != nil {
		return err
	}
	if code == 404 {
		return fmt.Errorf("không tìm thấy repo %s/%s", ref.Owner, ref.Name)
	}
	if code >= 400 {
		return fmt.Errorf("github repo %d", code)
	}
	return nil
}

func (c *Client) GetFile(ctx context.Context, token string, ref RepoRef, path, branch string) (content string, err error) {
	path = strings.Trim(path, "/")
	p := fmt.Sprintf("/repos/%s/%s/contents/%s", ref.Owner, ref.Name, escapePath(path))
	if branch = strings.TrimSpace(branch); branch != "" {
		p += "?ref=" + urlQueryEscape(branch)
	}
	raw, code, err := c.api(ctx, token, http.MethodGet, p, nil)
	if code == 404 {
		return "", fmt.Errorf("file not found: %s", path)
	}
	if err != nil || code >= 400 {
		return "", fmt.Errorf("get file %d", code)
	}
	var meta struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return "", err
	}
	if strings.EqualFold(meta.Encoding, "base64") {
		b, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(meta.Content, "\n", ""))
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
	return meta.Content, nil
}

func (c *Client) PutFile(ctx context.Context, token string, ref RepoRef, path, branch, message, content string) error {
	path = strings.Trim(path, "/")
	p := fmt.Sprintf("/repos/%s/%s/contents/%s", ref.Owner, ref.Name, escapePath(path))
	encoded := base64.StdEncoding.EncodeToString([]byte(content))
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt) * 400 * time.Millisecond):
			}
		}
		sha, err := c.fileSHA(ctx, token, ref, path, branch)
		if err != nil {
			return err
		}
		payload := map[string]string{
			"message": message,
			"content": encoded,
		}
		if sha != "" {
			payload["sha"] = sha
		}
		if branch = strings.TrimSpace(branch); branch != "" {
			payload["branch"] = branch
		}
		raw, code, err := c.api(ctx, token, http.MethodPut, p, payload)
		if err != nil {
			return err
		}
		if code < 400 {
			return nil
		}
		lastErr = fmt.Errorf("put file %s %d: %s", path, code, string(raw))
		// Race hoặc SHA cũ — thử lại với SHA mới (GitHub 422 "sha wasn't supplied").
		if code != 409 && code != 422 {
			return lastErr
		}
	}
	return lastErr
}

func (c *Client) FileExists(ctx context.Context, token string, ref RepoRef, path, branch string) (bool, error) {
	path = strings.Trim(path, "/")
	p := fmt.Sprintf("/repos/%s/%s/contents/%s", ref.Owner, ref.Name, escapePath(path))
	if branch = strings.TrimSpace(branch); branch != "" {
		p += "?ref=" + urlQueryEscape(branch)
	}
	_, code, err := c.api(ctx, token, http.MethodGet, p, nil)
	if code == 404 {
		return false, nil
	}
	if err != nil || code >= 400 {
		return false, fmt.Errorf("get file %d", code)
	}
	return true, nil
}

// ListPathsRecursive liệt kê mọi file dưới dir (GitHub Contents API).
func (c *Client) ListPathsRecursive(ctx context.Context, token string, ref RepoRef, dir, branch string) ([]string, error) {
	dir = strings.Trim(strings.TrimSpace(dir), "/")
	p := fmt.Sprintf("/repos/%s/%s/contents/%s", ref.Owner, ref.Name, escapePath(dir))
	if branch = strings.TrimSpace(branch); branch != "" {
		p += "?ref=" + urlQueryEscape(branch)
	}
	raw, code, err := c.api(ctx, token, http.MethodGet, p, nil)
	if code == 404 {
		return nil, nil
	}
	if err != nil || code >= 400 {
		return nil, fmt.Errorf("list dir %s %d", dir, code)
	}
	var entries []struct {
		Path string `json:"path"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		switch e.Type {
		case "file":
			out = append(out, e.Path)
		case "dir":
			sub, err := c.ListPathsRecursive(ctx, token, ref, e.Path, branch)
			if err != nil {
				return nil, err
			}
			out = append(out, sub...)
		}
	}
	return out, nil
}

// DeleteFile xóa file trên GitHub (cần SHA).
func (c *Client) DeleteFile(ctx context.Context, token string, ref RepoRef, path, branch, message string) error {
	path = strings.Trim(path, "/")
	sha, err := c.fileSHA(ctx, token, ref, path, branch)
	if err != nil {
		return err
	}
	if sha == "" {
		return nil
	}
	payload := map[string]string{
		"message": message,
		"sha":     sha,
	}
	if branch = strings.TrimSpace(branch); branch != "" {
		payload["branch"] = branch
	}
	p := fmt.Sprintf("/repos/%s/%s/contents/%s", ref.Owner, ref.Name, escapePath(path))
	_, code, err := c.api(ctx, token, http.MethodDelete, p, payload)
	if code == 404 {
		return nil
	}
	if err != nil || code >= 400 {
		return fmt.Errorf("delete file %s %d", path, code)
	}
	return nil
}

// DeleteProjectFolder xóa toàn bộ apps/{slug} trong repo GitOps.
func DeleteProjectFolder(ctx context.Context, client *Client, token string, ref RepoRef, basePath, slug, branch string) error {
	base := strings.Trim(strings.TrimSpace(basePath), "/")
	if base == "" {
		base = "apps"
	}
	root := base + "/" + strings.TrimSpace(slug)
	paths, err := client.ListPathsRecursive(ctx, token, ref, root, branch)
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		return nil
	}
	sortPaths := append([]string(nil), paths...)
	for i := 0; i < len(sortPaths); i++ {
		for j := i + 1; j < len(sortPaths); j++ {
			if len(sortPaths[j]) > len(sortPaths[i]) {
				sortPaths[i], sortPaths[j] = sortPaths[j], sortPaths[i]
			}
		}
	}
	msg := "chore(gitops): remove " + slug
	for _, path := range sortPaths {
		if err := client.DeleteFile(ctx, token, ref, path, branch, msg+" "+path); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) fileSHA(ctx context.Context, token string, ref RepoRef, path, branch string) (string, error) {
	p := fmt.Sprintf("/repos/%s/%s/contents/%s", ref.Owner, ref.Name, escapePath(path))
	if branch = strings.TrimSpace(branch); branch != "" {
		p += "?ref=" + urlQueryEscape(branch)
	}
	raw, code, err := c.api(ctx, token, http.MethodGet, p, nil)
	if code == 404 {
		return "", nil
	}
	if err != nil || code >= 400 {
		return "", fmt.Errorf("get sha %d", code)
	}
	var meta struct {
		SHA string `json:"sha"`
	}
	if json.Unmarshal(raw, &meta) != nil {
		return "", nil
	}
	return meta.SHA, nil
}

func (c *Client) api(ctx context.Context, token, method, path string, payload any) ([]byte, int, error) {
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, 0, err
		}
		body = strings.NewReader(string(b))
	}
	req, err := http.NewRequestWithContext(ctx, method, "https://api.github.com"+path, body)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
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

func escapePath(path string) string {
	parts := strings.Split(path, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}

func urlQueryEscape(s string) string {
	return url.QueryEscape(s)
}
