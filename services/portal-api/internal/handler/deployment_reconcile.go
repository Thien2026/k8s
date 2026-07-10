package handler

import "strings"

// reconcileDeploymentRow đồng bộ status tổng + downstream từ các giai đoạn thật (không tô xanh giả).
// reconcileBuildFromSteps — steps xanh thì không giữ failed từ workflow conclusion cũ.
func reconcileBuildFromSteps(d *deploymentRow) {
	if d == nil {
		return
	}
	syncBuildStatusFromSteps(d)
	if !allBuildStepsSuccess(d.BuildSteps) {
		return
	}
	d.BuildStatus = "success"
	phase := strings.ToLower(strings.TrimSpace(d.ErrorPhase))
	msg := strings.ToLower(strings.TrimSpace(d.ErrorMessage))
	if phase == "build" || strings.HasPrefix(msg, "github actions:") {
		d.ErrorPhase = ""
		d.ErrorMessage = ""
	}
}

func reconcileDeploymentRow(d *deploymentRow) {
	if d == nil {
		return
	}
	reconcilePlatformHookFalseFailure(d)
	propagateSkippedDownstream(d)

	if strings.TrimSpace(d.ErrorMessage) != "" {
		d.Status = "failed"
		d.Live = false
		return
	}

	bs := stageStatus(d.BuildStatus)
	if bs == "failed" {
		d.Status = "failed"
		if strings.TrimSpace(d.ErrorPhase) == "" {
			d.ErrorPhase = "build"
		}
		d.Live = false
		return
	}
	if bs == "running" || bs == "pending" {
		d.Status = "in_progress"
		d.Live = true
		return
	}

	reg := stageStatus(d.RegistryStatus)
	dep := stageStatus(d.DeployStatus)
	rt := stageStatus(d.RuntimeStatus)

	if reg == "failed" || dep == "failed" || rt == "failed" {
		d.Status = "failed"
		d.Live = false
		return
	}
	if strings.EqualFold(strings.TrimSpace(d.SmokeStatus), "failed") {
		d.Status = "failed"
		if strings.TrimSpace(d.ErrorPhase) == "" {
			d.ErrorPhase = "runtime"
		}
		if strings.TrimSpace(d.ErrorMessage) == "" && strings.TrimSpace(d.SmokeDetail) != "" {
			d.ErrorMessage = "Smoke check: " + d.SmokeDetail
		}
		d.Live = false
		return
	}

	if reg == "running" || dep == "running" || rt == "running" || reg == "pending" || dep == "pending" || rt == "pending" {
		d.Status = "in_progress"
		d.Live = true
		return
	}

	if bs == "success" && dep == "success" && rt == "success" {
		smoke := strings.ToLower(strings.TrimSpace(d.SmokeStatus))
		if smoke == "failed" {
			d.Status = "failed"
			d.Live = false
			return
		}
		if smoke == "success" || smoke == "skipped" || smoke == "" {
			d.Status = "success"
			d.Live = false
			return
		}
		if smoke == "running" || smoke == "pending" {
			d.Status = "in_progress"
			d.Live = true
			return
		}
	}

	if deploymentIsTerminal(*d) {
		d.Live = false
	}
}

func propagateSkippedDownstream(d *deploymentRow) {
	bs := strings.ToLower(strings.TrimSpace(d.BuildStatus))
	if bs != "failed" && bs != "cancelled" {
		return
	}
	// Cluster đã deploy — không ghi đè khi chỉ hook GitHub fail ảo.
	if stageStatus(d.DeployStatus) == "success" {
		return
	}
	if (stageStatus(d.RuntimeStatus) == "success" || stageStatus(d.RuntimeStatus) == "running") &&
		strings.HasPrefix(strings.ToLower(strings.TrimSpace(d.ErrorMessage)), "github actions:") {
		return
	}
	markSkipped := func(field *string) {
		v := strings.ToLower(strings.TrimSpace(*field))
		if v == "" || v == "pending" || v == "running" || v == "in_progress" || v == "building" || v == "deploying" || v == "queued" || v == "waiting" {
			*field = "skipped"
		}
	}
	markSkipped(&d.RegistryStatus)
	markSkipped(&d.DeployStatus)
	markSkipped(&d.RuntimeStatus)
}

