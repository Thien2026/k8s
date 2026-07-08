package handler

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/Thien2026/k8s/services/portal-api/internal/auth"
	"github.com/go-chi/chi/v5"
)

func (h *Handler) GetProjectDeployActivity(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	p, ok := h.requireProjectAccess(w, r, slug)
	if !ok {
		return
	}
	u, _ := auth.UserFromContext(r.Context())
	env := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("environment")))
	if env == "" {
		env = "dev"
	}
	scope := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("scope")))
	if scope == "" {
		scope = "current"
	}
	repo, _ := h.getProjectRepo(r.Context(), p.ID)
	listLimit := deployHistoryLimit
	if scope == "current" {
		listLimit = 8
	}
	items, err := h.listProjectDeployments(r.Context(), p.ID, env, listLimit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	h.sealSupersededDeploymentRows(r.Context(), p.ID, env)
	items, err = h.listProjectDeployments(r.Context(), p.ID, env, listLimit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	items = dedupeDeploymentsByTag(items)
	items = h.enrichDeploymentsFromGitHub(r.Context(), u, p, repo, items, scope)
	items = dedupeDeploymentsByTag(items)
	for i := range items {
		_ = h.repairHistoricalDeploymentRow(r.Context(), p, &items[i])
	}
	items = dedupeDeploymentsByTag(items)
	servingTag := h.clusterServingImageTag(r.Context(), p, env)
	currentIdx := pickCurrentDeploymentIndex(items, env, servingTag)
	var current *deploymentRow
	pollSec := 5
	if currentIdx >= 0 {
		if scope == "current" {
			if items[currentIdx].GitHubRunID == 0 {
				h.attachGitHubRunForDeployment(r.Context(), u.ID, p, repo, &items[currentIdx])
			}
			if items[currentIdx].GitHubRunID > 0 {
				h.enrichBuildLive(r.Context(), u.ID, repo.GitHubOwner, repo.GitHubRepo, &items[currentIdx])
			} else if deploymentIsTrulyActive(items[currentIdx]) {
				items[currentIdx].Live = true
				pollSec = 2
			}
			cur := &items[currentIdx]
			if stageStatus(cur.BuildStatus) == "success" || allBuildStepsSuccess(cur.BuildSteps) ||
				stageStatus(cur.DeployStatus) != "pending" || deploymentIsTerminal(*cur) {
				h.refreshDeploymentRuntime(r.Context(), p, cur)
			}
			h.enrichHarborScan(r.Context(), p, &items[currentIdx], true)
			if deploymentNeedsFastPoll(items[currentIdx]) {
				pollSec = 2
			} else if deploymentIsTerminal(items[currentIdx]) {
				items[currentIdx].Live = false
				pollSec = 0
			}
		}
		items[currentIdx].Stages = h.deploymentStages(&items[currentIdx])
		current = &items[currentIdx]
	}
	for i := range items {
		if i != currentIdx {
			if scope == "history" && i < 5 {
				h.enrichHarborScan(r.Context(), p, &items[i], false)
			}
			items[i].Stages = h.deploymentStages(&items[i])
		}
	}
	var filtered []deploymentRow
	if scope == "current" {
		if current != nil {
			filtered = []deploymentRow{*current}
		}
	} else {
		filtered = append(filtered, items...)
	}
	markServing := func(d *deploymentRow) {
		if d == nil {
			return
		}
		d.Serving = strings.EqualFold(d.Status, "success") && servingTag != "" &&
			imageTagsMatch(d.ImageTag, servingTag)
	}
	markServing(current)
	for i := range filtered {
		markServing(&filtered[i])
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"environment":       env,
		"scope":             scope,
		"current":           current,
		"items":             filtered,
		"serving_image_tag": servingTag,
		"console_profile":   h.consoleDeployProfile(r.Context(), p.ID, repo),
		"cluster_profile":   h.clusterRuntimeProfile(r.Context(), p, env),
		"workflow_url": func() string {
			if repo.GitHubOwner != "" && repo.GitHubRepo != "" {
				return fmt.Sprintf("https://github.com/%s/%s/actions", repo.GitHubOwner, repo.GitHubRepo)
			}
			return ""
		}(),
		"poll_interval_sec": func() int {
			if scope == "history" {
				return 0
			}
			return pollSec
		}(),
	})
}
