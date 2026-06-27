package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/Thien2026/k8s/services/portal-api/internal/auth"
)

func (h *Handler) AuthLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "payload không hợp lệ"})
		return
	}
	email := strings.TrimSpace(strings.ToLower(body.Email))
	if email == "" || body.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "email và mật khẩu bắt buộc"})
		return
	}

	ip := clientIP(r)
	dbUser, err := h.auth.GetUserByEmail(r.Context(), email)
	if err != nil {
		h.auth.Audit(r.Context(), nil, "auth.login_failed", email, map[string]string{"reason": "unknown_user"}, ip)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Email hoặc mật khẩu không đúng"})
		return
	}
	if !dbUser.Active {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Tài khoản đã bị vô hiệu hóa"})
		return
	}
	if dbUser.LockedUntil != nil && dbUser.LockedUntil.After(time.Now()) {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{
			"error": "Tài khoản tạm khóa do đăng nhập sai nhiều lần — thử lại sau 15 phút",
		})
		return
	}
	if !auth.CheckPassword(dbUser.PasswordHash, body.Password) {
		_ = h.auth.RecordFailedLogin(r.Context(), dbUser.ID)
		uid := dbUser.ID
		h.auth.Audit(r.Context(), &uid, "auth.login_failed", email, nil, ip)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Email hoặc mật khẩu không đúng"})
		return
	}

	u := auth.User{ID: dbUser.ID, Email: dbUser.Email, DisplayName: dbUser.DisplayName, Role: dbUser.Role}
	if err := h.issueSession(w, r, u); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "không tạo được phiên đăng nhập"})
		return
	}
	_ = h.auth.RecordSuccessfulLogin(r.Context(), dbUser.ID)
	uid := dbUser.ID
	h.auth.Audit(r.Context(), &uid, "auth.login", email, nil, ip)
	writeJSON(w, http.StatusOK, map[string]any{"user": u})
}

func (h *Handler) AuthRefresh(w http.ResponseWriter, r *http.Request) {
	rt, err := r.Cookie(auth.CookieRefresh)
	if err != nil || rt.Value == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "phiên hết hạn — đăng nhập lại"})
		return
	}
	hash := auth.HashToken(rt.Value)
	userID, err := h.auth.GetSessionUserID(r.Context(), hash)
	if err != nil {
		h.clearSessionCookies(w)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "phiên hết hạn — đăng nhập lại"})
		return
	}
	u, err := h.auth.GetUserByID(r.Context(), userID)
	if err != nil {
		h.clearSessionCookies(w)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "phiên hết hạn — đăng nhập lại"})
		return
	}
	_ = h.auth.RevokeSession(r.Context(), hash)
	if err := h.issueSession(w, r, u); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "không gia hạn được phiên"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": u})
}

func (h *Handler) AuthLogout(w http.ResponseWriter, r *http.Request) {
	if rt, err := r.Cookie(auth.CookieRefresh); err == nil && rt.Value != "" {
		_ = h.auth.RevokeSession(r.Context(), auth.HashToken(rt.Value))
	}
	if u, ok := auth.UserFromContext(r.Context()); ok {
		uid := u.ID
		h.auth.Audit(r.Context(), &uid, "auth.logout", u.Email, nil, clientIP(r))
	}
	h.clearSessionCookies(w)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) AuthMe(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "chưa đăng nhập"})
		return
	}
	writeJSON(w, http.StatusOK, u)
}

// AuthQuickLogin — tạm thời: trả cred admin khi QUICK_LOGIN_ENABLED=true (gỡ trước production).
func (h *Handler) AuthQuickLogin(w http.ResponseWriter, _ *http.Request) {
	if !h.cfg.QuickLoginEnabled {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false})
		return
	}
	email := strings.TrimSpace(strings.ToLower(h.cfg.QuickLoginEmail))
	pass := h.cfg.QuickLoginPassword
	if email == "" || pass == "" {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":  true,
		"email":    email,
		"password": pass,
		"label":    "Admin (đăng nhập nhanh)",
	})
}

func (h *Handler) issueSession(w http.ResponseWriter, r *http.Request, u auth.User) error {
	access, err := h.tokens.SignAccess(u)
	if err != nil {
		return err
	}
	refresh, hash, err := auth.NewRefreshToken()
	if err != nil {
		return err
	}
	expires := time.Now().Add(h.tokens.RefreshTTL)
	if err := h.auth.CreateSession(r.Context(), u.ID, hash, r.UserAgent(), clientIP(r), expires); err != nil {
		return err
	}
	h.setCookie(w, auth.CookieAccess, access, h.tokens.AccessTTL)
	h.setCookie(w, auth.CookieRefresh, refresh, h.tokens.RefreshTTL)
	return nil
}

func (h *Handler) setCookie(w http.ResponseWriter, name, value string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   int(ttl.Seconds()),
		HttpOnly: true,
		Secure:   h.tokens.Secure,
		SameSite: http.SameSiteStrictMode,
	})
}

func (h *Handler) clearSessionCookies(w http.ResponseWriter) {
	for _, name := range []string{auth.CookieAccess, auth.CookieRefresh} {
		http.SetCookie(w, &http.Cookie{
			Name:     name,
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   h.tokens.Secure,
			SameSite: http.SameSiteStrictMode,
		})
	}
}
