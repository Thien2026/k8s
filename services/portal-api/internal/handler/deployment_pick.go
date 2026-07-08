package handler

import (
	"context"
	"strings"
)

// deploymentSupersededByNewerSuccess — bản in_progress cũ sau khi đã có success mới hơn cùng tag.
func deploymentSupersededByNewerSuccess(items []deploymentRow, env string, idx int) bool {
	if idx < 0 || idx >= len(items) {
		return false
	}
	env = strings.ToLower(strings.TrimSpace(env))
	tag := strings.TrimSpace(items[idx].ImageTag)
	for i := range items {
		if i == idx {
			continue
		}
		if strings.ToLower(strings.TrimSpace(items[i].Environment)) != env {
			continue
		}
		if tag != "" && !imageTagsMatch(items[i].ImageTag, tag) {
			continue
		}
		if items[i].ID > items[idx].ID && strings.EqualFold(strings.TrimSpace(items[i].Status), "success") {
			return true
		}
	}
	return false
}

// deploymentSupersededByTagSibling — cùng image_tag đã có bản success mới hơn (rollback row).
func deploymentSupersededByTagSibling(items []deploymentRow, env string, idx int) bool {
	if idx < 0 || idx >= len(items) {
		return false
	}
	env = strings.ToLower(strings.TrimSpace(env))
	tag := strings.TrimSpace(items[idx].ImageTag)
	if tag == "" {
		return false
	}
	for i := range items {
		if i == idx {
			continue
		}
		if strings.ToLower(strings.TrimSpace(items[i].Environment)) != env {
			continue
		}
		if !imageTagsMatch(items[i].ImageTag, tag) {
			continue
		}
		if items[i].ID > items[idx].ID && strings.EqualFold(strings.TrimSpace(items[i].Status), "success") {
			return true
		}
	}
	return false
}

func deploymentHistoryRank(d deploymentRow) int {
	switch strings.ToLower(strings.TrimSpace(d.Status)) {
	case "success":
		return 3
	case "failed":
		return 2
	default:
		return 1
	}
}

// dedupeDeploymentsByTag — một tag chỉ hiện một row “tốt nhất” (ưu tiên deploy đang chạy, rồi success, rồi ID cao).
func dedupeDeploymentsByTag(items []deploymentRow) []deploymentRow {
	byKey := map[string]deploymentRow{}
	for _, d := range items {
		key := deploymentMergeKey(d)
		existing, ok := byKey[key]
		if !ok {
			byKey[key] = d
			continue
		}
		if deploymentIsOngoingDeploy(d) {
			if !deploymentIsOngoingDeploy(existing) {
				byKey[key] = d
				continue
			}
		}
		if deploymentIsOngoingDeploy(existing) {
			continue
		}
		dRank := deploymentHistoryRank(d)
		eRank := deploymentHistoryRank(existing)
		if dRank > eRank || (dRank == eRank && d.ID > existing.ID) {
			byKey[key] = d
		}
	}
	return deploymentItemsSorted(mapValuesDeployment(byKey))
}

func (h *Handler) isHistoricalDeploymentRow(ctx context.Context, p projectRow, d *deploymentRow) bool {
	if d == nil {
		return false
	}
	serving := strings.TrimSpace(h.clusterServingImageTag(ctx, p, d.Environment))
	if serving == "" {
		return false
	}
	if imageTagsMatch(d.ImageTag, serving) {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(d.Status), "success") &&
		strings.EqualFold(strings.TrimSpace(d.RuntimeStatus), "success") {
		return true
	}
	if strings.TrimSpace(d.FinishedAt) != "" && deploymentIsTerminal(*d) {
		return true
	}
	return false
}

func deploymentHistoricalRuntimeNoise(d deploymentRow) bool {
	if !strings.EqualFold(strings.TrimSpace(d.Status), "failed") {
		return false
	}
	if strings.TrimSpace(d.DeployStatus) != "success" {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(d.ErrorMessage))
	return strings.Contains(msg, "image spec") ||
		strings.Contains(msg, "smoke check") ||
		strings.Contains(msg, "chưa xác định image") ||
		strings.Contains(msg, "rollout fleet")
}

func (h *Handler) repairHistoricalDeploymentRow(ctx context.Context, p projectRow, d *deploymentRow) bool {
	if d == nil || d.ID <= 0 {
		return false
	}
	serving := strings.TrimSpace(h.clusterServingImageTag(ctx, p, d.Environment))
	if !deploymentHistoricalRuntimeNoise(*d) && !isTrafficGateStaleRow(*d) &&
		!isSupersededClusterTrafficRow(*d) && !isStaleRuntimePollRow(*d, serving) {
		return false
	}
	if !h.isHistoricalDeploymentRow(ctx, p, d) && !isTrafficGateStaleRow(*d) &&
		!isSupersededClusterTrafficRow(*d) && !isStaleRuntimePollRow(*d, serving) {
		return false
	}
	d.Status = "success"
	d.RuntimeStatus = "success"
	d.ErrorPhase = ""
	d.ErrorMessage = ""
	if strings.TrimSpace(d.RuntimeDetail) == "" {
		d.RuntimeDetail = "Đã deploy thành công — cluster hiện phục vụ bản khác"
	}
	d.Live = false
	_, _ = h.db.Exec(ctx, `
		UPDATE project_deployments SET status='success', runtime_status='success',
			error_phase='', error_message='', runtime_detail=$1, updated_at=now(),
			finished_at=COALESCE(finished_at, now())
		WHERE id=$2`, d.RuntimeDetail, d.ID)
	reconcileDeploymentRow(d)
	return true
}

