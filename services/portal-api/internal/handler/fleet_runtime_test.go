package handler

import (
	"testing"

	"github.com/Thien2026/k8s/services/portal-api/internal/deploy"
	"github.com/Thien2026/k8s/services/portal-api/internal/rancher"
)

func TestImageTagsMatch(t *testing.T) {
	if !imageTagsMatch("e1e626012c79", "e1e6260") {
		t.Fatal("prefix match")
	}
	if imageTagsMatch("abc", "def") {
		t.Fatal("different tags")
	}
}

func TestUnanimousServingTagMixed(t *testing.T) {
	got := unanimousServingTag(map[string]string{
		"api":  "783611a",
		"web":  "783611a",
		"node": "e1e6260",
	}, 3)
	if got != "783611a" {
		t.Fatalf("want dominant legacy tag, got %s", got)
	}
}

func TestUnanimousServingTagAllSame(t *testing.T) {
	got := unanimousServingTag(map[string]string{
		"api": "e1e6260",
		"web": "e1e6260",
	}, 2)
	if got != "e1e6260" {
		t.Fatalf("want unanimous tag, got %s", got)
	}
}

func TestFilterFleetRolloutServicesSkipsUndeployed(t *testing.T) {
	services := []deploy.ServiceDef{
		{Name: "api", ExposeIngress: true},
		{Name: "web", ExposeIngress: true},
		{Name: "node", ExposeIngress: false, IngressPath: "internal"},
		{Name: "worker", ExposeIngress: false, IngressPath: "-"},
	}
	got := filterFleetRolloutServices(services, func(name string) bool {
		return name == "api" || name == "web"
	})
	if len(got) != 2 || got[0].Name != "api" || got[1].Name != "web" {
		t.Fatalf("want api+web on cluster, got %+v", got)
	}
	// Chưa deploy gì — chỉ yêu cầu public
	got2 := filterFleetRolloutServices(services, func(string) bool { return false })
	if len(got2) != 2 {
		t.Fatalf("want public only, got %d", len(got2))
	}
}

func TestEvaluateFleetRolloutBlocked(t *testing.T) {
	h := &Handler{}
	// evaluateFleetRollout needs rancher — test via pod helpers only here
	st, _, _, _ := evaluateK8sPodTier([]rancher.ResourceRow{
		{Name: "node-abc", Status: "CrashLoopBackOff", Ready: false},
	})
	if st != "failed" {
		t.Fatalf("expected failed, got %s", st)
	}
	_ = h
}
