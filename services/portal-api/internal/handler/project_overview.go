package handler

import (
	"context"
	"net/http"
	"sync"

	"github.com/Thien2026/k8s/services/portal-api/internal/auth"
	"github.com/go-chi/chi/v5"
)

func (h *Handler) GetProject(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	p, ok := h.requireProjectAccess(w, r, slug)
	if !ok {
		return
	}
	members, _ := h.listProjectMembers(r.Context(), p.ID)
	repo, _ := h.getProjectRepo(r.Context(), p.ID)
	h.enrichRepoWorkflowStatus(r.Context(), p.ID, &repo)
	services, _ := h.listProjectServices(r.Context(), p.ID)
	if p.Layout == "" {
		p.Layout = h.getProjectLayout(r.Context(), p.ID)
	}
	u, _ := auth.UserFromContext(r.Context())
	_ = h.resolveBuildMode(r.Context(), u.ID, p.ID, &repo)
	_ = h.ensureAutoDomains(r.Context(), p)
	domainsList, _ := h.listProjectDomainsEnriched(r.Context(), p, r.URL.Query().Get("cluster_id"))
	h.enrichProjectRegistry(r.Context(), &p)
	writeJSON(w, http.StatusOK, map[string]any{
		"project":  p,
		"members":  members,
		"repo":     repo,
		"domains":  domainsList,
		"services": map[string]any{"layout": p.Layout, "items": services},
	})
}

func (h *Handler) ProjectOverview(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	p, ok := h.requireProjectAccess(w, r, slug)
	if !ok {
		return
	}
	devMap := map[string]any{"namespace": p.NamespaceDev}
	prodMap := map[string]any{"namespace": p.NamespaceProd}
	out := map[string]any{
		"project": p,
		"dev":     devMap,
		"prod":    prodMap,
	}
	if h.monitoringConfigured() {
		out["monitoring"] = map[string]any{
			"grafana_url":        trimURL(h.cfg.GrafanaURL),
			"dev_dashboard_url":  grafanaNamespaceDashboardURL(h.cfg.GrafanaURL, p.NamespaceDev),
			"prod_dashboard_url": grafanaNamespaceDashboardURL(h.cfg.GrafanaURL, p.NamespaceProd),
		}
	}
	if dep, err := h.projectDeploySummary(r.Context(), p.ID, "dev"); err == nil {
		devMap["deploy"] = dep
	}
	if dep, err := h.projectDeploySummary(r.Context(), p.ID, "prod"); err == nil {
		prodMap["deploy"] = dep
	}
	if h.rancher != nil && h.rancher.Enabled() {
		cid := r.URL.Query().Get("cluster_id")
		jobs := []struct {
			env, key, ns string
		}{
			{"dev", "pods", p.NamespaceDev},
			{"dev", "deployments", p.NamespaceDev},
			{"prod", "pods", p.NamespaceProd},
			{"prod", "deployments", p.NamespaceProd},
		}
		var wg sync.WaitGroup
		var mu sync.Mutex
		for _, job := range jobs {
			wg.Add(1)
			go func(env, key, ns string) {
				defer wg.Done()
				n, err := h.rancher.CountK8s(r.Context(), cid, key, ns)
				if err != nil {
					return
				}
				mu.Lock()
				defer mu.Unlock()
				m := devMap
				if env == "prod" {
					m = prodMap
				}
				field := key
				if field == "deployments" {
					m["deployments"] = n
				} else {
					m[field] = n
				}
			}(job.env, job.key, job.ns)
		}
		wg.Wait()
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) projectDeploySummary(ctx context.Context, projectID int64, env string) (map[string]any, error) {
	items, err := h.listProjectDeployments(ctx, projectID, env, 1)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return map[string]any{"status": "none"}, nil
	}
	d := items[0]
	return map[string]any{
		"status":         d.Status,
		"image_tag":      d.ImageTag,
		"runtime_status": d.RuntimeStatus,
		"deploy_status":  d.DeployStatus,
		"build_status":   d.BuildStatus,
	}, nil
}
