package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/Thien2026/k8s/services/portal-api/internal/auth"
)

var errNoSession = errors.New("no session")

func (h *Handler) authenticateRequest(r *http.Request) (auth.User, error) {
	if c, err := r.Cookie(auth.CookieAccess); err == nil && c.Value != "" {
		claims, err := h.tokens.ParseAccess(c.Value)
		if err == nil {
			return auth.User{
				ID:    claims.UserID,
				Email: claims.Email,
				Role:  claims.Role,
			}, nil
		}
	}
	return auth.User{}, errNoSession
}

func (h *Handler) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, err := h.authenticateRequest(r)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "chưa đăng nhập"})
			return
		}
		fresh, err := h.auth.GetUserByID(r.Context(), u.ID)
		if err != nil {
			h.clearSessionCookies(w)
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "phiên không hợp lệ"})
			return
		}
		ctx := auth.WithUser(r.Context(), fresh)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (h *Handler) requireRole(roles ...string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(roles))
	for _, r := range roles {
		allowed[r] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, ok := auth.UserFromContext(r.Context())
			if !ok {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "chưa đăng nhập"})
				return
			}
			if _, ok := allowed[u.Role]; !ok {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "không đủ quyền"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (h *Handler) requireMutatingOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		if !h.originAllowed(r) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "origin không hợp lệ"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) originAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		// Same-origin form hoặc non-browser client qua cookie — cho phép nếu không có Origin
		return true
	}
	for _, o := range h.cfg.AllowedOrigins {
		if strings.EqualFold(strings.TrimSuffix(origin, "/"), strings.TrimSuffix(o, "/")) {
			return true
		}
	}
	return false
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i >= 0 {
		return host[:i]
	}
	return host
}

func auditAction(ctx context.Context, h *Handler, r *http.Request, action, resource string, detail any) {
	var uid *int64
	if u, ok := auth.UserFromContext(ctx); ok {
		id := u.ID
		uid = &id
	}
	h.auth.Audit(ctx, uid, action, resource, detail, clientIP(r))
}
