package deploy

import "testing"

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

func TestNormalizeInternalIngressMarker(t *testing.T) {
	s := normalizeServiceDef(ServiceDef{Name: "worker", IngressPath: "internal"})
	if s.ExposeIngress {
		t.Fatal("internal marker should not expose ingress")
	}
}
