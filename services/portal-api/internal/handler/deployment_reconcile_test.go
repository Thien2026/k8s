package handler

import "testing"

func TestSyncBuildStatusFromSteps(t *testing.T) {
	d := &deploymentRow{
		BuildStatus: "running",
		BuildSteps: []buildStepView{
			{Name: "checkout", Status: "success"},
			{Name: "build", Status: "success"},
			{Name: "deploy", Status: "success"},
		},
	}
	syncBuildStatusFromSteps(d)
	if d.BuildStatus != "success" {
		t.Fatalf("expected success from steps, got %q", d.BuildStatus)
	}
}

func TestFinalizeBuildStepsTruth(t *testing.T) {
	steps := []buildStepView{
		{Name: "checkout", Status: "success"},
		{Name: "validate", Status: "failed"},
		{Name: "build", Status: "pending"},
		{Name: "deploy", Status: "running"},
	}
	out := finalizeBuildStepsTruth(steps)
	if out[2].Status != "skipped" || out[3].Status != "skipped" {
		t.Fatalf("expected skipped after fail: %+v", out)
	}
}

func TestReconcileBuildFromStepsClearsGitHubFalseFail(t *testing.T) {
	d := &deploymentRow{
		BuildStatus:  "failed",
		Status:       "failed",
		ErrorPhase:   "build",
		ErrorMessage: "GitHub Actions: failure",
		BuildSteps: []buildStepView{
			{Name: "checkout", Status: "success"},
			{Name: "build", Status: "success"},
			{Name: "Deploy to Platform", Status: "success"},
		},
	}
	reconcileBuildFromSteps(d)
	if d.BuildStatus != "success" {
		t.Fatalf("build=%s", d.BuildStatus)
	}
	if d.ErrorMessage != "" {
		t.Fatalf("error should clear, got %q", d.ErrorMessage)
	}
	reconcileDeploymentRow(d)
	if d.Status == "failed" {
		t.Fatalf("should not stay failed after clear")
	}
}

func TestReconcileBuildFailSkipsDownstream(t *testing.T) {
	d := &deploymentRow{
		BuildStatus:    "failed",
		RegistryStatus: "pending",
		DeployStatus:   "running",
		RuntimeStatus:  "pending",
		ErrorPhase:     "build",
		ErrorMessage:   "env missing",
	}
	reconcileDeploymentRow(d)
	if d.Status != "failed" {
		t.Fatalf("status=%s", d.Status)
	}
	if d.RegistryStatus != "skipped" || d.DeployStatus != "skipped" || d.RuntimeStatus != "skipped" {
		t.Fatalf("downstream not skipped: reg=%s dep=%s rt=%s", d.RegistryStatus, d.DeployStatus, d.RuntimeStatus)
	}
	stages := (&Handler{}).deploymentStages(d)
	if len(stages) < 4 {
		t.Fatal("expected stages")
	}
	if stages[1].Status != "skipped" || stages[2].Status != "skipped" {
		t.Fatalf("stages: %+v", stages)
	}
}
