package github

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// DownloadRunLog tải toàn bộ log của workflow run (zip).
func (c *Client) DownloadRunLog(ctx context.Context, token, owner, repo string, runID int64) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/actions/runs/%d/logs", apiBase, owner, repo, runID)
	return c.downloadLogURL(ctx, token, url)
}

// DownloadJobLog tải log text của GitHub Actions job (zip từ API).
func (c *Client) DownloadJobLog(ctx context.Context, token, owner, repo string, jobID int64) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/actions/jobs/%d/logs", apiBase, owner, repo, jobID)
	return c.downloadLogURL(ctx, token, url)
}

func (c *Client) downloadLogURL(ctx context.Context, token, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{
		Timeout: 90 * time.Second,
		CheckRedirect: func(r *http.Request, via []*http.Request) error {
			r.Header.Set("Authorization", "Bearer "+token)
			return nil
		},
	}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	if res.StatusCode >= 400 {
		return "", fmt.Errorf("logs %d: %s", res.StatusCode, string(body))
	}
	if len(body) >= 2 && body[0] == 'P' && body[1] == 'K' {
		return extractZipLogs(body)
	}
	return string(body), nil
}

func extractZipLogs(zipBytes []byte) (string, error) {
	r, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return "", err
	}
	var names []string
	for _, f := range r.File {
		if !f.FileInfo().IsDir() {
			names = append(names, f.Name)
		}
	}
	sort.Strings(names)
	var b strings.Builder
	for _, name := range names {
		f, err := r.Open(name)
		if err != nil {
			continue
		}
		raw, _ := io.ReadAll(f)
		f.Close()
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.Write(raw)
	}
	return b.String(), nil
}

func TruncateLog(s string, maxBytes int) string {
	if maxBytes <= 0 || len(s) <= maxBytes {
		return s
	}
	// Giữ đầu + cuối để không mất lỗi ở đầu pipeline lẫn cuối.
	head := 12 * 1024
	tail := maxBytes - head - 96
	if tail < 12*1024 {
		head = maxBytes / 4
		tail = maxBytes - head - 96
	}
	if tail < 1024 {
		return "… (log quá dài, " + fmt.Sprintf("%d", len(s)) + " bytes — mở GitHub Actions để xem đủ)\n\n" + s[len(s)-maxBytes:]
	}
	return s[:head] + "\n\n… [" + fmt.Sprintf("%d", len(s)-head-tail) + " bytes ở giữa đã lược] …\n\n" + s[len(s)-tail:]
}

func TruncateLogFull(s string, maxBytes int) (string, bool) {
	if len(s) <= maxBytes {
		return s, false
	}
	return TruncateLog(s, maxBytes), true
}
