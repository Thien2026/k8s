package deploy

import "testing"

func TestWorkflowProfileKeySingle(t *testing.T) {
	layout, svcs := WorkflowProfileKey(LayoutSingle, nil)
	if layout != LayoutSingle || svcs != "app" {
		t.Fatalf("got %s/%s", layout, svcs)
	}
}

func TestWorkflowProfileKeyMultiSorted(t *testing.T) {
	_, svcs := WorkflowProfileKey(LayoutMulti, []ServiceDef{
		{Name: "web"},
		{Name: "api"},
		{Name: "worker"},
	})
	if svcs != "api,web,worker" {
		t.Fatalf("want sorted names, got %s", svcs)
	}
}

func TestWorkflowProfileMatches(t *testing.T) {
	svcs := []ServiceDef{{Name: "api"}, {Name: "web"}}
	if !WorkflowProfileMatches(LayoutMulti, "api,web", LayoutMulti, svcs) {
		t.Fatal("expected match")
	}
	if WorkflowProfileMatches(LayoutSingle, "app", LayoutMulti, svcs) {
		t.Fatal("layout mismatch should fail")
	}
	if WorkflowProfileMatches(LayoutMulti, "api,web,worker", LayoutMulti, svcs) {
		t.Fatal("service list mismatch should fail")
	}
}
