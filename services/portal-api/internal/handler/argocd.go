package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type argoAppStatus struct {
	SyncStatus   string
	HealthStatus string
	Revision     string
	Phase        string
	Message      string
}

func (h *Handler) argoEnabled() bool {
	return h.argoEnabledCtx(context.Background())
}

func (h *Handler) argoEnabledCtx(ctx context.Context) bool {
	g := h.loadGitOpsConfig(ctx)
	return h.rancher != nil && h.rancher.Enabled() &&
		strings.TrimSpace(g.RepoURL) != "" &&
		strings.TrimSpace(h.cfg.ArgoCDNamespace) != ""
}

func (h *Handler) argoAppName(slug, env string) string {
	env = strings.ToLower(strings.TrimSpace(env))
	if env == "" {
		env = "dev"
	}
	return strings.TrimSpace(slug) + "-" + env
}

func (h *Handler) argoRepoPath(slug, env string) string {
	return h.argoRepoPathCtx(context.Background(), slug, env)
}

func (h *Handler) argoRepoPathCtx(ctx context.Context, slug, env string) string {
	g := h.loadGitOpsConfig(ctx)
	base := strings.Trim(strings.TrimSpace(g.BasePath), "/")
	if base == "" {
		base = "apps"
	}
	env = strings.ToLower(strings.TrimSpace(env))
	if env == "" {
		env = "dev"
	}
	return fmt.Sprintf("%s/%s/overlays/%s", base, strings.TrimSpace(slug), env)
}

func (h *Handler) argoDashboardURL(appName string) string {
	base := strings.TrimRight(strings.TrimSpace(h.cfg.ArgoCDURL), "/")
	if base == "" || appName == "" {
		return ""
	}
	return base + "/applications/" + appName
}

func (h *Handler) ensureArgoApplication(ctx context.Context, p projectRow, env, imageTag string) (string, string, error) {
	appName := h.argoAppName(p.Slug, env)
	ns := strings.TrimSpace(h.cfg.ArgoCDNamespace)
	g := h.loadGitOpsConfig(ctx)
	destNS := h.projectNamespace(p, env)
	payload := map[string]any{
		"apiVersion": "argoproj.io/v1alpha1",
		"kind":       "Application",
		"metadata": map[string]any{
			"name":      appName,
			"namespace": ns,
			"labels": map[string]any{
				"platform.project": p.Slug,
				"platform.env":     strings.ToLower(strings.TrimSpace(env)),
			},
			"annotations": map[string]any{
				"platform.7mlabs.io/requested-image-tag": strings.TrimSpace(imageTag),
			},
		},
		"spec": map[string]any{
			"project": "default",
			"source": map[string]any{
				"repoURL":        strings.TrimSpace(g.RepoURL),
				"targetRevision": strings.TrimSpace(g.RepoBranch),
				"path":           h.argoRepoPathCtx(ctx, p.Slug, env),
			},
			"destination": map[string]any{
				"server":    "https://kubernetes.default.svc",
				"namespace": destNS,
			},
			"syncPolicy": map[string]any{
				"syncOptions": []string{"CreateNamespace=true"},
			},
		},
	}
	if strings.EqualFold(strings.TrimSpace(env), "dev") {
		payload["spec"].(map[string]any)["syncPolicy"] = map[string]any{
			"automated": map[string]any{
				"prune":    true,
				"selfHeal": true,
			},
			"syncOptions": []string{"CreateNamespace=true"},
		}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return appName, "", err
	}
	if err := h.rancher.ApplyNamespacedObject(ctx, "", "/apis/argoproj.io/v1alpha1/applications", ns, raw); err != nil {
		return appName, "", err
	}
	return appName, h.argoDashboardURL(appName), nil
}

func (h *Handler) argoApplicationStatus(ctx context.Context, appName string) (argoAppStatus, error) {
	ns := strings.TrimSpace(h.cfg.ArgoCDNamespace)
	raw, err := h.rancher.GetK8sResource(ctx, "", "argocdapplications", ns, appName)
	if err != nil {
		return argoAppStatus{}, err
	}
	var app struct {
		Status struct {
			Sync struct {
				Status   string `json:"status"`
				Revision string `json:"revision"`
			} `json:"sync"`
			Health struct {
				Status string `json:"status"`
			} `json:"health"`
			OperationState struct {
				Phase   string `json:"phase"`
				Message string `json:"message"`
			} `json:"operationState"`
		} `json:"status"`
	}
	if err := json.Unmarshal(raw, &app); err != nil {
		return argoAppStatus{}, err
	}
	return argoAppStatus{
		SyncStatus:   strings.TrimSpace(app.Status.Sync.Status),
		HealthStatus: strings.TrimSpace(app.Status.Health.Status),
		Revision:     strings.TrimSpace(app.Status.Sync.Revision),
		Phase:        strings.TrimSpace(app.Status.OperationState.Phase),
		Message:      strings.TrimSpace(app.Status.OperationState.Message),
	}, nil
}

func (h *Handler) deleteArgoApplications(ctx context.Context, clusterID, slug string) []string {
	if !h.argoEnabled() || h.rancher == nil {
		return nil
	}
	ns := strings.TrimSpace(h.cfg.ArgoCDNamespace)
	if ns == "" {
		return nil
	}
	var warnings []string
	for _, env := range []string{"dev", "prod"} {
		appName := h.argoAppName(slug, env)
		if err := h.rancher.DeleteNamespacedObject(ctx, clusterID, "/apis/argoproj.io/v1alpha1/applications", ns, appName); err != nil {
			warnings = append(warnings, fmt.Sprintf("ArgoCD app %s: %s", appName, err.Error()))
		}
	}
	return warnings
}

func argoRuntimeVerdict(st argoAppStatus) (status, detail, errMsg string) {
	syncSt := strings.ToLower(strings.TrimSpace(st.SyncStatus))
	healthSt := strings.ToLower(strings.TrimSpace(st.HealthStatus))
	phase := strings.ToLower(strings.TrimSpace(st.Phase))
	shortRev := strings.TrimSpace(st.Revision)
	if len(shortRev) > 12 {
		shortRev = shortRev[:12]
	}
	if syncSt == "synced" && healthSt == "healthy" {
		detail = "ArgoCD Synced/Healthy"
		if shortRev != "" {
			detail += " · rev " + shortRev
		}
		return "success", detail, ""
	}
	if healthSt == "degraded" || phase == "failed" || phase == "error" {
		msg := strings.TrimSpace(st.Message)
		if msg == "" {
			msg = fmt.Sprintf("ArgoCD sync=%s health=%s phase=%s", st.SyncStatus, st.HealthStatus, st.Phase)
		}
		return "failed", msg, msg
	}
	detail = fmt.Sprintf("ArgoCD sync=%s health=%s", st.SyncStatus, st.HealthStatus)
	if phase != "" {
		detail += " phase=" + st.Phase
	}
	if shortRev != "" {
		detail += " rev=" + shortRev
	}
	return "running", detail, ""
}
