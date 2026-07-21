package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/Thien2026/k8s/services/portal-api/internal/auth"
)

var ingressBodySizeRe = regexp.MustCompile(`(?i)^(\d+)(m|g|k)?$|^0$`)

type platformPolicy struct {
	RedisMaxMemoryMB     int    `json:"redis_max_memory_mb"`
	RedisMaxClients      int    `json:"redis_max_clients"`
	MinioMaxStorageGB    int    `json:"minio_max_storage_gb"`
	MinioMaxMemoryMB     int    `json:"minio_max_memory_mb"`
	MinioConsoleUploadMB int    `json:"minio_console_upload_mb"`
	MinioMaxObjectMB     int    `json:"minio_max_object_mb"`
	IngressProxyBodySize string `json:"ingress_proxy_body_size"`
	// Default quota per (project × env) — áp cho project mới; override riêng ở project_env_quota.
	DefaultQuotaStorageGBDev  int `json:"default_quota_storage_gb_dev"`
	DefaultQuotaStorageGBProd int `json:"default_quota_storage_gb_prod"`
	DefaultQuotaMemoryMBDev   int `json:"default_quota_memory_mb_dev"`
	DefaultQuotaMemoryMBProd  int `json:"default_quota_memory_mb_prod"`
	DefaultQuotaCPUmDev       int `json:"default_quota_cpu_m_dev"`
	DefaultQuotaCPUmProd      int `json:"default_quota_cpu_m_prod"`
	DefaultQuotaMaxPods       int `json:"default_quota_max_pods"`
	DefaultQuotaMaxPVCs       int `json:"default_quota_max_pvcs"`
	UnlockConfigured          bool   `json:"unlock_configured,omitempty"`
	UpdatedAt                 string `json:"updated_at,omitempty"`
}

func defaultPlatformPolicy() platformPolicy {
	return platformPolicy{
		RedisMaxMemoryMB:     512,
		RedisMaxClients:      1000,
		MinioMaxStorageGB:    100,
		MinioMaxMemoryMB:     1024,
		MinioConsoleUploadMB: 10,
		MinioMaxObjectMB:     5120,
		IngressProxyBodySize: "32m",
		DefaultQuotaStorageGBDev:  20,
		DefaultQuotaStorageGBProd: 50,
		DefaultQuotaMemoryMBDev:   2048,
		DefaultQuotaMemoryMBProd:  4096,
		DefaultQuotaCPUmDev:       2000,
		DefaultQuotaCPUmProd:      4000,
		DefaultQuotaMaxPods:       50,
		DefaultQuotaMaxPVCs:       20,
	}
}

func (h *Handler) loadPlatformPolicy(ctx context.Context) (platformPolicy, string, error) {
	def := defaultPlatformPolicy()
	var unlockHash string
	var updatedAt *time.Time
	err := h.db.QueryRow(ctx, `
		SELECT redis_max_memory_mb, redis_max_clients,
		       minio_max_storage_gb, minio_max_memory_mb, minio_console_upload_mb,
		       COALESCE(minio_max_object_mb, 5120),
		       ingress_proxy_body_size,
		       COALESCE(default_quota_storage_gb_dev, 20),
		       COALESCE(default_quota_storage_gb_prod, 50),
		       COALESCE(default_quota_memory_mb_dev, 2048),
		       COALESCE(default_quota_memory_mb_prod, 4096),
		       COALESCE(default_quota_cpu_m_dev, 2000),
		       COALESCE(default_quota_cpu_m_prod, 4000),
		       COALESCE(default_quota_max_pods, 50),
		       COALESCE(default_quota_max_pvcs, 20),
		       COALESCE(policy_unlock_hash, ''), updated_at
		FROM platform_policy WHERE id = 1`).Scan(
		&def.RedisMaxMemoryMB, &def.RedisMaxClients,
		&def.MinioMaxStorageGB, &def.MinioMaxMemoryMB, &def.MinioConsoleUploadMB,
		&def.MinioMaxObjectMB,
		&def.IngressProxyBodySize,
		&def.DefaultQuotaStorageGBDev, &def.DefaultQuotaStorageGBProd,
		&def.DefaultQuotaMemoryMBDev, &def.DefaultQuotaMemoryMBProd,
		&def.DefaultQuotaCPUmDev, &def.DefaultQuotaCPUmProd,
		&def.DefaultQuotaMaxPods, &def.DefaultQuotaMaxPVCs,
		&unlockHash, &updatedAt,
	)
	if err != nil {
		return def, "", err
	}
	def.UnlockConfigured = unlockHash != ""
	if updatedAt != nil {
		def.UpdatedAt = updatedAt.UTC().Format(time.RFC3339)
	}
	return def, unlockHash, nil
}

