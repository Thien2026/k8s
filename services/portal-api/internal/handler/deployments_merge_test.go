package handler

import "testing"

func TestMergeDeploymentItemsKeepsDevAndProdSameTag(t *testing.T) {
	dbItems := []deploymentRow{
		{ID: 2, Environment: "prod", ImageTag: "c906db4", Status: "in_progress", CreatedAt: "2026-06-24T20:00:00Z"},
		{ID: 1, Environment: "dev", ImageTag: "c906db4", Status: "success", CreatedAt: "2026-06-24T19:00:00Z"},
	}
	ghItems := []deploymentRow{
		{Environment: "dev", ImageTag: "c906db4", GitHubRunID: 99, BuildStatus: "success", GitHubRunURL: "https://github.com/run/99"},
	}
	merged := mergeDeploymentItems(dbItems, ghItems)
	if len(merged) != 2 {
		t.Fatalf("expected 2 items, got %d", len(merged))
	}
	var prod, dev *deploymentRow
	for i := range merged {
		switch merged[i].Environment {
		case "prod":
			prod = &merged[i]
		case "dev":
			dev = &merged[i]
		}
	}
	if prod == nil || dev == nil {
		t.Fatal("missing prod or dev row after merge")
	}
	if prod.ID != 2 {
		t.Fatalf("prod id=%d want 2", prod.ID)
	}
	if dev.GitHubRunID != 99 {
		t.Fatalf("dev github_run_id=%d want 99", dev.GitHubRunID)
	}
}

func TestMergeGhDeploymentDoesNotPromoteSuccessFromBuildOnly(t *testing.T) {
	existing := deploymentRow{
		ID:             10,
		Environment:    "dev",
		ImageTag:       "9897ba5",
		Status:         "in_progress",
		BuildStatus:    "running",
		DeployStatus:   "pending",
		RuntimeStatus:  "pending",
		DeployLayout:   "multi",
		DeployProfile:  "multi · api+web",
	}
	g := deploymentRow{
		Environment:    "dev",
		ImageTag:       "9897ba5",
		Status:         "success", // GH row sai: build xong nhưng deploy chưa
		BuildStatus:    "success",
		RegistryStatus: "success",
		DeployStatus:   "pending",
		RuntimeStatus:  "pending",
		GitHubRunID:    42,
	}
	out := mergeGhDeployment(existing, g)
	if out.Status != "in_progress" {
		t.Fatalf("status=%q want in_progress (không tô success từ GH)", out.Status)
	}
	if out.BuildStatus != "success" {
		t.Fatalf("build_status=%q want success", out.BuildStatus)
	}
	if out.GitHubRunID != 42 {
		t.Fatalf("github_run_id=%d", out.GitHubRunID)
	}
}
