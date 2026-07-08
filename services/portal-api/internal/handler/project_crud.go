package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Thien2026/k8s/services/portal-api/internal/auth"
	"github.com/Thien2026/k8s/services/portal-api/internal/deploy"
	"github.com/Thien2026/k8s/services/portal-api/internal/plugins"
	"github.com/Thien2026/k8s/services/portal-api/internal/registry"
	"github.com/go-chi/chi/v5"
)

func (h *Handler) CreateProject(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name             string  `json:"name"`
		Slug             string  `json:"slug"`
		Description      string  `json:"description"`
		NamespaceDev     string  `json:"namespace_dev"`
		NamespaceProd    string  `json:"namespace_prod"`
		RegistryProvider string  `json:"registry_provider"`
		Layout           string  `json:"layout"`
		MemberIDs        []int64 `json:"member_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "payload không hợp lệ"})
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "tên project bắt buộc"})
		return
	}
	slug := slugify(body.Slug)
	if slug == "" {
		slug = slugify(name)
	}
	if slug == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "slug không hợp lệ"})
		return
	}
	nsDev := strings.TrimSpace(body.NamespaceDev)
	nsProd := strings.TrimSpace(body.NamespaceProd)
	if nsDev == "" {
		nsDev = slug + "-dev"
	}
	if nsProd == "" {
		nsProd = slug + "-prod"
	}
	provider := strings.TrimSpace(body.RegistryProvider)
	if provider == "" {
		provider = registry.GHCR
	}
	if provider != registry.GHCR && provider != registry.Harbor {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "registry_provider phải là ghcr hoặc harbor"})
		return
	}
	if err := h.registry.ValidateProvider(r.Context(), provider); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	actor := auth.MustUser(r.Context())
	prov, err := h.registry.Provision(r.Context(), provider, slug)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	harborName := prov.HarborProject
	warnings := []string{}
	// Kiểu chạy chốt khi gắn GitHub (Deploy / Git), không lúc tạo project.
	layout := deploy.LayoutSingle

	var id int64
	err = h.db.QueryRow(r.Context(), `
		INSERT INTO projects (name, slug, description, namespace_dev, namespace_prod, harbor_project, registry_provider, owner_id, layout)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id`,
		name, slug, strings.TrimSpace(body.Description), nsDev, nsProd, harborName, provider, actor.ID, layout,
	).Scan(&id)
	if err != nil {
		if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "duplicate") {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "tên hoặc slug đã tồn tại"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	_, _ = h.db.Exec(r.Context(), `INSERT INTO project_repos (project_id) VALUES ($1)`, id)
	_, _ = h.db.Exec(r.Context(),
		`INSERT INTO project_members (project_id, user_id, role) VALUES ($1, $2, 'owner') ON CONFLICT DO NOTHING`,
		id, actor.ID)

	for _, uid := range body.MemberIDs {
		if uid == actor.ID {
			continue
		}
		_, _ = h.db.Exec(r.Context(),
			`INSERT INTO project_members (project_id, user_id, role) VALUES ($1, $2, 'dev') ON CONFLICT DO NOTHING`,
			id, uid)
	}

	rancherOn, _ := h.plugins.Enabled(r.Context(), plugins.Rancher)
	if rancherOn {
		if h.rancher != nil && h.rancher.Enabled() {
			cid := r.URL.Query().Get("cluster_id")
			for _, ns := range []string{nsDev, nsProd} {
				if err := h.rancher.EnsureNamespace(r.Context(), cid, ns); err != nil {
					warnings = append(warnings, fmt.Sprintf("Namespace %s: %s", ns, err.Error()))
				}
			}
		} else {
			warnings = append(warnings, "Rancher addon đã bật nhưng chưa cấu hình — chạy bootstrap/addons/install-rancher.sh")
		}
	} else {
		warnings = append(warnings, "Rancher addon chưa bật — bỏ qua tạo namespace (bật trong Addons)")
	}

	auditAction(r.Context(), h, r, "project.create", slug, map[string]any{"name": name, "by": actor.Email})

	created := projectRow{
		ID: id, Slug: slug, Name: name,
		NamespaceDev: nsDev, NamespaceProd: nsProd,
	}
	_ = h.ensureAutoDomains(r.Context(), created)
	cid := r.URL.Query().Get("cluster_id")
	if rancherOn && h.rancher != nil && h.rancher.Enabled() {
		domList, _ := h.listProjectDomains(r.Context(), id)
		for i := range domList {
			h.syncProjectDomain(r.Context(), created, &domList[i], cid, nil)
		}
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id": id, "slug": slug, "name": name,
		"layout":        layout,
		"namespace_dev": nsDev, "namespace_prod": nsProd,
		"registry_provider": provider,
		"harbor_project":    harborName, "warnings": warnings,
	})
}

func (h *Handler) DeleteProject(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	u, _ := auth.UserFromContext(r.Context())
	if !auth.CanDeleteProject(u.Role) {
		writeAccessDenied(w)
		return
	}
	p, err := h.getProjectBySlug(r.Context(), slug)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "project không tồn tại"})
		return
	}

	var body struct {
		PurgeK8s bool `json:"purge_k8s"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	purgeK8s := true
	if r.ContentLength > 0 {
		purgeK8s = body.PurgeK8s
	}
	// Luôn dọn tài nguyên trên VPS — chỉ giữ lại dữ liệu bên thứ 3 (GitHub/GHCR cloud).
	purgeK8s = true

	cid := r.URL.Query().Get("cluster_id")
	warnings := []string{}
	purged := []string{}

	warnings = append(warnings, h.deleteArgoApplications(r.Context(), cid, p.Slug)...)

	domList, _ := h.listProjectDomains(r.Context(), p.ID)
	syncer := h.domainSyncer()
	for _, d := range domList {
		ns := h.projectNamespace(p, d.Environment)
		if err := syncer.DeleteIngress(r.Context(), cid, ns, d.ID); err != nil {
			warnings = append(warnings, fmt.Sprintf("Ingress %s: %s", d.Hostname, err.Error()))
		}
	}

	if purgeK8s && h.rancher != nil && h.rancher.Enabled() {
		for _, ns := range []string{p.NamespaceDev, p.NamespaceProd} {
			if strings.TrimSpace(ns) == "" {
				continue
			}
			if err := h.rancher.PurgeNamespace(r.Context(), cid, ns); err != nil {
				warnings = append(warnings, fmt.Sprintf("Namespace %s: %s", ns, err.Error()))
				continue
			}
			if err := h.rancher.WaitNamespaceDeleted(r.Context(), cid, ns, 90*time.Second); err != nil {
				warnings = append(warnings, err.Error())
			} else {
				purged = append(purged, "k8s:"+ns)
			}
		}
	}

	harborName := strings.TrimSpace(p.HarborProject)
	if harborName == "" {
		harborName = p.Slug
	}
	if strings.EqualFold(strings.TrimSpace(p.RegistryProvider), "harbor") && h.registry != nil {
		if err := h.registry.Deprovision(r.Context(), p.RegistryProvider, p.Slug, harborName); err != nil {
			warnings = append(warnings, fmt.Sprintf("Harbor project %s: %s", harborName, err.Error()))
		} else {
			purged = append(purged, "harbor:"+harborName)
		}
	}

	_, err = h.db.Exec(r.Context(), `DELETE FROM projects WHERE id=$1`, p.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	auditAction(r.Context(), h, r, "project.delete", slug, map[string]any{
		"name": p.Name, "by": u.Email, "purge_k8s": purgeK8s, "purged": purged,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"status":   "ok",
		"slug":     slug,
		"purged":   purged,
		"warnings": warnings,
		"note":     "Đã xóa metadata DB, namespace K8s, Harbor và ArgoCD app (nếu có). GitHub Actions / GHCR cloud không xóa.",
	})
}

func (h *Handler) PatchProject(w http.ResponseWriter, r *http.Request) {
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
	var body struct {
		Description      string `json:"description"`
		RegistryProvider string `json:"registry_provider"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "payload không hợp lệ"})
		return
	}
	desc := strings.TrimSpace(body.Description)
	provider := strings.TrimSpace(body.RegistryProvider)
	if provider != "" && provider != p.RegistryProvider {
		if provider != registry.GHCR && provider != registry.Harbor {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "registry_provider phải là ghcr hoặc harbor"})
			return
		}
		if err := h.registry.ValidateProvider(r.Context(), provider); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		prov, err := h.registry.Provision(r.Context(), provider, p.Slug)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		harborName := prov.HarborProject
		if provider == registry.GHCR {
			harborName = p.HarborProject
		}
		_, err = h.db.Exec(r.Context(), `
			UPDATE projects SET registry_provider=$1, harbor_project=$2, description=COALESCE(NULLIF($3,''), description), updated_at=now()
			WHERE id=$4`,
			provider, harborName, desc, p.ID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		auditAction(r.Context(), h, r, "project.registry", slug, map[string]any{"provider": provider, "by": u.Email})
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	_, err := h.db.Exec(r.Context(), `UPDATE projects SET description=$1, updated_at=now() WHERE id=$2`, desc, p.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
