package handler

import (
	"testing"

	"github.com/Thien2026/k8s/services/portal-api/internal/rancher"
)

func TestClassifyPodHealth_Running(t *testing.T) {
	if got := classifyPodHealth(rancher.ResourceRow{Status: "Running", Ready: true}); got != "running" {
		t.Fatalf("want running, got %q", got)
	}
}

func TestClassifyPodHealth_CrashLoop(t *testing.T) {
	if got := classifyPodHealth(rancher.ResourceRow{Status: "CrashLoopBackOff"}); got != "crash_loop" {
		t.Fatalf("want crash_loop, got %q", got)
	}
}

func TestClassifyPodHealth_OOM(t *testing.T) {
	pod := rancher.ResourceRow{Status: "Running", LastTerminationReason: "OOMKilled", Restarts: 3}
	if got := classifyPodHealth(pod); got != "oom_killed" {
		t.Fatalf("want oom_killed, got %q", got)
	}
}

func TestClassifyPodHealth_Restarting(t *testing.T) {
	pod := rancher.ResourceRow{Status: "Running", Ready: false, Restarts: 2}
	if got := classifyPodHealth(pod); got != "restarting" {
		t.Fatalf("want restarting, got %q", got)
	}
}

func TestSummarizeWorkloadHealth(t *testing.T) {
	sum := summarizeWorkloadHealth([]workloadPodView{
		{Health: "running", Restarts: 0},
		{Health: "crash_loop", Restarts: 5},
	})
	if sum.Overall != "crash_loop" {
		t.Fatalf("overall=%q", sum.Overall)
	}
	if sum.TotalRestarts != 5 || sum.PodsCrashLoop != 1 {
		t.Fatalf("unexpected summary: %+v", sum)
	}
}
