package registry

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/Thien2026/k8s/services/portal-api/internal/harbor"
)

type harborProvider struct {
	client *harbor.Client
}

func (h *harborProvider) name() string { return Harbor }

func (h *harborProvider) label() string { return "Harbor (on-prem)" }

func (h *harborProvider) host() string {
	if h.client == nil || !h.client.Enabled() {
		return ""
	}
	u, err := url.Parse(strings.TrimRight(h.client.BaseURL(), "/"))
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Host
}

func (h *harborProvider) imagePrefix(slug string) string {
	host := h.host()
	if host == "" {
		return ""
	}
	return host + "/" + slug
}

func (h *harborProvider) ensureProject(ctx context.Context, slug string) error {
	if h.client == nil || !h.client.Enabled() {
		return fmt.Errorf("Harbor chưa cấu hình")
	}
	return h.client.EnsureProject(ctx, slug)
}

func (h *harborProvider) projectRegistry(slug string, ready bool, hint string) ProjectRegistry {
	prefix := h.imagePrefix(slug)
	pr := ProjectRegistry{
		Provider:    Harbor,
		Label:       h.label(),
		ImagePrefix: prefix,
		Ready:       ready,
		ReadyHint:   hint,
	}
	if prefix != "" {
		pr.PushExample = prefix + "/app:latest"
		pr.LoginHint = "docker login " + h.host()
	}
	return pr
}
