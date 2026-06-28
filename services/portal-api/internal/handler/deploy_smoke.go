package handler

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Thien2026/k8s/services/portal-api/internal/deploy"
)

func (h *Handler) primaryDomainHost(ctx context.Context, projectID int64, env string) string {
	var hostname string
	_ = h.db.QueryRow(ctx, `
		SELECT hostname FROM project_domains
		WHERE project_id=$1 AND environment=$2 AND sync_status='synced'
		ORDER BY CASE kind WHEN 'auto' THEN 0 ELSE 1 END, id
		LIMIT 1`, projectID, env).Scan(&hostname)
	return strings.TrimSpace(hostname)
}

func (h *Handler) smokeCheckHTTP(ctx context.Context, hostname string, paths []string) (status, detail string) {
	if hostname == "" {
		return "skipped", "Không có domain HTTPS — bỏ qua smoke check"
	}
	if len(paths) == 0 {
		paths = []string{"/health", "/"}
	}
	client := &http.Client{Timeout: 12 * time.Second}
	var okParts []string
	var lastErr string
	for _, path := range paths {
		url := "https://" + hostname + path
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			lastErr = err.Error()
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err.Error()
			continue
		}
		code := resp.StatusCode
		_ = resp.Body.Close()
		if code >= 200 && code < 400 {
			okParts = append(okParts, fmt.Sprintf("HTTP %d tại %s", code, path))
			continue
		}
		lastErr = fmt.Sprintf("HTTP %d tại %s", code, path)
		return "failed", lastErr
	}
	if len(okParts) == 0 {
		if lastErr == "" {
			lastErr = "không phản hồi"
		}
		return "failed", lastErr
	}
	return "success", strings.Join(okParts, "; ")
}

func imageTagFromImageRef(image string) string {
	image = strings.TrimSpace(image)
	if i := strings.LastIndex(image, ":"); i >= 0 && i < len(image)-1 {
		return image[i+1:]
	}
	return ""
}

func (h *Handler) clusterServingImageTag(ctx context.Context, p projectRow, env string) string {
	if h.rancher == nil || !h.rancher.Enabled() {
		return ""
	}
	ns := h.projectNamespace(p, env)
	list, err := h.rancher.ListK8s(ctx, "", "pods", ns, 1, 80)
	if err != nil {
		return ""
	}
	repo, _ := h.getProjectRepo(ctx, p.ID)
	services, layout := h.loadDeployServices(ctx, p.ID, repo)
	prefixes := []string{"app-"}
	if layout == deploy.LayoutMulti {
		prefixes = nil
		for _, s := range services {
			if name := strings.TrimSpace(s.Name); name != "" {
				prefixes = append(prefixes, name+"-")
			}
		}
		if len(prefixes) == 0 {
			prefixes = []string{"api-", "web-"}
		}
	}
	for _, pod := range list.Items {
		if pod.Status != "Running" || !pod.Ready {
			continue
		}
		okPrefix := false
		for _, pfx := range prefixes {
			if strings.HasPrefix(pod.Name, pfx) {
				okPrefix = true
				break
			}
		}
		if !okPrefix {
			continue
		}
		img := strings.TrimSpace(pod.Images)
		if img == "" {
			continue
		}
		if idx := strings.Index(img, " (+"); idx > 0 {
			img = img[:idx]
		}
		if tag := imageTagFromImageRef(img); tag != "" {
			return tag
		}
	}
	return ""
}

func (h *Handler) applySmokeGate(ctx context.Context, p projectRow, env string, d *deploymentRow) bool {
	if d == nil {
		return true
	}
	host := h.primaryDomainHost(ctx, p.ID, env)
	repo, _ := h.getProjectRepo(ctx, p.ID)
	paths := h.smokePathsForProject(ctx, p.ID, repo)
	st, detail := h.smokeCheckHTTP(ctx, host, paths)
	d.SmokeStatus = st
	d.SmokeDetail = detail
	return st != "failed"
}
