package deploy

import (
	"encoding/json"
	"testing"

	"github.com/Thien2026/k8s/services/portal-api/internal/registry"
)

func TestK8sManifest_RollingUpdateDev(t *testing.T) {
	m, err := K8sManifest(Params{
		ProjectSlug: "demo",
		Namespace:   "demo-dev",
		Environment: "dev",
		ImageTag:    "abc123",
		Registry:    registry.ProjectRegistry{ImagePrefix: "harbor.example.com/demo"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var dep map[string]any
	if err := json.Unmarshal(m.Deployment, &dep); err != nil {
		t.Fatal(err)
	}
	spec := dep["spec"].(map[string]any)
	if spec["replicas"].(float64) != 1 {
		t.Fatalf("dev replicas want 1, got %v", spec["replicas"])
	}
	strat := spec["strategy"].(map[string]any)
	if strat["type"] != "RollingUpdate" {
		t.Fatalf("want RollingUpdate, got %v", strat["type"])
	}
	ru := strat["rollingUpdate"].(map[string]any)
	if ru["maxUnavailable"].(float64) != 0 || ru["maxSurge"].(float64) != 1 {
		t.Fatalf("unexpected rollingUpdate: %v", ru)
	}
}

func TestK8sManifest_ProdTwoReplicas(t *testing.T) {
	m, err := K8sManifest(Params{
		ProjectSlug: "demo",
		Namespace:   "demo-prod",
		Environment: "prod",
		ImageTag:    "abc123",
		Registry:    registry.ProjectRegistry{ImagePrefix: "harbor.example.com/demo"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var dep map[string]any
	if err := json.Unmarshal(m.Deployment, &dep); err != nil {
		t.Fatal(err)
	}
	spec := dep["spec"].(map[string]any)
	if spec["replicas"].(float64) != 2 {
		t.Fatalf("prod replicas want 2, got %v", spec["replicas"])
	}
}

func TestK8sManifest_ApiProbeUsesIngressPrefix(t *testing.T) {
	m, err := K8sManifestForService(Params{
		ProjectSlug: "shop",
		Namespace:   "shop-dev",
		Environment: "dev",
		ImageTag:    "abc123",
		Layout:      LayoutMulti,
		Registry:    registry.ProjectRegistry{ImagePrefix: "harbor.example.com/shop"},
	}, DefaultMultiServices[0])
	if err != nil {
		t.Fatal(err)
	}
	var dep map[string]any
	if err := json.Unmarshal(m.Deployment, &dep); err != nil {
		t.Fatal(err)
	}
	spec := dep["spec"].(map[string]any)
	tmpl := spec["template"].(map[string]any)
	podSpec := tmpl["spec"].(map[string]any)
	containers := podSpec["containers"].([]any)
	c0 := containers[0].(map[string]any)
	rp := c0["readinessProbe"].(map[string]any)
	hg := rp["httpGet"].(map[string]any)
	if hg["path"] != "/api/health" {
		t.Fatalf("readiness path want /api/health, got %v", hg["path"])
	}
}
