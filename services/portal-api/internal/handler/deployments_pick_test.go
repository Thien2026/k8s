package handler

import "testing"

func TestPickCurrentDeployment_ServingTagWinsOverStaleRunning(t *testing.T) {
	items := []deploymentRow{
		{ID: 82, Environment: "dev", ImageTag: "afcef05f05493ec886520ac763728fb3915ba778", Status: "success", BuildStatus: "success", CreatedAt: "2026-06-26T15:26:09Z"},
		{ID: 1, Environment: "dev", ImageTag: "test", Status: "failed", BuildStatus: "running", CreatedAt: "2026-06-23T14:30:00Z"},
	}
	idx := pickCurrentDeploymentIndex(items, "dev", "afcef05f05493ec886520ac763728fb3915ba778")
	if idx != 0 {
		t.Fatalf("expected serving success deploy, got index %d (%s)", idx, items[idx].ImageTag)
	}
}

func TestPickCurrentDeployment_ActiveInProgressFirst(t *testing.T) {
	items := []deploymentRow{
		{ID: 3, Environment: "dev", ImageTag: "newsha111", Status: "in_progress", BuildStatus: "running", CreatedAt: "2026-06-26T16:00:00Z"},
		{ID: 2, Environment: "dev", ImageTag: "oldsha222", Status: "success", BuildStatus: "success", CreatedAt: "2026-06-26T15:00:00Z"},
	}
	idx := pickCurrentDeploymentIndex(items, "dev", "oldsha222")
	if idx != 0 {
		t.Fatalf("expected in-progress deploy, got %d", idx)
	}
}

func TestNormalizeStaleDeploymentRow(t *testing.T) {
	d := &deploymentRow{Status: "failed", BuildStatus: "running", ErrorPhase: "runtime"}
	normalizeStaleDeploymentRow(d)
	if d.BuildStatus != "success" {
		t.Fatalf("want build success on runtime-failed stale row, got %q", d.BuildStatus)
	}
}
