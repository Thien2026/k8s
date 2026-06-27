package deploy

import (
	"testing"

	"github.com/Thien2026/k8s/services/portal-api/internal/registry"
)

func TestK8sManifestsMultiService(t *testing.T) {
	p := Params{
		ProjectSlug: "demo",
		Namespace:   "demo-dev",
		Environment: "dev",
		Layout:      LayoutMulti,
		Registry:    registry.ProjectRegistry{ImagePrefix: "harbor.example.com/demo"},
		Services:    DefaultMultiServices,
		ImageTag:    "abc123",
	}
	manifests, err := K8sManifests(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifests) != 2 {
		t.Fatalf("want 2 manifests, got %d", len(manifests))
	}
	if manifests[0].ServiceName != "api" || manifests[1].ServiceName != "web" {
		t.Fatalf("unexpected service names: %s, %s", manifests[0].ServiceName, manifests[1].ServiceName)
	}
	routes := IngressRoutesFromServices(p.EffectiveServices())
	if len(routes) != 2 {
		t.Fatalf("want 2 ingress routes, got %d", len(routes))
	}
	if routes[0].Path != "/api" || routes[0].ServiceName != "api" {
		t.Fatalf("first route should be /api -> api, got %+v", routes[0])
	}
	svcs := p.EffectiveServices()
	if p.imageRefFor(svcs[0]) != "harbor.example.com/demo/api:abc123" {
		t.Fatalf("api image ref mismatch: %s", p.imageRefFor(svcs[0]))
	}
	if p.imageRefFor(svcs[1]) != "harbor.example.com/demo/web:abc123" {
		t.Fatalf("web image ref mismatch: %s", p.imageRefFor(svcs[1]))
	}
}

func TestGitHubWorkflowMultiService(t *testing.T) {
	p := Params{
		ProjectSlug:      "demo",
		Branch:           "main",
		Layout:           LayoutMulti,
		RegistryProvider: registry.Harbor,
		HarborHost:       "harbor.example.com",
		Registry:         registry.ProjectRegistry{ImagePrefix: "harbor.example.com/demo"},
		Services:         DefaultMultiServices,
	}
	wf := GitHubWorkflow(p)
	if !stringsContains(wf.Content, "Build and push API") || !stringsContains(wf.Content, "Build and push Web") {
		t.Fatalf("workflow should build both services: %s", wf.Content)
	}
	if !stringsContains(wf.Content, "API_IMAGE:") || !stringsContains(wf.Content, "WEB_IMAGE:") {
		t.Fatal("workflow should define per-service image env")
	}
}

func stringsContains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
