package handler

import "testing"

func TestPickCurrentDeployment_ServingTagWinsOverStaleRunning(t *testing.T) {
	items := []deploymentRow{
		{ID: 82, Environment: "dev", ImageTag: "afcef05f05493ec886520ac763728fb3915ba778", Status: "success", BuildStatus: "success", CreatedAt: "2026-06-26T15:26:09Z"},
		{ID: 1, Environment: "dev", ImageTag: "test", Status: "failed", BuildStatus: "running", CreatedAt: "2026-06-23T14:30:00Z"},
	}
	idx := pickCurrentDeploymentIndex(items, "dev", "afcef05f05493ec886520ac763728fb3915ba778")
	if idx != 0 {
		t.Fatalf("expected serving success deploy, got index %d (%s)", idx, items[idx].ImageTag)
	}
}

func TestPickCurrentDeployment_ActiveInProgressFirst(t *testing.T) {
	items := []deploymentRow{
		{ID: 3, Environment: "dev", ImageTag: "newsha111", Status: "in_progress", BuildStatus: "running", CreatedAt: "2026-06-26T16:00:00Z"},
		{ID: 2, Environment: "dev", ImageTag: "oldsha222", Status: "success", BuildStatus: "success", CreatedAt: "2026-06-26T15:00:00Z"},
	}
	idx := pickCurrentDeploymentIndex(items, "dev", "oldsha222")
	if idx != 0 {
		t.Fatalf("expected in-progress deploy, got %d", idx)
	}
}

func TestPickCurrentDeployment_SkipsSupersededInProgress(t *testing.T) {
	items := []deploymentRow{
		{ID: 113, Environment: "dev", ImageTag: "2891894105e7", Status: "success", BuildStatus: "success", DeployStatus: "success", RuntimeStatus: "success", CreatedAt: "2026-06-29T22:10:00Z"},
		{ID: 112, Environment: "dev", ImageTag: "7d9e7cb89696", Status: "success", BuildStatus: "success", DeployStatus: "success", RuntimeStatus: "success", CreatedAt: "2026-06-29T22:05:00Z"},
		{ID: 109, Environment: "dev", ImageTag: "6724a0cf0106", Status: "in_progress", BuildStatus: "success", DeployStatus: "success", RuntimeStatus: "running", CreatedAt: "2026-06-29T21:00:00Z"},
	}
	idx := pickCurrentDeploymentIndex(items, "dev", "2891894105e7")
	if idx != 0 {
		t.Fatalf("expected newest success, got index %d (tag %s)", idx, items[idx].ImageTag)
	}
}

func TestMergeGhDeployment_KeepsTerminalSuccess(t *testing.T) {
	existing := deploymentRow{
		ID: 10, Environment: "dev", ImageTag: "e76a87030c5f", Status: "success",
		BuildStatus: "success", DeployStatus: "success", RuntimeStatus: "success",
	}
	g := deploymentRow{
		ImageTag: "e76a870", Status: "in_progress", BuildStatus: "running", Live: true,
	}
	out := mergeGhDeployment(existing, g)
	if out.Status != "success" {
		t.Fatalf("want success preserved, got %q", out.Status)
	}
	if out.Live {
		t.Fatal("terminal success should not be live")
	}
}

func TestPickCurrentDeployment_SkipsTrafficGateStaleSingle(t *testing.T) {
	items := []deploymentRow{
		{ID: 162, Environment: "dev", ImageTag: "f16a9be4f4bb", Status: "failed", DeployLayout: "multi", CreatedAt: "2026-07-01T15:03:00Z"},
		{ID: 159, Environment: "dev", ImageTag: "ef8c0036a8f9", Status: "in_progress", DeployLayout: "single",
			RuntimeDetail: "Chưa xác định image đang phục vụ trên cluster", CreatedAt: "2026-07-01T14:59:00Z"},
	}
	idx := pickCurrentDeploymentIndex(items, "dev", "f16a9be4f4bb")
	if idx != 0 {
		t.Fatalf("expected serving f16a9be row, got %d tag %s", idx, items[idx].ImageTag)
	}
}

