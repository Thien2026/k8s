package handler

import (
	"context"
	"net/http"
	"slices"

	"github.com/Thien2026/k8s/services/portal-api/internal/auth"
	"github.com/Thien2026/k8s/services/portal-api/internal/rancher"
)

// accessScope quyền namespace theo role + project_members.
type accessScope struct {
	All             bool
	ReadNamespaces  []string
	WriteNamespaces []string
}

func (h *Handler) accessScope(ctx context.Context, u auth.User) (accessScope, error) {
	if auth.CanViewInfra(u.Role) {
		return accessScope{All: true}, nil
	}

	rows, err := h.db.Query(ctx, `
		SELECT p.namespace_dev, p.namespace_prod
		FROM projects p
		INNER JOIN project_members pm ON pm.project_id = p.id
		WHERE pm.user_id = $1
		ORDER BY p.id`, u.ID)
	if err != nil {
		return accessScope{}, err
	}
	defer rows.Close()

	read := make([]string, 0, 4)
	write := make([]string, 0, 2)
	for rows.Next() {
		var dev, prod string
		if err := rows.Scan(&dev, &prod); err != nil {
			return accessScope{}, err
		}
		if dev != "" && !slices.Contains(read, dev) {
			read = append(read, dev)
		}
		if prod != "" && !slices.Contains(read, prod) {
			read = append(read, prod)
		}
		if u.Role == auth.RoleDev && dev != "" && !slices.Contains(write, dev) {
			write = append(write, dev)
		}
	}
	if err := rows.Err(); err != nil {
		return accessScope{}, err
	}
	return accessScope{ReadNamespaces: read, WriteNamespaces: write}, nil
}

func (h *Handler) accessScopeFromRequest(r *http.Request) (accessScope, error) {
	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		return accessScope{}, errForbidden
	}
	return h.accessScope(r.Context(), u)
}

var errForbidden = &accessError{msg: "không có quyền truy cập tài nguyên này"}

type accessError struct{ msg string }

func (e *accessError) Error() string { return e.msg }

func writeAccessDenied(w http.ResponseWriter) {
	writeJSON(w, http.StatusForbidden, map[string]string{"error": "không có quyền truy cập tài nguyên này"})
}

func namespaceAllowed(scope accessScope, ns string) bool {
	if scope.All {
		return true
	}
	return ns != "" && slices.Contains(scope.ReadNamespaces, ns)
}

func namespaceWritable(scope accessScope, u auth.User, ns string) bool {
	if !auth.CanWriteK8s(u.Role) {
		return false
	}
	if scope.All {
		if auth.CanWriteProd(u.Role) {
			return true
		}
		// dev không có scope.All; tech_lead/admin có
		return true
	}
	return slices.Contains(scope.WriteNamespaces, ns)
}

func (h *Handler) guardK8sRead(w http.ResponseWriter, r *http.Request, key, ns string) (accessScope, bool) {
	scope, err := h.accessScopeFromRequest(r)
	if err != nil {
		writeAccessDenied(w)
		return scope, false
	}
	res, ok := rancher.K8sResourceByKey(key)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown resource"})
		return scope, false
	}
	if scope.All {
		return scope, true
	}
	allowedKey := false
	for _, k := range devK8sKeys {
		if k == key {
			allowedKey = true
			break
		}
	}
	if !allowedKey {
		writeAccessDenied(w)
		return scope, false
	}
	if !res.Namespaced {
		writeAccessDenied(w)
		return scope, false
	}
	if ns != "" && !namespaceAllowed(scope, ns) {
		writeAccessDenied(w)
		return scope, false
	}
	return scope, true
}

func (h *Handler) guardK8sWrite(w http.ResponseWriter, r *http.Request, ns string) (accessScope, bool) {
	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeAccessDenied(w)
		return accessScope{}, false
	}
	scope, err := h.accessScope(r.Context(), u)
	if err != nil {
		writeAccessDenied(w)
		return scope, false
	}
	if ns == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "namespace bắt buộc"})
		return scope, false
	}
	if !namespaceWritable(scope, u, ns) {
		writeAccessDenied(w)
		return scope, false
	}
	return scope, true
}

func filterResourceList(scope accessScope, list rancher.ResourceList) rancher.ResourceList {
	if scope.All {
		return list
	}
	allowed := make(map[string]struct{}, len(scope.ReadNamespaces))
	for _, ns := range scope.ReadNamespaces {
		allowed[ns] = struct{}{}
	}
	filtered := make([]rancher.ResourceRow, 0, len(list.Items))
	for _, item := range list.Items {
		if item.Namespace == "" {
			continue
		}
		if _, ok := allowed[item.Namespace]; ok {
			filtered = append(filtered, item)
		}
	}
	list.Items = filtered
	list.Total = len(filtered)
	if list.Page < 1 {
		list.Page = 1
	}
	return list
}

// DevResources — workload trong namespace project (không cluster-wide).
var devK8sKeys = []string{
	"pods", "deployments", "services", "ingresses",
	"statefulsets", "daemonsets", "jobs", "cronjobs",
	"configmaps", "secrets", "persistentvolumeclaims", "horizontalpodautoscalers",
}
