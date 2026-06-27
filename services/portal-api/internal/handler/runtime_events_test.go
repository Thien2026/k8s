package handler

import (
	"strings"
	"testing"

	"github.com/Thien2026/k8s/services/portal-api/internal/rancher"
)

func TestEvaluateDeploymentImage_match(t *testing.T) {
	st, detail := evaluateDeploymentImage(
		"harbor.example.com/research-labs/app:abc123",
		"abc123",
		"harbor.example.com/research-labs/app:abc123",
		rancher.DeploymentRolloutStatus{ReadyReplicas: 1, AvailableReplicas: 1},
	)
	if st != "success" {
		t.Fatalf("expected success, got %s (%s)", st, detail)
	}
}

func TestEvaluateDeploymentImage_mismatchReady(t *testing.T) {
	st, detail := evaluateDeploymentImage(
		"harbor.example.com/research-labs/app:oldtag",
		"newtag",
		"harbor.example.com/research-labs/app:newtag",
		rancher.DeploymentRolloutStatus{ReadyReplicas: 1, AvailableReplicas: 1},
	)
	if st != "failed" {
		t.Fatalf("expected failed, got %s (%s)", st, detail)
	}
}

func TestBuildDeployRuntimeEvents_expand(t *testing.T) {
	v := buildDeployRuntimeEvents([]rancher.ResourceRow{
		{Reason: "BackOff", Message: "restarting", Object: "Pod/app-1", Created: "2026-06-26T08:00:00Z"},
		{Reason: "Unhealthy", Message: "probe failed", Object: "Pod/app-1", Created: "2026-06-26T08:00:01Z"},
	}, "app-1")
	if v.Status != "failed" {
		t.Fatalf("expected failed, got %s", v.Status)
	}
	if len(v.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(v.Items))
	}
	if !strings.Contains(v.Detail, "2 events") {
		t.Fatalf("headline should count events: %q", v.Detail)
	}
}

func TestFormatDeployRuntimeEvents_clean(t *testing.T) {
	st, _ := formatDeployRuntimeEvents([]rancher.ResourceRow{
		{Reason: "Scheduled", Message: "Successfully assigned", Object: "Pod/app-xyz"},
	}, "app-xyz")
	if st != "success" {
		t.Fatalf("expected success for non-warning event, got %s", st)
	}
}