func (h *Handler) ensurePolicyUnlockSeed(ctx context.Context) {
	pass := strings.TrimSpace(h.cfg.PolicyUnlockPassphrase)
	if pass == "" {
		return
	}
	var hash string
	err := h.db.QueryRow(ctx, `SELECT COALESCE(policy_unlock_hash, '') FROM platform_policy WHERE id = 1`).Scan(&hash)
	if err != nil || hash != "" {
		return
	}
	hashed, err := auth.HashSecret(pass, 10)
	if err != nil {
		return
	}
	_, _ = h.db.Exec(ctx, `
		UPDATE platform_policy SET policy_unlock_hash = $1, updated_at = now() WHERE id = 1`, hashed)
}

func normalizeIngressProxyBodySize(raw string) (string, bool) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return "32m", true
	}
	if !ingressBodySizeRe.MatchString(raw) {
		return "", false
	}
	return raw, true
}

func (h *Handler) requirePolicyStepUp(w http.ResponseWriter, r *http.Request, u auth.User) bool {
	tok := strings.TrimSpace(r.Header.Get(auth.HeaderPlatformStepUp))
	if tok == "" {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": "thiếu xác thực 2 lớp — gọi POST /admin/policy/step-up trước",
			"code":  "step_up_required",
		})
		return false
	}
	claims, err := h.tokens.ParseStepUp(tok, auth.StepUpPurposePlatformPolicy)
	if err != nil || claims.UserID != u.ID {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": "step-up token không hợp lệ hoặc hết hạn — xác thực lại",
			"code":  "step_up_invalid",
		})
		return false
	}
	return true
}

func (h *Handler) GetPlatformPolicyCeilings(w http.ResponseWriter, r *http.Request) {
	p, _, err := h.loadPlatformPolicy(r.Context())
	if err != nil {
		writeJSON(w, http.StatusOK, defaultPlatformPolicy())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"redis_max_memory_mb":      p.RedisMaxMemoryMB,
		"redis_max_clients":        p.RedisMaxClients,
		"minio_max_storage_gb":     p.MinioMaxStorageGB,
		"minio_max_memory_mb":      p.MinioMaxMemoryMB,
		"minio_console_upload_mb":  p.MinioConsoleUploadMB,
		"minio_max_object_mb":      p.MinioMaxObjectMB,
		"ingress_proxy_body_size":  p.IngressProxyBodySize,
	})
}

