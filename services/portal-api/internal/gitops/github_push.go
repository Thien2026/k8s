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

func (c *Client) PutFile(ctx context.Context, token string, ref RepoRef, path, branch, message, content string) error {
	path = strings.Trim(path, "/")
	sha, _ := c.fileSHA(ctx, token, ref, path, branch)
	payload := map[string]string{
		"message": message,
		"content": base64.StdEncoding.EncodeToString([]byte(content)),
	}
	if sha != "" {
		payload["sha"] = sha
	}
	if branch = strings.TrimSpace(branch); branch != "" {
		payload["branch"] = branch
	}
	p := fmt.Sprintf("/repos/%s/%s/contents/%s", ref.Owner, ref.Name, escapePath(path))
	raw, code, err := c.api(ctx, token, http.MethodPut, p, payload)
	if err != nil {
		return err
	}
	if code >= 400 {
		return fmt.Errorf("put file %s %d: %s", path, code, string(raw))
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
