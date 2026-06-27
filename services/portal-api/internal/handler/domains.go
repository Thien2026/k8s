package handler

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/Thien2026/k8s/services/portal-api/internal/auth"
	"github.com/Thien2026/k8s/services/portal-api/internal/domains"
	"github.com/Thien2026/k8s/services/portal-api/internal/plugins"
	"github.com/go-chi/chi/v5"
)

func (h *Handler) domainPlatform() domains.Platform {
	return domains.Platform{
		Domain:   h.cfg.PlatformDomain,
		PublicIP: h.cfg.NodePublicIP,
	}
}

func (h *Handler) domainSyncer() *domains.Syncer {
	return &domains.Syncer{Rancher: h.rancher, Platform: h.domainPlatform()}
}

func (h *Handler) projectNamespace(p projectRow, env string) string {
	if env == "prod" {
		return p.NamespaceProd
	}
	return p.NamespaceDev
}

func (h *Handler) ensureAutoDomains(ctx context.Context, p projectRow) error {
	plat := h.domainPlatform()
	if plat.Domain == "" {
		return nil
	}
	for _, env := range []string{"dev", "prod"} {
		host := plat.AutoHostname(p.Slug, env)
		var exists bool
		_ = h.db.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM project_domains WHERE project_id=$1 AND environment=$2 AND kind='auto')`,
			p.ID, env).Scan(&exists)
		if exists {
			continue
		}
		_, err := h.db.Exec(ctx, `
			INSERT INTO project_domains (project_id, hostname, environment, tls_enabled, kind, ingress_name)
			VALUES ($1, $2, $3, true, 'auto', '')
			ON CONFLICT (project_id, hostname) DO NOTHING`,
			p.ID, host, env)
		if err != nil {
			return err
		}
	}
	rows, err := h.db.Query(ctx,
		`SELECT id FROM project_domains WHERE project_id=$1 AND ingress_name=''`, p.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return err
		}
		_, _ = h.db.Exec(ctx, `UPDATE project_domains SET ingress_name=$1 WHERE id=$2`, domains.IngressName(id), id)
	}
	return rows.Err()
}

func (h *Handler) syncProjectDomain(ctx context.Context, p projectRow, d *projectDomainRow, clusterID string) {
	syncer := h.domainSyncer()
	if !syncer.Ready() {
		d.SyncStatus = "pending"
		d.SyncError = "Rancher chưa sẵn sàng"
		return
	}
	ns := h.projectNamespace(p, d.Environment)
	in := domains.DomainInput{
		ID:          d.ID,
		Hostname:    d.Hostname,
		Environment: d.Environment,
		TLSEnabled:  d.TLSEnabled,
		Namespace:   ns,
	}
	if err := syncer.SyncIngress(ctx, clusterID, in); err != nil {
		d.SyncStatus = "error"
		d.SyncError = err.Error()
		_, _ = h.db.Exec(ctx,
			`UPDATE project_domains SET sync_status='error', sync_error=$1 WHERE id=$2`, err.Error(), d.ID)
		return
	}
	cert := syncer.CertStatus(ctx, clusterID, ns, d.ID, d.TLSEnabled)
	d.SyncStatus = "synced"
	d.SyncError = ""
	d.CertStatus = cert
	_, _ = h.db.Exec(ctx,
		`UPDATE project_domains SET sync_status='synced', sync_error='', cert_status=$1, ingress_name=$2 WHERE id=$3`,
		cert, domains.IngressName(d.ID), d.ID)
}

func (h *Handler) enrichProjectDomain(ctx context.Context, p projectRow, d *projectDomainRow, clusterID string) {
	plat := h.domainPlatform()
	d.URL = domains.PublicURL(d.Hostname, d.TLSEnabled)
	d.DNS = domains.DNSInstructions(d.Kind, d.Hostname, plat)
	if d.IngressName == "" {
		d.IngressName = domains.IngressName(d.ID)
	}
	if d.SyncStatus == "synced" && d.TLSEnabled && h.domainSyncer().Ready() {
		ns := h.projectNamespace(p, d.Environment)
		if st := h.domainSyncer().CertStatus(ctx, clusterID, ns, d.ID, d.TLSEnabled); st != "" {
			d.CertStatus = st
			_, _ = h.db.Exec(ctx, `UPDATE project_domains SET cert_status=$1 WHERE id=$2`, st, d.ID)
		}
	}
}

func (h *Handler) syncProjectDomainsForEnv(ctx context.Context, p projectRow, env, clusterID string) []string {
	_ = h.ensureAutoDomains(ctx, p)
	list, err := h.listProjectDomains(ctx, p.ID)
	if err != nil {
		return []string{err.Error()}
	}
	warnings := []string{}
	for i := range list {
		if list[i].Environment != env {
			continue
		}
		h.syncProjectDomain(ctx, p, &list[i], clusterID)
		if list[i].SyncStatus == "error" && list[i].SyncError != "" {
			warnings = append(warnings, list[i].SyncError)
		}
	}
	return warnings
}

func (h *Handler) ListProjectDomains(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	p, ok := h.requireProjectAccess(w, r, slug)
	if !ok {
		return
	}
	_ = h.ensureAutoDomains(r.Context(), p)
	items, err := h.listProjectDomainsEnriched(r.Context(), p, r.URL.Query().Get("cluster_id"))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) SyncProjectDomain(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	p, ok := h.requireProjectAccess(w, r, slug)
	if !ok {
		return
	}
	u, _ := auth.UserFromContext(r.Context())
	if !h.canManageProject(r.Context(), u, p.ID) {
		writeAccessDenied(w)
		return
	}
	var domainID int64
	if _, err := fmt.Sscanf(chi.URLParam(r, "domainId"), "%d", &domainID); err != nil || domainID < 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id không hợp lệ"})
		return
	}
	d, err := h.getProjectDomainByID(r.Context(), p.ID, domainID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "domain không tồn tại"})
		return
	}
	clusterID := r.URL.Query().Get("cluster_id")
	rancherOn, _ := h.plugins.Enabled(r.Context(), plugins.Rancher)
	if !rancherOn {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Rancher addon chưa bật"})
		return
	}
	h.syncProjectDomain(r.Context(), p, &d, clusterID)
	h.enrichProjectDomain(r.Context(), p, &d, clusterID)
	writeJSON(w, http.StatusOK, d)
}

func (h *Handler) getProjectDomainByID(ctx context.Context, projectID, domainID int64) (projectDomainRow, error) {
	var d projectDomainRow
	var created time.Time
	err := h.db.QueryRow(ctx, `
		SELECT id, hostname, environment, tls_enabled, COALESCE(kind,'custom'), COALESCE(ingress_name,''),
		       COALESCE(sync_status,'pending'), COALESCE(sync_error,''), COALESCE(cert_status,'unknown'), created_at
		FROM project_domains WHERE id=$1 AND project_id=$2`,
		domainID, projectID).Scan(
		&d.ID, &d.Hostname, &d.Environment, &d.TLSEnabled, &d.Kind, &d.IngressName,
		&d.SyncStatus, &d.SyncError, &d.CertStatus, &created)
	if err != nil {
		return d, err
	}
	d.CreatedAt = created.UTC().Format(time.RFC3339)
	return d, nil
}

func (h *Handler) listProjectDomainsEnriched(ctx context.Context, p projectRow, clusterID string) ([]projectDomainRow, error) {
	list, err := h.listProjectDomains(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	for i := range list {
		h.enrichProjectDomain(ctx, p, &list[i], clusterID)
	}
	return list, nil
}
