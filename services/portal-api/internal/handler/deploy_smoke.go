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
	host, _, _ := h.primaryDomainForEnv(ctx, projectID, env)
	return host
}

func (h *Handler) primaryDomainForEnv(ctx context.Context, projectID int64, env string) (hostname string, domainID int64, tlsEnabled bool) {
	var tls bool
	err := h.db.QueryRow(ctx, `
		SELECT id, hostname, tls_enabled FROM project_domains
		WHERE project_id=$1 AND environment=$2 AND sync_status='synced'
		ORDER BY CASE kind WHEN 'auto' THEN 0 ELSE 1 END, id
		LIMIT 1`, projectID, env).Scan(&domainID, &hostname, &tls)
	if err != nil {
		return "", 0, false
	}
	return strings.TrimSpace(hostname), domainID, tls
}

func isTLSCertPendingErr(msg string) bool {
	msg = strings.ToLower(strings.TrimSpace(msg))
	return strings.Contains(msg, "tls:") ||
		strings.Contains(msg, "x509:") ||
		strings.Contains(msg, "certificate") ||
		strings.Contains(msg, "ingress.local")
}

func (h *Handler) waitDomainTLSReady(ctx context.Context, p projectRow, env string, domainID int64) (bool, string) {
	if domainID <= 0 || h.rancher == nil || !h.rancher.Enabled() {
		return true, ""
	}
	certName := fmt.Sprintf("tls-app-%d", domainID)
	ns := h.projectNamespace(p, env)
	deadline := time.Now().Add(3 * time.Minute)
	for {
		st, _ := h.rancher.CertificateReady(ctx, "", ns, certName)
		switch st {
		case "ready":
			return true, ""
		case "failed":
			return false, "Certificate Let's Encrypt thất bại — xem cert-manager / tab Domains"
		}
		if time.Now().After(deadline) {
			return false, "Đang chờ cert Let's Encrypt (cert-manager) — thường 30–90 giây sau deploy"
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err().Error()
		case <-time.After(5 * time.Second):
		}
	}
}

func (h *Handler) smokeCheckProjectDomain(ctx context.Context, p projectRow, env string, paths []string) (status, detail string) {
	host := h.primaryDomainHost(ctx, p.ID, env)
	return h.smokeCheckHTTP(ctx, p, env, host, paths)
}

func (h *Handler) smokeCheckHTTP(ctx context.Context, p projectRow, env, hostname string, paths []string) (status, detail string) {
	if hostname == "" {
		return "skipped", "Không có domain HTTPS — bỏ qua smoke check"
	}
	_, domainID, tlsOn := h.primaryDomainForEnv(ctx, p.ID, env)
	if tlsOn && domainID > 0 {
		ready, waitDetail := h.waitDomainTLSReady(ctx, p, env, domainID)
		if !ready {
			if strings.Contains(strings.ToLower(waitDetail), "đang chờ") {
				return "running", waitDetail
			}
			return "failed", waitDetail
		}
	}
	if len(paths) == 0 {
		paths = []string{"/health", "/"}
	}
	client := &http.Client{Timeout: 12 * time.Second}
	var okParts []string
	var lastErr string
	const attempts = 6
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return "running", "Smoke check bị hủy — " + ctx.Err().Error()
			case <-time.After(8 * time.Second):
			}
		}
		okParts = nil
		lastErr = ""
		retryableHTTP := false
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
			pathErr := fmt.Sprintf("HTTP %d tại %s", code, path)
			if lastErr == "" {
				lastErr = pathErr
			}
			if code == 502 || code == 503 {
				retryableHTTP = true
			}
		}
		if len(okParts) > 0 {
			return "success", strings.Join(okParts, "; ")
		}
		if lastErr == "" {
			lastErr = "không phản hồi"
		}
		if retryableHTTP && attempt < attempts-1 {
			continue
		}
		if isTLSCertPendingErr(lastErr) && attempt < attempts-1 {
			continue
		}
	}
	if isTLSCertPendingErr(lastErr) {
		return "running", "Đang chờ TLS cert: " + lastErr
	}
	return "failed", lastErr
}

func imageTagFromImageRef(image string) string {
	image = normalizeListedImage(strings.TrimSpace(image))
	if image == "" {
		return ""
	}
	if i := strings.Index(image, ","); i > 0 {
		image = strings.TrimSpace(image[:i])
	}
	if i := strings.LastIndex(image, ":"); i >= 0 && i < len(image)-1 {
		tag := strings.TrimSpace(image[i+1:])
		if j := strings.Index(tag, "@"); j >= 0 {
			tag = tag[:j]
		}
		return tag
	}
	return ""
}

