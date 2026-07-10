package gitops

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Thien2026/k8s/services/portal-api/internal/deploy"
)

// ScaffoldInput đầu vào sinh manifest GitOps cho một project.
type ScaffoldInput struct {
	Slug         string
	BasePath     string
	PlatformHost string
	DevParams    deploy.Params
	ProdParams   deploy.Params
	DevRoutes    []deploy.IngressRoute
	ProdRoutes   []deploy.IngressRoute
	DevHost      string
	ProdHost     string
}

// BuildFiles trả về path → nội dung file trong repo GitOps.
func BuildFiles(in ScaffoldInput) (map[string]string, error) {
	slug := strings.TrimSpace(in.Slug)
	if slug == "" {
		return nil, fmt.Errorf("slug trống")
	}
	base := strings.Trim(strings.TrimSpace(in.BasePath), "/")
	if base == "" {
		base = "apps"
	}
	root := base + "/" + slug
	out := map[string]string{}

	baseResources := []string{}
	manifests, err := deploy.K8sManifests(in.DevParams)
	if err != nil {
		return nil, err
	}
	for _, m := range manifests {
		name := strings.TrimSpace(m.ServiceName)
		if name == "" {
			continue
		}
		fn := name + ".yaml"
		out[root+"/base/"+fn] = strings.TrimSpace(m.YAML) + "\n"
		baseResources = append(baseResources, fn)
	}
	// Ingress do Console tab Domains quản lý (app-{domainId}) — không đưa vào GitOps để tránh trùng host với ArgoCD sync.
	sort.Strings(baseResources)
	out[root+"/base/kustomization.yaml"] = renderBaseKustomization(baseResources)

	for _, env := range []struct {
		name   string
		params deploy.Params
		host   string
		routes []deploy.IngressRoute
	}{
		{"dev", in.DevParams, in.DevHost, in.DevRoutes},
		{"prod", in.ProdParams, in.ProdHost, in.ProdRoutes},
	} {
		overlayDir := root + "/overlays/" + env.name
		out[overlayDir+"/kustomization.yaml"] = renderOverlayKustomization(env.params, env.host, env.routes, env.name)
		if patch := PullSecretPatchYAML(env.params.ImagePullSecret); patch != "" {
			out[overlayDir+"/"+pullSecretPatchFile] = patch
		}
	}
	return out, nil
}

// SyncBaseManifests cập nhật base deployments (service discovery, service mới như worker).
func SyncBaseManifests(ctx context.Context, client *Client, token string, ref RepoRef, basePath, slug, branch string, params deploy.Params, commitMsg string) error {
	base := strings.Trim(strings.TrimSpace(basePath), "/")
	if base == "" {
		base = "apps"
	}
	root := base + "/" + strings.TrimSpace(slug)
	manifests, err := deploy.K8sManifests(params)
	if err != nil {
		return err
	}
	var resources []string
	for _, m := range manifests {
		name := strings.TrimSpace(m.ServiceName)
		if name == "" {
			continue
		}
		fn := name + ".yaml"
		path := root + "/base/" + fn
		if err := client.PutFile(ctx, token, ref, path, branch, commitMsg+" "+path, strings.TrimSpace(m.YAML)+"\n"); err != nil {
			return fmt.Errorf("sync base %s: %w", name, err)
		}
		resources = append(resources, fn)
	}
	sort.Strings(resources)
	kustPath := root + "/base/kustomization.yaml"
	return client.PutFile(ctx, token, ref, kustPath, branch, commitMsg+" kustomization", renderBaseKustomization(resources))
}

func renderBaseKustomization(resources []string) string {
	var b strings.Builder
	b.WriteString("apiVersion: kustomize.config.k8s.io/v1beta1\n")
	b.WriteString("kind: Kustomization\n")
	b.WriteString("resources:\n")
	for _, r := range resources {
		b.WriteString("  - " + r + "\n")
	}
	return b.String()
}

func renderOverlayKustomization(p deploy.Params, host string, routes []deploy.IngressRoute, env string) string {
	var b strings.Builder
	b.WriteString("apiVersion: kustomize.config.k8s.io/v1beta1\n")
	b.WriteString("kind: Kustomization\n")
	b.WriteString("namespace: " + strings.TrimSpace(p.Namespace) + "\n")
	b.WriteString("resources:\n  - ../../base\n")
	svcs := p.EffectiveServices()
	if len(svcs) > 0 {
		b.WriteString("images:\n")
		for _, svc := range svcs {
			b.WriteString("  - name: " + p.ImageRepositoryFor(svc) + "\n")
			b.WriteString("    newTag: latest\n")
		}
	}
	if secret := strings.TrimSpace(p.ImagePullSecret); secret != "" {
		b.WriteString("patches:\n")
		b.WriteString("  - path: " + pullSecretPatchFile + "\n")
		b.WriteString("    target:\n")
		b.WriteString("      kind: Deployment\n")
	}
	_ = env
	_ = host
	_ = routes
	return b.String()
}
