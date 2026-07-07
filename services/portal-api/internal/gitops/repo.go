package gitops

import (
	"fmt"
	"net/url"
	"strings"
)

// RepoRef parsed GitHub repository from HTTPS URL.
type RepoRef struct {
	Owner string
	Name  string
}

func ParseRepoURL(raw string) (RepoRef, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return RepoRef{}, fmt.Errorf("repo URL trống")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return RepoRef{}, fmt.Errorf("repo URL không hợp lệ")
	}
	host := strings.ToLower(u.Host)
	if host != "github.com" && host != "www.github.com" {
		return RepoRef{}, fmt.Errorf("chỉ hỗ trợ github.com")
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return RepoRef{}, fmt.Errorf("URL phải dạng https://github.com/owner/repo")
	}
	return RepoRef{Owner: parts[0], Name: strings.TrimSuffix(parts[1], ".git")}, nil
}

func OverlayPath(basePath, slug, env string) string {
	base := strings.Trim(strings.TrimSpace(basePath), "/")
	if base == "" {
		base = "apps"
	}
	return fmt.Sprintf("%s/%s/overlays/%s/kustomization.yaml", base, strings.TrimSpace(slug), strings.ToLower(strings.TrimSpace(env)))
}
