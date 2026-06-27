package rancher

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseDeploymentRollout_ready(t *testing.T) {
	raw := json.RawMessage(`{
		"metadata": {"name": "app", "generation": 3},
		"status": {
			"replicas": 1,
			"readyReplicas": 1,
			"updatedReplicas": 1,
			"availableReplicas": 1,
			"observedGeneration": 3,
			"conditions": [
				{"type": "Available", "status": "True", "reason": "MinimumReplicasAvailable"},
				{"type": "Progressing", "status": "True", "reason": "NewReplicaSetAvailable"}
			]
		}
	}`)
	st, err := ParseDeploymentRollout(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !st.IsReady() {
		t.Fatalf("expected ready, got %+v", st)
	}
	if st.IsFailed() {
		t.Fatal("should not be failed")
	}
	if st.Summary() == "" {
		t.Fatal("expected summary")
	}
}

func TestParseDeploymentRollout_failed(t *testing.T) {
	raw := json.RawMessage(`{
		"metadata": {"name": "app", "generation": 2},
		"status": {
			"replicas": 1,
			"readyReplicas": 0,
			"updatedReplicas": 1,
			"availableReplicas": 0,
			"observedGeneration": 2,
			"conditions": [
				{"type": "Progressing", "status": "False", "reason": "ProgressDeadlineExceeded", "message": "ReplicaSet app-xyz has timed out progressing"}
			]
		}
	}`)
	st, err := ParseDeploymentRollout(raw)
	if err != nil {
		t.Fatal(err)
	}
	if st.IsReady() {
		t.Fatal("should not be ready")
	}
	if !st.IsFailed() {
		t.Fatal("expected failed")
	}
	if st.FailureMessage() == "" {
		t.Fatal("expected failure message")
	}
}

func TestParseDeploymentRollout_availableFalseNotFailed(t *testing.T) {
	// Trạng thái K8s phổ biến khi pod mới đang khởi động — trước đây bị nhầm là failed.
	raw := json.RawMessage(`{
		"metadata": {"name": "app", "generation": 2},
		"status": {
			"replicas": 1,
			"readyReplicas": 0,
			"updatedReplicas": 1,
			"availableReplicas": 0,
			"observedGeneration": 2,
			"conditions": [
				{"type": "Available", "status": "False", "reason": "MinimumReplicasUnavailable", "message": "Deployment does not have minimum availability."},
				{"type": "Progressing", "status": "True", "reason": "ReplicaSetUpdated"}
			]
		}
	}`)
	st, err := ParseDeploymentRollout(raw)
	if err != nil {
		t.Fatal(err)
	}
	if st.IsFailed() {
		t.Fatalf("Available=False during rollout must not be failed: %q", st.FailureMessage())
	}
	if st.IsReady() {
		t.Fatal("should not be ready yet")
	}
	if !st.IsRolloutInProgress() {
		t.Fatal("expected rollout in progress")
	}
}

func TestParseDeploymentDetail_image(t *testing.T) {
	raw := json.RawMessage(`{
		"metadata": {"name": "app", "generation": 1},
		"spec": {
			"template": {
				"metadata": {"labels": {"platform.7mlabs.com/image-tag": "abc123"}},
				"spec": {"containers": [{"image": "harbor.example.com/proj/app:abc123"}]}
			}
		},
		"status": {
			"replicas": 1,
			"readyReplicas": 1,
			"updatedReplicas": 1,
			"availableReplicas": 1,
			"observedGeneration": 1,
			"conditions": [{"type": "Available", "status": "True"}]
		}
	}`)
	d, err := ParseDeploymentDetail(raw)
	if err != nil {
		t.Fatal(err)
	}
	if d.ContainerImage != "harbor.example.com/proj/app:abc123" {
		t.Fatalf("image: %q", d.ContainerImage)
	}
	if d.PodImageTagLabel != "abc123" {
		t.Fatalf("label: %q", d.PodImageTagLabel)
	}
	if !d.IsReady() {
		t.Fatal("expected ready")
	}
}

func TestParseDeploymentRollout_rolloutInProgress(t *testing.T) {
	raw := json.RawMessage(`{
		"metadata": {"name": "app", "generation": 5},
		"status": {
			"replicas": 1,
			"readyReplicas": 0,
			"updatedReplicas": 1,
			"availableReplicas": 0,
			"observedGeneration": 4,
			"conditions": [
				{"type": "Progressing", "status": "True", "reason": "ReplicaSetUpdated"}
			]
		}
	}`)
	st, err := ParseDeploymentRollout(raw)
	if err != nil {
		t.Fatal(err)
	}
	if st.IsReady() {
		t.Fatal("rollout still in progress")
	}
	if !strings.Contains(st.Summary(), "đang rollout") {
		t.Fatalf("summary should mention rollout: %q", st.Summary())
	}
}