func (h *Handler) GetAdminPlatformPolicy(w http.ResponseWriter, r *http.Request) {
	p, _, err := h.loadPlatformPolicy(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	out := map[string]any{
		"redis_max_memory_mb":     p.RedisMaxMemoryMB,
		"redis_max_clients":       p.RedisMaxClients,
		"minio_max_storage_gb":    p.MinioMaxStorageGB,
		"minio_max_memory_mb":     p.MinioMaxMemoryMB,
		"minio_console_upload_mb": p.MinioConsoleUploadMB,
		"minio_max_object_mb":     p.MinioMaxObjectMB,
		"ingress_proxy_body_size": p.IngressProxyBodySize,
		"unlock_configured":       p.UnlockConfigured,
		"updated_at":              p.UpdatedAt,
		"infra":                   h.policyInfraHint(r.Context()),
	}
	writeJSON(w, http.StatusOK, out)
}

// policyInfraHint — gợi ý dung lượng cluster + đã cấp MinIO addon (admin Policy UI).
func (h *Handler) policyInfraHint(ctx context.Context) map[string]any {
	out := map[string]any{
		"disk_ready": false,
		"hint":       "Không lấy được dung lượng node — kiểm tra Rancher/metrics.",
	}
	var allocated int
	_ = h.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(storage_gb), 0)::int
		FROM project_data_addons
		WHERE engine = 'minio' AND status NOT IN ('stopped', 'failed')`).Scan(&allocated)
	out["minio_allocated_gb"] = allocated

	if h.rancher == nil || !h.rancher.Enabled() {
		out["hint"] = "Rancher chưa kết nối — không ước lượng disk cluster."
		return out
	}
	dash, err := h.rancher.ClusterDashboard(ctx, "")
	if err != nil {
		out["hint"] = "Không đọc được cluster dashboard: " + err.Error()
		return out
	}
	disk := dash.Capacity.Disk
	free := disk.Total - disk.Used
	if free < 0 {
		free = 0
	}
	out["disk_ready"] = disk.Total > 0
	out["disk_total_gib"] = disk.Total
	out["disk_used_gib"] = disk.Used
	out["disk_free_gib"] = round1Local(free)
	out["disk_used_pct"] = disk.UsedPct
	out["cpu_free_cores"] = round1Local(dash.Capacity.CPU.Total - dash.Capacity.CPU.Used)
	out["memory_free_gib"] = round1Local(dash.Capacity.Memory.Total - dash.Capacity.Memory.Used)
	if disk.Total > 0 {
		out["hint"] = fmt.Sprintf(
			"Disk node ~%.0f GiB trống / %.0f GiB. Đã cấp MinIO addon: %d GiB. Trần per-project nên thấp hơn phần trống (chừa OS, image, PVC khác).",
			free, disk.Total, allocated,
		)
	}
	return out
}

func round1Local(f float64) float64 {
	return float64(int(f*10+0.5)) / 10
}

func (h *Handler) PostAdminPolicyStepUp(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "chưa đăng nhập"})
		return
	}
	var body struct {
		Password          string `json:"password"`
		UnlockPassphrase  string `json:"unlock_passphrase"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "payload không hợp lệ"})
		return
	}
	if strings.TrimSpace(body.Password) == "" || strings.TrimSpace(body.UnlockPassphrase) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cần mật khẩu đăng nhập và Policy Unlock passphrase"})
		return
	}

	dbUser, err := h.auth.GetDBUserByID(r.Context(), u.ID)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "phiên không hợp lệ"})
		return
	}
	ip := clientIP(r)
	if !dbUser.Active {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "tài khoản đã bị vô hiệu hóa"})
		return
	}
	if !auth.CheckPassword(dbUser.PasswordHash, body.Password) {
		uid := u.ID
		h.auth.Audit(r.Context(), &uid, "policy.step_up_failed", "login_password", nil, ip)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "mật khẩu đăng nhập không đúng"})
		return
	}

	_, unlockHash, err := h.loadPlatformPolicy(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if unlockHash == "" {
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": "chưa cấu hình Policy Unlock — set POLICY_UNLOCK_PASSPHRASE rồi restart portal-api",
			"code":  "unlock_not_configured",
		})
		return
	}
	if !auth.CheckPassword(unlockHash, body.UnlockPassphrase) {
		uid := u.ID
		h.auth.Audit(r.Context(), &uid, "policy.step_up_failed", "unlock_passphrase", nil, ip)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Policy Unlock passphrase không đúng"})
		return
	}

	tok, exp, err := h.tokens.SignStepUp(u.ID, auth.StepUpPurposePlatformPolicy, 10*time.Minute)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "không tạo được step-up token"})
		return
	}
	uid := u.ID
	h.auth.Audit(r.Context(), &uid, "policy.step_up", "platform_policy", nil, ip)
	writeJSON(w, http.StatusOK, map[string]any{
		"step_up_token": tok,
		"expires_at":    exp.UTC().Format(time.RFC3339),
		"expires_in":    int(time.Until(exp).Seconds()),
	})
}

