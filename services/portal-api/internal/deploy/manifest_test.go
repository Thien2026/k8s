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
