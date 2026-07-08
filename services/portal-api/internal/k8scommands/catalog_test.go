package k8scommands

import "testing"

func TestListProjectScope(t *testing.T) {
	list := List("dev", "project", false)
	if len(list) < 20 {
		t.Fatalf("expected many project commands, got %d", len(list))
	}
	for _, c := range list {
		if c.InfraOnly {
			t.Fatalf("dev should not see infra command %s", c.ID)
		}
	}
}

func TestListPlatformInfra(t *testing.T) {
	list := List("admin", "platform", true)
	foundNodes := false
	for _, c := range list {
		if c.ID == "nodes_list" {
			foundNodes = true
		}
	}
	if !foundNodes {
		t.Fatal("admin platform should include nodes_list")
	}
}

func TestListKeyFromID(t *testing.T) {
	key, ok := ListKeyFromID("pvc_list")
	if !ok || key != "persistentvolumeclaims" {
		t.Fatalf("unexpected: %s %v", key, ok)
	}
}