func (h *Handler) sealSupersededDeploymentRows(ctx context.Context, projectID int64, env string) {
	env = strings.ToLower(strings.TrimSpace(env))
	if env == "" {
		env = "dev"
	}
	_, _ = h.db.Exec(ctx, `
		UPDATE project_deployments older SET
			status='success', runtime_status='success',
			runtime_detail=CASE WHEN COALESCE(older.runtime_detail,'')='' THEN 'Đã thay bởi deploy mới hơn' ELSE older.runtime_detail END,
			error_phase='', error_message='', updated_at=now(),
			finished_at=COALESCE(older.finished_at, now())
		FROM project_deployments newer
		WHERE older.project_id=$1 AND older.environment=$2
		  AND newer.project_id=older.project_id AND newer.environment=older.environment
		  AND older.image_tag=newer.image_tag AND older.id < newer.id
		  AND newer.status='success' AND newer.runtime_status='success'
		  AND older.status IN ('in_progress','failed')
		  AND COALESCE(older.error_phase,'') <> 'build'`, projectID, env)
}

func normalizeStaleDeploymentRow(d *deploymentRow) {
	if d == nil || !strings.EqualFold(strings.TrimSpace(d.Status), "failed") {
		return
	}
	switch stageStatus(d.BuildStatus) {
	case "running", "pending", "":
		if strings.EqualFold(strings.TrimSpace(d.ErrorPhase), "build") {
			d.BuildStatus = "failed"
		} else {
			d.BuildStatus = "success"
		}
	}
}

func isTrafficGateStaleRow(d deploymentRow) bool {
	if !strings.EqualFold(strings.TrimSpace(d.Status), "in_progress") {
		return false
	}
	// "Chưa xác định image" xuất hiện khi applyTrafficGate không tìm được serving tag
	return strings.Contains(strings.TrimSpace(d.RuntimeDetail), "Chưa xác định image")
}

func isSupersededClusterTrafficRow(d deploymentRow) bool {
	if !strings.EqualFold(strings.TrimSpace(d.Status), "in_progress") {
		return false
	}
	rt := strings.TrimSpace(d.RuntimeDetail)
	return strings.Contains(rt, "Cluster phục vụ") && strings.Contains(rt, "chưa thay traffic")
}

// isStaleRuntimePollRow — deploy đã apply xong nhưng cluster không còn phục vụ tag này (poll cũ).
func isStaleRuntimePollRow(d deploymentRow, servingTag string) bool {
	servingTag = strings.TrimSpace(servingTag)
	if servingTag == "" || imageTagsMatch(d.ImageTag, servingTag) {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(d.DeployStatus), "success") {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(d.Status), "in_progress") ||
		strings.EqualFold(strings.TrimSpace(d.RuntimeStatus), "running")
}

func pickIndexForServingTag(items []deploymentRow, env, servingTag string) int {
	servingTag = strings.TrimSpace(servingTag)
	if servingTag == "" {
		return -1
	}
	env = strings.ToLower(strings.TrimSpace(env))
	best := -1
	bestRank := -1
	for i := range items {
		if strings.ToLower(strings.TrimSpace(items[i].Environment)) != env {
			continue
		}
		if !imageTagsMatch(items[i].ImageTag, servingTag) {
			continue
		}
		rank := deploymentHistoryRank(items[i])
		if rank > bestRank || (rank == bestRank && (best < 0 || items[i].ID > items[best].ID)) {
			best = i
			bestRank = rank
		}
	}
	return best
}

func pickCurrentDeploymentIndex(items []deploymentRow, env, servingTag string) int {
	env = strings.ToLower(strings.TrimSpace(env))
	if env == "" {
		env = "dev"
	}
	servingTag = strings.TrimSpace(servingTag)
	pickActive := func(requireDB bool) int {
		best := -1
		for i := range items {
			if strings.ToLower(strings.TrimSpace(items[i].Environment)) != env {
				continue
			}
			if requireDB && items[i].ID <= 0 {
				continue
			}
			if deploymentSupersededByTagSibling(items, env, i) {
				continue
			}
			if isTrafficGateStaleRow(items[i]) {
				continue
			}
			if isSupersededClusterTrafficRow(items[i]) {
				continue
			}
			if isStaleRuntimePollRow(items[i], servingTag) {
				continue
			}
			if deploymentIsTrulyActive(items[i]) && !deploymentSupersededByNewerSuccess(items, env, i) {
				if best < 0 || items[i].ID > items[best].ID {
					best = i
				}
			}
		}
		if best >= 0 {
			return best
		}
		return -1
	}
	if idx := pickActive(true); idx >= 0 {
		return idx
	}
	if idx := pickActive(false); idx >= 0 {
		return idx
	}
	if idx := pickIndexForServingTag(items, env, servingTag); idx >= 0 {
		return idx
	}
	servingTag = strings.TrimSpace(servingTag)
	if servingTag != "" {
		best := -1
		for i := range items {
			if strings.ToLower(strings.TrimSpace(items[i].Environment)) != env {
				continue
			}
			if imageTagsMatch(items[i].ImageTag, servingTag) {
				if best < 0 || items[i].ID > items[best].ID {
					best = i
				}
			}
		}
		if best >= 0 {
			return best
		}
	}
	bestSuccess := -1
	best := -1
	for i := range items {
		if strings.ToLower(strings.TrimSpace(items[i].Environment)) != env {
			continue
		}
		d := items[i]
		if strings.EqualFold(strings.TrimSpace(d.Status), "success") {
			if bestSuccess < 0 || strings.TrimSpace(d.CreatedAt) > strings.TrimSpace(items[bestSuccess].CreatedAt) {
				bestSuccess = i
			}
		}
		if best < 0 || strings.TrimSpace(d.CreatedAt) > strings.TrimSpace(items[best].CreatedAt) {
			best = i
		}
	}
	if bestSuccess >= 0 {
		return bestSuccess
	}
	return best
}