func (h *Handler) clusterServingImageTag(ctx context.Context, p projectRow, env string) string {
	if h.rancher == nil || !h.rancher.Enabled() {
		return ""
	}
	if tag := h.clusterServingImageTagFromDeployments(ctx, p, env); tag != "" {
		return tag
	}
	ns := h.projectNamespace(p, env)
	list, err := h.rancher.ListK8s(ctx, "", "pods", ns, 1, 80)
	if err != nil {
		return ""
	}
	repo, _ := h.getProjectRepo(ctx, p.ID)
	services, consoleLayout := h.loadDeployServices(ctx, p.ID, repo)
	prefixes, layout := h.clusterPodPrefixes(ctx, p, env, services, consoleLayout)
	byPrefix := map[string]string{}
	for _, pod := range list.Items {
		if !strings.EqualFold(strings.TrimSpace(pod.Status), "Running") {
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
		if tag := imageTagFromImageRef(pod.Images); tag != "" {
			if layout == deploy.LayoutMulti {
				byPrefix[servicePrefixFromPod(pod.Name, prefixes)] = tag
			} else {
				return tag
			}
		}
	}
	if layout == deploy.LayoutMulti && len(byPrefix) > 0 {
		return unanimousServingTag(byPrefix, len(prefixes))
	}
	return ""
}

func (h *Handler) clusterServingImageTagFromDeployments(ctx context.Context, p projectRow, env string) string {
	if h.rancher == nil || !h.rancher.Enabled() {
		return ""
	}
	ns := h.projectNamespace(p, env)
	names := []string{"app"}
	if h.clusterRunsFleet(ctx, p, env) {
		names = []string{"api", "web"}
	}
	bySvc := map[string]string{}
	for _, name := range names {
		dep, err := h.rancher.GetDeploymentDetail(ctx, "", ns, name)
		if err != nil {
			continue
		}
		st, _, _ := evaluateDeploymentRollout(dep.DeploymentRolloutStatus)
		if stageStatus(st) != "success" {
			continue
		}
		if tag := imageTagFromImageRef(dep.ContainerImage); tag != "" {
			bySvc[name] = tag
		}
	}
	if len(bySvc) == 0 {
		return ""
	}
	if len(bySvc) >= 2 {
		return unanimousServingTag(bySvc, 2)
	}
	for _, tag := range bySvc {
		return tag
	}
	return ""
}

func servicePrefixFromPod(name string, prefixes []string) string {
	for _, pfx := range prefixes {
		if strings.HasPrefix(name, pfx) {
			return strings.TrimSuffix(pfx, "-")
		}
	}
	return ""
}

func unanimousServingTag(byService map[string]string, want int) string {
	if len(byService) == 0 {
		return ""
	}
	var first string
	for _, tag := range byService {
		if first == "" {
			first = tag
			continue
		}
		if !imageTagsMatch(first, tag) {
			// Mixed fleet — trả tag xuất hiện nhiều nhất (thường là bản cũ vẫn giữ traffic).
			counts := map[string]int{}
			for _, t := range byService {
				counts[t]++
			}
			best, n := "", 0
			for t, c := range counts {
				if c > n {
					best, n = t, c
				}
			}
			return best
		}
	}
	if want > 0 && len(byService) < want {
		return ""
	}
	return first
}

// clusterPodPrefixes — đọc workload thực tế trên cluster, không chỉ layout Console.
func (h *Handler) clusterPodPrefixes(ctx context.Context, p projectRow, env string, dbServices []deploy.ServiceDef, consoleLayout string) (prefixes []string, layout string) {
	if h.clusterRunsSingleAppOnly(ctx, p, env, dbServices) {
		return []string{"app-"}, deploy.LayoutSingle
	}
	var running []string
	for _, svc := range dbServices {
		name := strings.TrimSpace(svc.Name)
		if name == "" {
			continue
		}
		if h.serviceWorkloadExists(ctx, p, env, name) {
			running = append(running, name)
		}
	}
	if len(running) == 0 {
		for _, name := range []string{"api", "web"} {
			if h.serviceWorkloadExists(ctx, p, env, name) {
				running = append(running, name)
			}
		}
	}
	if len(running) >= 2 {
		for _, n := range running {
			prefixes = append(prefixes, n+"-")
		}
		return prefixes, deploy.LayoutMulti
	}
	if len(running) == 1 {
		return []string{running[0] + "-"}, deploy.LayoutSingle
	}
	if deploy.NormalizeLayout(consoleLayout) == deploy.LayoutMulti {
		check := h.fleetServicesForRollout(ctx, p, env, dbServices)
		for _, s := range check {
			if name := strings.TrimSpace(s.Name); name != "" {
				prefixes = append(prefixes, name+"-")
			}
		}
		if len(prefixes) == 0 {
			prefixes = []string{"api-", "web-"}
		}
		return prefixes, deploy.LayoutMulti
	}
	return []string{"app-"}, deploy.LayoutSingle
}

func (h *Handler) applySmokeGate(ctx context.Context, p projectRow, env string, d *deploymentRow) bool {
	if d == nil {
		return true
	}
	repo, _ := h.getProjectRepo(ctx, p.ID)
	paths := h.smokePathsForDeployment(ctx, p.ID, repo, d)
	st, detail := h.smokeCheckProjectDomain(ctx, p, env, paths)
	d.SmokeStatus = st
	d.SmokeDetail = detail
	return st != "failed"
}
