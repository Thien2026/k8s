package k8scommands

import "testing"

func TestListProjectScope(t *testing.T) {
	list := List("dev", "project", false)
	if len(list) == 0 {
		t.Fatal("expected project commands")
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
