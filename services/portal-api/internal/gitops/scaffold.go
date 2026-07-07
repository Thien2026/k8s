package gitops

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/Thien2026/k8s/services/portal-api/internal/deploy"
	"github.com/Thien2026/k8s/services/portal-api/internal/domains"
	"gopkg.in/yaml.v3"
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
	if len(in.DevRoutes) > 0 && strings.TrimSpace(in.DevHost) != "" {
		ingRaw, err := domains.IngressManifest(in.DevHost, in.DevParams.Namespace, 1, true, in.DevRoutes)
		if err != nil {
			return nil, err
		}
		ingYAML, err := jsonToYAML(ingRaw)
		if err != nil {
			return nil, err
		}
		out[root+"/base/ingress.yaml"] = ingYAML
		baseResources = append(baseResources, "ingress.yaml")
	}
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
		overlay := root + "/overlays/" + env.name + "/kustomization.yaml"
		out[overlay] = renderOverlayKustomization(env.params, env.host, env.routes, env.name)
	}
	return out, nil
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
			b.WriteString("  - name: " + p.ImageRefFor(svc) + "\n")
			b.WriteString("    newTag: latest\n")
		}
	}
	host = strings.TrimSpace(host)
	if host != "" && len(routes) > 0 {
		b.WriteString("patches:\n")
		b.WriteString("  - target:\n")
		b.WriteString("      kind: Ingress\n")
		b.WriteString("      name: app-1\n")
		b.WriteString("    patch: |-\n")
		b.WriteString("      - op: replace\n")
		b.WriteString("        path: /spec/rules/0/host\n")
		b.WriteString("        value: " + host + "\n")
		b.WriteString("      - op: replace\n")
		b.WriteString("        path: /spec/tls/0/hosts/0\n")
		b.WriteString("        value: " + host + "\n")
		b.WriteString("      - op: replace\n")
		b.WriteString("        path: /spec/tls/0/secretName\n")
		b.WriteString("        value: tls-app-1\n")
	}
	_ = env
	return b.String()
}

func jsonToYAML(raw []byte) (string, error) {
	var obj any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return "", err
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(obj); err != nil {
		return "", err
	}
	_ = enc.Close()
	return buf.String(), nil
}
