package handler

import (
	"fmt"
	"strings"

	"github.com/Thien2026/k8s/services/portal-api/internal/deploy"
	"github.com/Thien2026/k8s/services/portal-api/internal/rancher"
)

func evaluateDeploymentImage(containerImage, tag, wantImage string, rollout rancher.DeploymentRolloutStatus) (status, detail string) {
	containerImage = normalizeListedImage(containerImage)
	if containerImage == "" {
		return "running", "Chưa đọc được image từ Deployment spec"
	}
	if imageMatchesDeploy(containerImage, tag, wantImage) {
		return "success", "Image khớp tag: " + shortenImageRef(containerImage)
	}
	if rollout.IsRolloutInProgress() {
		return "running", "Đang rollout image: " + shortenImageRef(containerImage)
	}
	return "failed", "Image spec không khớp tag deploy — " + shortenImageRef(containerImage)
}

func shortenImageRef(img string) string {
	img = strings.TrimSpace(img)
	if len(img) > 72 {
		return "…" + img[len(img)-69:]
	}
	return img
}

func mergeImageIntoRollout(k8sSt, k8sDetail, k8sErr, imgSt, imgDetail string) (string, string, string) {
	switch stageStatus(imgSt) {
	case "failed":
		return "failed", strings.TrimSpace(k8sDetail + " · " + imgDetail), imgDetail
	case "success":
		if stageStatus(k8sSt) == "success" {
			return k8sSt, strings.TrimSpace(k8sDetail + " · " + imgDetail), k8sErr
		}
		return k8sSt, strings.TrimSpace(k8sDetail + " · " + imgDetail), k8sErr
	default:
		if imgDetail != "" {
			k8sDetail = strings.TrimSpace(k8sDetail + " · " + imgDetail)
		}
		return k8sSt, k8sDetail, k8sErr
	}
}

func isInterestingK8sEvent(ev rancher.ResourceRow) bool {
	reason := strings.ToLower(strings.TrimSpace(ev.Reason))
	msg := strings.ToLower(strings.TrimSpace(ev.Message))
	typeSt := strings.ToLower(strings.TrimSpace(ev.Status))
	if typeSt == "warning" || strings.Contains(typeSt, "warn") {
		return true
	}
	needles := []string{
		"failed", "error", "backoff", "crashloop", "unhealthy", "killing",
		"errimagepull", "imagepullbackoff", "failedscheduling", "oom",
	}
	for _, n := range needles {
		if strings.Contains(reason, n) || strings.Contains(msg, n) {
			return true
		}
	}
	return false
}

func filterDeployEvents(events []rancher.ResourceRow, deploymentName, podName string) []rancher.ResourceRow {
	if len(events) == 0 {
		return nil
	}
	var out []rancher.ResourceRow
	podKey := strings.ToLower(strings.TrimSpace(podName))
	depKey := strings.ToLower("deployment/" + strings.TrimSpace(deploymentName))
	for _, ev := range events {
		if !isInterestingK8sEvent(ev) {
			continue
		}
		if podKey != "" {
			out = append(out, ev)
			continue
		}
		obj := strings.ToLower(strings.TrimSpace(ev.Object))
		if strings.Contains(obj, depKey) || strings.Contains(obj, "pod/app-") {
			out = append(out, ev)
		}
	}
	return out
}

type deployEventsView struct {
	Status string
	Detail string
	Items  []string
}

func buildDeployRuntimeEvents(events []rancher.ResourceRow, podName string) deployEventsView {
	events = filterDeployEvents(events, "app", podName)
	if len(events) == 0 {
		return deployEventsView{
			Status: "success",
			Detail: "Không có event cảnh báo gần đây",
		}
	}
	seen := make(map[string]bool)
	var items []string
	hasHardFail := false
	var headline string
	for i := len(events) - 1; i >= 0; i-- {
		ev := events[i]
		key := formatEventLine(ev)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		items = append(items, formatEventRowFull(ev))
		lower := strings.ToLower(key)
		if strings.Contains(lower, "backoff") || strings.Contains(lower, "crashloop") ||
			strings.Contains(lower, "errimagepull") || strings.Contains(lower, "failed") {
			hasHardFail = true
		}
		if headline == "" {
			headline = key
		}
		if len(items) >= 20 {
			break
		}
	}
	if len(items) == 0 {
		return deployEventsView{
			Status: "success",
			Detail: "Không có event cảnh báo gần đây",
		}
	}
	detail := headline
	if len(items) > 1 {
		detail = fmt.Sprintf("%d events · %s", len(items), headline)
	}
	st := "info"
	if hasHardFail {
		st = "failed"
	}
	return deployEventsView{Status: st, Detail: detail, Items: items}
}

func formatDeployRuntimeEvents(events []rancher.ResourceRow, podName string) (status, detail string) {
	v := buildDeployRuntimeEvents(events, podName)
	return v.Status, v.Detail
}

func formatEventRowFull(ev rancher.ResourceRow) string {
	ts := strings.TrimSpace(ev.Created)
	if ts == "" {
		ts = "-"
	}
	obj := strings.TrimSpace(ev.Object)
	if obj == "" {
		obj = "-"
	}
	line := formatEventLine(ev)
	if line == "" {
		line = "-"
	}
	return ts + " | " + obj + " | " + line
}

func formatEventLine(ev rancher.ResourceRow) string {
	reason := strings.TrimSpace(ev.Reason)
	msg := strings.TrimSpace(ev.Message)
	if reason == "" && msg == "" {
		return ""
	}
	if reason != "" && msg != "" {
		return reason + ": " + msg
	}
	if reason != "" {
		return reason
	}
	return msg
}

func eventFailureHint(events []rancher.ResourceRow, podName string) string {
	events = filterDeployEvents(events, "app", podName)
	for i := len(events) - 1; i >= 0; i-- {
		line := formatEventLine(events[i])
		lower := strings.ToLower(line)
		if strings.Contains(lower, "backoff") || strings.Contains(lower, "crashloop") ||
			strings.Contains(lower, "errimagepull") || strings.Contains(lower, "failed") {
			return line
		}
	}
	return ""
}

func deployImageTagLabelMatches(tag string, labelValue string) bool {
	tag = deploy.ImageTagLabelValue(tag)
	labelValue = strings.TrimSpace(labelValue)
	if labelValue == "" {
		return true
	}
	return tag == labelValue
}
