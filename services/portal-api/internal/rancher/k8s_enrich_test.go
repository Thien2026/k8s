package rancher

import (
	"encoding/json"
	"testing"
)

func TestEnrichWorkloadRow_PodListItemWithoutKind(t *testing.T) {
	raw := `{
		"metadata": {"name": "app-abc", "namespace": "dev"},
		"status": {
			"phase": "Running",
			"conditions": [{"type": "Ready", "status": "True"}],
			"containerStatuses": [{"ready": true, "restartCount": 0, "image": "demo:1"}]
		}
	}`
	row, ok := parseK8sItem(json.RawMessage(raw), "Pod")
	if !ok {
		t.Fatal("parse failed")
	}
	if row.Status != "Running" {
		t.Fatalf("status=%q", row.Status)
	}
	if !row.Ready {
		t.Fatal("expected Ready=true from containerStatuses")
	}
}

func TestEnrichWorkloadRow_PodReadyFromConditions(t *testing.T) {
	raw := `{
		"metadata": {"name": "app-abc"},
		"status": {
			"phase": "Running",
			"conditions": [{"type": "Ready", "status": "True"}]
		}
	}`
	row, ok := parseK8sItem(json.RawMessage(raw), "Pod")
	if !ok || !row.Ready {
		t.Fatalf("expected ready from conditions, ok=%v ready=%v", ok, row.Ready)
	}
}

func TestEnrichWorkloadRow_DeploymentListItemWithoutKind(t *testing.T) {
	raw := `{
		"metadata": {"name": "api", "namespace": "demo-dev"},
		"spec": {"replicas": 1},
		"status": {
			"replicas": 1,
			"readyReplicas": 1,
			"availableReplicas": 1
		}
	}`
	row, ok := parseK8sItem(json.RawMessage(raw), "Deployment")
	if !ok {
		t.Fatal("parse failed")
	}
	if row.Replicas != "1/1 ready" {
		t.Fatalf("replicas=%q", row.Replicas)
	}
	if row.Status == "" {
		t.Fatal("expected status")
	}
}
