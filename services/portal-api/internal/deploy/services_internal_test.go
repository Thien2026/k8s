package deploy

import (
	"strings"
	"testing"
)

func TestIngressRoutesSkipsInternal(t *testing.T) {
	svcs := []ServiceDef{
		{Name: "api", IngressPath: "/api", ExposeIngress: true, ContainerPort: 8080, HealthPath: "/health"},
		{Name: "web", IngressPath: "/", ExposeIngress: true, ContainerPort: 8080, HealthPath: "/"},
		{Name: "engine-matrix", IngressPath: "internal", ExposeIngress: false, ContainerPort: 5070, HealthPath: "/health"},
	}
	routes := IngressRoutesFromServices(svcs)
	if len(routes) != 2 {
		t.Fatalf("want 2 public routes, got %d", len(routes))
	}
}

func TestServiceDiscoveryEnvVars(t *testing.T) {
	svcs := []ServiceDef{
		{Name: "api", ExposeIngress: true},
		{Name: "engine-matrix", ExposeIngress: false},
	}
	envs := ServiceDiscoveryEnvVars(svcs, "api")
	if len(envs) != 1 {
		t.Fatalf("want 1 peer env, got %d", len(envs))
	}
	if envs[0]["name"] != "SVC_ENGINE_MATRIX_URL" {
		t.Fatalf("unexpected env name: %v", envs[0]["name"])
	}
	if envs[0]["value"] != "http://engine-matrix:80" {
		t.Fatalf("unexpected url: %v", envs[0]["value"])
	}
}

func TestK8sManifestYAMLIncludesServiceDiscovery(t *testing.T) {
	p := Params{
		ProjectSlug: "demo",
		ProjectName: "demo",
		Environment: "dev",
		Namespace:   "demo-dev",
		ImageTag:    "abc123",
		Layout:      LayoutMulti,
		Services: []ServiceDef{
			{Name: "api", ExposeIngress: true, ContainerPort: 8080, HealthPath: "/health", IngressPath: "/api"},
			{Name: "worker", ExposeIngress: false, ContainerPort: 8080, HealthPath: "/health", IngressPath: "internal"},
		},
	}
	m, err := K8sManifestForService(p, p.Services[1])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(m.YAML, "SVC_API_URL") {
		t.Fatalf("worker yaml missing service discovery: %s", m.YAML)
	}
	if !strings.Contains(m.YAML, "http://api:80") {
		t.Fatalf("worker yaml missing api url: %s", m.YAML)
	}
}

func TestNormalizeInternalIngressMarker(t *testing.T) {
	s := normalizeServiceDef(ServiceDef{Name: "worker", IngressPath: "internal"})
	if s.ExposeIngress {
		t.Fatal("internal marker should not expose ingress")
	}
}
