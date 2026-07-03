package handler

import (
	"context"
	"testing"

	"github.com/Thien2026/k8s/services/portal-api/internal/deploy"
)

func TestLayoutUserLabel(t *testing.T) {
	if layoutUserLabel(deploy.LayoutMulti) != "Web + API riêng" {
		t.Fatal("multi label")
	}
	if layoutUserLabel(deploy.LayoutSingle) != "Một website" {
		t.Fatal("single label")
	}
}

func TestDeployProfileLabel(t *testing.T) {
	if got := deployProfileLabel("multi", []deployServiceSnap{{Name: "api"}, {Name: "web"}}); got != "multi · api+web" {
		t.Fatalf("multi: %q", got)
	}
	if got := deployProfileLabel("single", []deployServiceSnap{{Name: "app"}}); got != "single · app" {
		t.Fatalf("single: %q", got)
	}
}

func TestSnapServicesToDefs_ApiWebIngress(t *testing.T) {
	defs := snapServicesToDefs(
		[]deployServiceSnap{{Name: "api"}, {Name: "web"}},
		nil,
	)
	if len(defs) != 2 {
		t.Fatalf("len=%d", len(defs))
	}
	routes := deploy.IngressRoutesFromServices(defs)
	if len(routes) < 2 {
		t.Fatalf("routes thiếu path api/web: %v", routes)
	}
}

func TestIngressRoutesForParams_SingleAppRoute(t *testing.T) {
	routes := ingressRoutesForParams(deploy.Params{Layout: deploy.LayoutSingle})
	if len(routes) != 1 || routes[0].ServiceName != "app" {
		t.Fatalf("single routes: %v", routes)
	}
}

func TestIngressRoutesForParams_MultiDefault(t *testing.T) {
	routes := ingressRoutesForParams(deploy.Params{Layout: deploy.LayoutMulti})
	if len(routes) < 2 {
		t.Fatalf("multi routes: %v", routes)
	}
}

func TestSmokePathsForDeployment_SingleUsesAppPaths(t *testing.T) {
	h := &Handler{}
	paths := h.smokePathsForDeployment(context.Background(), 1, projectRepoRow{}, &deploymentRow{
		DeployLayout:   deploy.LayoutSingle,
		DeployServices: []deployServiceSnap{{Name: "app"}},
	})
	if len(paths) != 2 || paths[0] != "/health" || paths[1] != "/" {
		t.Fatalf("single paths: %v", paths)
	}
}