func TestDedupeDeploymentsByTag_PrefersSuccessOverStaleInProgress(t *testing.T) {
	items := []deploymentRow{
		{ID: 162, Environment: "dev", ImageTag: "f16a9be4f4bb", Status: "success", CreatedAt: "2026-07-01T15:12:00Z"},
		{ID: 157, Environment: "dev", ImageTag: "f16a9be4f4bb", Status: "in_progress", CreatedAt: "2026-07-01T14:55:00Z"},
		{ID: 159, Environment: "dev", ImageTag: "ef8c0036a8f9", Status: "failed", CreatedAt: "2026-07-01T14:59:00Z"},
	}
	out := dedupeDeploymentsByTag(items)
	if len(out) != 2 {
		t.Fatalf("want 2 tags, got %d", len(out))
	}
	for _, d := range out {
		if imageTagsMatch(d.ImageTag, "f16a9be4f4bb") && d.ID != 162 {
			t.Fatalf("f16a9be should keep id 162, got %d status %s", d.ID, d.Status)
		}
	}
}

func TestPickCurrentDeployment_SkipsStaleRuntimePollRow(t *testing.T) {
	items := []deploymentRow{
		{ID: 163, Environment: "dev", ImageTag: "913136e4648b6", Status: "in_progress", DeployStatus: "success", RuntimeStatus: "running", CreatedAt: "2026-07-01T22:29:00Z"},
		{ID: 164, Environment: "dev", ImageTag: "ef8c0036a8f9", Status: "failed", DeployStatus: "success", CreatedAt: "2026-07-01T23:00:00Z"},
		{ID: 159, Environment: "dev", ImageTag: "ef8c0036a8f9", Status: "success", DeployStatus: "success", RuntimeStatus: "success", CreatedAt: "2026-07-01T14:59:00Z"},
	}
	idx := pickCurrentDeploymentIndex(items, "dev", "ef8c0036a8f9")
	if idx != 2 {
		t.Fatalf("expected ef8c003 success row 159, got index %d (id %d tag %s status %s)", idx, items[idx].ID, items[idx].ImageTag, items[idx].Status)
	}
}

func TestPickIndexForServingTag_PrefersSuccess(t *testing.T) {
	items := []deploymentRow{
		{ID: 164, Environment: "dev", ImageTag: "ef8c0036a8f9", Status: "failed"},
		{ID: 159, Environment: "dev", ImageTag: "ef8c0036a8f9", Status: "success", RuntimeStatus: "success"},
	}
	idx := pickIndexForServingTag(items, "dev", "ef8c0036a8f9")
	if idx != 1 {
		t.Fatalf("expected success row 159, got %d", items[idx].ID)
	}
}

func TestDedupeDeploymentsByTag_KeepsActiveInProgress(t *testing.T) {
	items := []deploymentRow{
		{ID: 165, Environment: "dev", ImageTag: "ef8c0036a8f9", Status: "in_progress", DeployStatus: "running", CreatedAt: "2026-07-02T03:20:00Z"},
		{ID: 164, Environment: "dev", ImageTag: "ef8c0036a8f9", Status: "success", CreatedAt: "2026-07-02T03:10:00Z"},
	}
	out := dedupeDeploymentsByTag(items)
	if len(out) != 1 || out[0].ID != 165 {
		t.Fatalf("want active rollback row 165, got %+v", out)
	}
}

func TestDeploymentSupersededByNewerSuccess_SameTagOnly(t *testing.T) {
	items := []deploymentRow{
		{ID: 164, Environment: "dev", ImageTag: "ef8c0036a8f9", Status: "success"},
		{ID: 163, Environment: "dev", ImageTag: "913136e4648b", Status: "in_progress"},
	}
	if deploymentSupersededByNewerSuccess(items, "dev", 1) {
		t.Fatal("different tag in_progress should not be superseded by unrelated success")
	}
	items[0] = deploymentRow{ID: 166, Environment: "dev", ImageTag: "913136e4648b", Status: "success"}
	if !deploymentSupersededByNewerSuccess(items, "dev", 1) {
		t.Fatal("same tag newer success should supersede older in_progress")
	}
}

func TestNormalizeStaleDeploymentRow(t *testing.T) {
	d := &deploymentRow{Status: "failed", BuildStatus: "running", ErrorPhase: "runtime"}
	normalizeStaleDeploymentRow(d)
	if d.BuildStatus != "success" {
		t.Fatalf("want build success on runtime-failed stale row, got %q", d.BuildStatus)
	}
}
