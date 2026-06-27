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