func (h *Handler) PatchAdminPlatformPolicy(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "chưa đăng nhập"})
		return
	}
	if !h.requirePolicyStepUp(w, r, u) {
		return
	}

	cur, _, err := h.loadPlatformPolicy(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	var body struct {
		RedisMaxMemoryMB     *int    `json:"redis_max_memory_mb"`
		RedisMaxClients      *int    `json:"redis_max_clients"`
		MinioMaxStorageGB    *int    `json:"minio_max_storage_gb"`
		MinioMaxMemoryMB     *int    `json:"minio_max_memory_mb"`
		MinioConsoleUploadMB *int    `json:"minio_console_upload_mb"`
		MinioMaxObjectMB     *int    `json:"minio_max_object_mb"`
		IngressProxyBodySize *string `json:"ingress_proxy_body_size"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "payload không hợp lệ"})
		return
	}

	next := cur
	if body.RedisMaxMemoryMB != nil {
		if *body.RedisMaxMemoryMB < 64 || *body.RedisMaxMemoryMB > 4096 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "redis_max_memory_mb phải trong 64–4096"})
			return
		}
		next.RedisMaxMemoryMB = *body.RedisMaxMemoryMB
	}
	if body.RedisMaxClients != nil {
		if *body.RedisMaxClients < 10 || *body.RedisMaxClients > 10000 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "redis_max_clients phải trong 10–10000"})
			return
		}
		next.RedisMaxClients = *body.RedisMaxClients
	}
	if body.MinioMaxStorageGB != nil {
		if *body.MinioMaxStorageGB < 1 || *body.MinioMaxStorageGB > 2000 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "minio_max_storage_gb phải trong 1–2000"})
			return
		}
		next.MinioMaxStorageGB = *body.MinioMaxStorageGB
	}
	if body.MinioMaxMemoryMB != nil {
		if *body.MinioMaxMemoryMB < 128 || *body.MinioMaxMemoryMB > 8192 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "minio_max_memory_mb phải trong 128–8192"})
			return
		}
		next.MinioMaxMemoryMB = *body.MinioMaxMemoryMB
	}
	if body.MinioConsoleUploadMB != nil {
		if *body.MinioConsoleUploadMB < 1 || *body.MinioConsoleUploadMB > 512 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "minio_console_upload_mb phải trong 1–512"})
			return
		}
		next.MinioConsoleUploadMB = *body.MinioConsoleUploadMB
	}
	if body.MinioMaxObjectMB != nil {
		if *body.MinioMaxObjectMB < 1 || *body.MinioMaxObjectMB > 51200 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "minio_max_object_mb phải trong 1–51200"})
			return
		}
		next.MinioMaxObjectMB = *body.MinioMaxObjectMB
	}
	if body.IngressProxyBodySize != nil {
		sz, ok := normalizeIngressProxyBodySize(*body.IngressProxyBodySize)
		if !ok {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ingress_proxy_body_size không hợp lệ (vd. 32m, 1g)"})
			return
		}
		next.IngressProxyBodySize = sz
	}

	_, err = h.db.Exec(r.Context(), `
		UPDATE platform_policy SET
			redis_max_memory_mb = $1,
			redis_max_clients = $2,
			minio_max_storage_gb = $3,
			minio_max_memory_mb = $4,
			minio_console_upload_mb = $5,
			minio_max_object_mb = $6,
			ingress_proxy_body_size = $7,
			updated_at = now(),
			updated_by = $8
		WHERE id = 1`,
		next.RedisMaxMemoryMB, next.RedisMaxClients,
		next.MinioMaxStorageGB, next.MinioMaxMemoryMB, next.MinioConsoleUploadMB,
		next.MinioMaxObjectMB,
		next.IngressProxyBodySize, u.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	auditAction(r.Context(), h, r, "policy.update", "platform_policy", map[string]any{
		"by": u.Email,
		"redis_max_memory_mb": next.RedisMaxMemoryMB,
		"redis_max_clients": next.RedisMaxClients,
		"minio_max_storage_gb": next.MinioMaxStorageGB,
		"minio_max_memory_mb": next.MinioMaxMemoryMB,
		"minio_console_upload_mb": next.MinioConsoleUploadMB,
		"minio_max_object_mb": next.MinioMaxObjectMB,
		"ingress_proxy_body_size": next.IngressProxyBodySize,
	})

	out, _, _ := h.loadPlatformPolicy(r.Context())
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) PostAdminPolicyUnlockPassphrase(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "chưa đăng nhập"})
		return
	}
	if !h.requirePolicyStepUp(w, r, u) {
		return
	}
	var body struct {
		NewUnlockPassphrase string `json:"new_unlock_passphrase"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "payload không hợp lệ"})
		return
	}
	pass := strings.TrimSpace(body.NewUnlockPassphrase)
	if len(pass) < 10 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "passphrase mới tối thiểu 10 ký tự"})
		return
	}
	hashed, err := auth.HashSecret(pass, 10)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	_, err = h.db.Exec(r.Context(), `
		UPDATE platform_policy SET policy_unlock_hash = $1, updated_at = now(), updated_by = $2 WHERE id = 1`,
		hashed, u.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	auditAction(r.Context(), h, r, "policy.unlock_rotated", "platform_policy", map[string]any{"by": u.Email})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "message": "Đã đổi Policy Unlock passphrase"})
}