func allBuildStepsSuccess(steps []buildStepView) bool {
	if len(steps) == 0 {
		return false
	}
	for _, s := range steps {
		st := stageStatus(s.Status)
		if st != "success" && st != "skipped" {
			return false
		}
	}
	return true
}

// syncBuildStatusFromSteps suy ra build_status từ GH steps khi DB/run API chưa kịp cập nhật.
func syncBuildStatusFromSteps(d *deploymentRow) {
	if d == nil || len(d.BuildSteps) == 0 {
		return
	}
	hasFailed := false
	allDone := true
	for _, s := range d.BuildSteps {
		st := stageStatus(s.Status)
		if st == "failed" {
			hasFailed = true
		}
		if st == "running" || st == "pending" {
			allDone = false
		}
	}
	cur := stageStatus(d.BuildStatus)
	if hasFailed && cur != "success" {
		d.BuildStatus = "failed"
	} else if allDone && !hasFailed && cur != "failed" {
		d.BuildStatus = "success"
	}
}

func finalizeBuildStepsTruth(steps []buildStepView) []buildStepView {
	if len(steps) == 0 {
		return steps
	}
	out := make([]buildStepView, 0, len(steps))
	sawFail := false
	for _, s := range steps {
		cp := s
		name := strings.ToLower(strings.TrimSpace(cp.Name))
		if strings.HasPrefix(name, "post ") {
			continue
		}
		if sawFail && (cp.Status == "pending" || cp.Status == "running") {
			cp.Status = "skipped"
		}
		if cp.Status == "failed" {
			sawFail = true
		}
		out = append(out, cp)
	}
	return out
}

// deploymentFailedMayRecover — failed tạm (smoke 503 lúc rollout, deadline K8s…) có thể sửa khi cluster đã ready.
func deploymentFailedMayRecover(d deploymentRow) bool {
	if !strings.EqualFold(strings.TrimSpace(d.Status), "failed") {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(d.ErrorPhase), "build") {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(d.ErrorMessage))
	if msg == "" {
		return true
	}
	for _, frag := range []string{
		"smoke check",
		"http 503",
		"minimum availability",
		"timed out progressing",
		"progressing:",
		"connection refused",
		"containercreating",
		"certificate is valid for ingress.local",
		"failed to verify certificate",
		"không tồn tại trên cluster",
		"không có pod",
		"đang chờ argocd",
	} {
		if strings.Contains(msg, frag) {
			return true
		}
	}
	return false
}

// reconcilePlatformHookFalseFailure — build/push OK, cluster đã deploy nhưng step curl hook GitHub fail (502/timeout).
func reconcilePlatformHookFalseFailure(d *deploymentRow) {
	if d == nil {
		return
	}
	dep := stageStatus(d.DeployStatus)
	rt := stageStatus(d.RuntimeStatus)
	if dep != "success" && dep != "running" && rt != "success" && rt != "running" {
		return
	}
	msg := strings.ToLower(strings.TrimSpace(d.ErrorMessage))
	if !strings.HasPrefix(msg, "github actions:") && d.ErrorPhase != "build" {
		return
	}
	buildOK := len(d.BuildSteps) > 0
	hookFailed := false
	for _, s := range d.BuildSteps {
		name := strings.ToLower(strings.TrimSpace(s.Name))
		st := stageStatus(s.Status)
		if strings.Contains(name, "deploy to platform") {
			if st == "failed" {
				hookFailed = true
			}
			continue
		}
		if strings.HasPrefix(name, "post ") {
			continue
		}
		if st != "success" && st != "skipped" {
			buildOK = false
		}
	}
	if !buildOK || !hookFailed {
		return
	}
	d.BuildStatus = "success"
	d.ErrorPhase = ""
	d.ErrorMessage = ""
}

func deploymentNeedsFastPoll(d deploymentRow) bool {
	if d.Live {
		return true
	}
	if !deploymentIsTerminal(d) {
		return true
	}
	for _, st := range []string{d.BuildStatus, d.DeployStatus, d.RuntimeStatus} {
		switch stageStatus(st) {
		case "running", "pending":
			return true
		}
	}
	return false
}
