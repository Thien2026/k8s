package handler

import "testing"

func TestDeployProfileLabel(t *testing.T) {
	if got := deployProfileLabel("multi", []deployServiceSnap{{Name: "api"}, {Name: "web"}}); got != "multi · api+web" {
		t.Fatalf("multi: %q", got)
	}
	if got := deployProfileLabel("single", []deployServiceSnap{{Name: "app"}}); got != "single · app" {
		t.Fatalf("single: %q", got)
	}
}
