package handler

import (
	"testing"

	"github.com/Thien2026/k8s/services/portal-api/internal/rancher"
)

func TestRuntimeLogHint_ErrorLine(t *testing.T) {
	hint := runtimeLogHint("Error: '' is not a valid port number.\n")
	if hint == "" {
		t.Fatal("expected log hint")
	}
}

func TestMergeRuntimeHealth_K8sFailed(t *testing.T) {
	v := mergeRuntimeHealth("failed", "CrashLoopBackOff", "app-1", "pod crash", "skipped", "", "Error: boom", nil)
	if v.Status != "failed" {
		t.Fatalf("expected failed, got %s", v.Status)
	}
}

func TestMergeRuntimeHealth_SmokeFailed(t *testing.T) {
	v := mergeRuntimeHealth("success", "Ready", "app-1", "", "failed", "HTTP 503 tại /health", "", nil)
	if v.Status != "failed" {
		t.Fatalf("expected failed, got %s", v.Status)
	}
}

func TestMergeRuntimeHealth_AllOK(t *testing.T) {
	v := mergeRuntimeHealth("success", "Ready", "app-1", "", "success", "HTTP 200 tại /health", "", nil)
	if v.Status != "success" {
		t.Fatalf("expected success, got %s", v.Status)
	}
}

func TestEvaluateK8sPodTier_WaitProbe(t *testing.T) {
	st, detail, _, _ := evaluateK8sPodTier([]rancher.ResourceRow{
		{Name: "app-abc", Status: "Running", Ready: false},
	})
	if st != "running" {
		t.Fatalf("expected running, got %s", st)
	}
	if detail == "" {
		t.Fatal("expected detail")
	}
}

func TestDeploymentFailedMayRecover(t *testing.T) {
	d := deploymentRow{Status: "failed", ErrorPhase: "runtime", ErrorMessage: "Smoke check: HTTP 503 tại /health"}
	if !deploymentFailedMayRecover(d) {
		t.Fatal("smoke false fail should recover")
	}
	d.ErrorPhase = "build"
	if deploymentFailedMayRecover(d) {
		t.Fatal("build fail should not recover")
	}
}

func TestMergeRuntimeHealth_SmokeOKWhileK8sRunning(t *testing.T) {
	v := mergeRuntimeHealth("running", "app-1: Running (NotReady)", "app-1", "", "success", "HTTP 200 tại /health", "", nil)
	if v.Status != "running" {
		t.Fatalf("expected running until rollout done, got %s", v.Status)
	}
}

func TestEvaluateK8sPodTier_Ready(t *testing.T) {
	st, _, _, _ := evaluateK8sPodTier([]rancher.ResourceRow{
		{Name: "app-abc", Status: "Running", Ready: true},
	})
	if st != "success" {
		t.Fatalf("expected success, got %s", st)
	}
}
