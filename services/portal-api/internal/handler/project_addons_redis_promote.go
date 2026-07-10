package handler

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/Thien2026/k8s/services/portal-api/internal/auth"
	"github.com/Thien2026/k8s/services/portal-api/internal/deploy"
	"github.com/go-chi/chi/v5"
)

func isAddonManagedRuntimeEnvKey(key string) bool {
	switch strings.TrimSpace(key) {
	case "REDIS_URL", "REDIS_KEY_TTL_SECONDS",
		"S3_ENDPOINT", "S3_ACCESS_KEY", "S3_SECRET_KEY", "S3_BUCKET", "S3_REGION", "S3_USE_SSL":
		return true
	default:
		return false
	}
}

func (h *Handler) projectRuntimeContractWantsRedis(ctx context.Context, projectID int64) bool {
	repo, err := h.getProjectRepo(ctx, projectID)
	if err != nil {
		return false
	}
	c := h.effectiveRuntimeContract(ctx, projectID, repo)
	if c == nil {
		return false
	}
	_, ok := c.Vars[deploy.ConventionRedisURLKey]
	return ok
}

func (h *Handler) projectNeedsRedis(ctx context.Context, projectID int64) bool {
	if h.redisDevAddonReady(ctx, projectID) || h.redisProdAddonReady(ctx, projectID) {
		return true
	}
	return h.projectRuntimeContractWantsRedis(ctx, projectID)
}

func (h *Handler) redisDevAddonReady(ctx context.Context, projectID int64) bool {
	addon, err := h.getProjectAddon(ctx, projectID, "redis", "dev")
	if err == nil && addon != nil && addon.HasConnection && addon.Status == "running" {
		return true
	}
	vars, _ := h.envVarsMap(ctx, projectID, "dev")
	return strings.TrimSpace(vars["REDIS_URL"]) != ""
}

func (h *Handler) devUsesRedisAddon(ctx context.Context, projectID int64) bool {
	return h.projectNeedsRedis(ctx, projectID)
}

func (h *Handler) redisProdAddonReady(ctx context.Context, projectID int64) bool {
	addon, err := h.getProjectAddon(ctx, projectID, "redis", "prod")
	if err != nil || addon == nil {
		return false
	}
	return addon.HasConnection && addon.Status == "running"
}

func (h *Handler) redisAddonPromoteReadiness(ctx context.Context, p projectRow) promoteReadinessItem {
	item := promoteReadinessItem{
		ID:    "redis_prod",
		Label: "Redis addon (prod)",
		Group: "prod",
		Tab:   "addons",
	}
	if !h.projectNeedsRedis(ctx, p.ID) {
		item.OK = true
		item.Detail = "Dev không dùng Redis — bỏ qua"
		return item
	}
	if !h.redisDevAddonReady(ctx, p.ID) {
		item.OK = false
		item.Detail = "Contract cần REDIS_URL — bật Redis addon trên Dev (tab Addons) trước promote"
		return item
	}
	if h.redisProdAddonReady(ctx, p.ID) {
		item.OK = true
		item.Detail = "Redis prod running · ClusterIP + NetworkPolicy (không NodePort)"
		return item
	}
	prodAddon, _ := h.getProjectAddon(ctx, p.ID, "redis", "prod")
	if prodAddon != nil && prodAddon.Status == "provisioning" {
		item.OK = false
		item.Detail = "Redis prod đang provision — đợi pod ready hoặc bấm Provision Redis prod"
		return item
	}
	item.OK = false
	item.Detail = "Dev đã có Redis — cần provision prod (copy quota từ dev, instance riêng)"
	return item
}

func (h *Handler) ensureRedisDevForPromote(ctx context.Context, p projectRow) error {
	if h.redisDevAddonReady(ctx, p.ID) {
		return nil
	}
	if !h.projectRuntimeContractWantsRedis(ctx, p.ID) {
		return nil
	}
	release := redisAddonRelease(p.Slug, "dev")
	policy := normalizeRedisMaxmemoryPolicy("allkeys-lru")
	ttl := int64(86400)
	_, err := h.db.Exec(ctx, `
		INSERT INTO project_data_addons (project_id, engine, environment, status, k8s_release, max_memory_mb, max_clients, maxmemory_policy, default_key_ttl_sec)
		VALUES ($1, 'redis', 'dev', 'pending', $2, 128, 100, $3, $4)
		ON CONFLICT (project_id, engine, environment) DO UPDATE SET
			status = CASE WHEN project_data_addons.status IN ('stopped', 'failed') THEN 'pending' ELSE project_data_addons.status END,
			updated_at = now()`,
		p.ID, release, policy, ttl)
	if err != nil {
		return err
	}
	addon, err := h.getProjectAddon(ctx, p.ID, "redis", "dev")
	if err != nil {
		return err
	}
	return h.provisionRedisAddon(ctx, p, "redis", "dev", addon)
}

func (h *Handler) promoteRedisAddonFromDev(ctx context.Context, p projectRow) error {
	if err := h.ensureRedisDevForPromote(ctx, p); err != nil {
		return fmt.Errorf("redis dev: %w", err)
	}
	dev, err := h.getProjectAddon(ctx, p.ID, "redis", "dev")
	if err != nil {
		vars, _ := h.envVarsMap(ctx, p.ID, "dev")
		if strings.TrimSpace(vars["REDIS_URL"]) == "" {
			return nil
		}
		dev = &projectAddonView{
			Engine:           "redis",
			Environment:      "dev",
			MaxMemoryMB:      128,
			MaxClients:       100,
			MaxmemoryPolicy:  "allkeys-lru",
			DefaultKeyTTLSec: 86400,
			HasConnection:    true,
		}
	}
	if dev == nil || !dev.HasConnection {
		return nil
	}
	if h.redisProdAddonReady(ctx, p.ID) {
		return nil
	}
	policy := normalizeRedisMaxmemoryPolicy(dev.MaxmemoryPolicy)
	ttl := normalizeRedisDefaultKeyTTL(dev.DefaultKeyTTLSec)
	if ttl == 0 && dev.DefaultKeyTTLSec == 0 {
		ttl = 86400
	}
	release := redisAddonRelease(p.Slug, "prod")
	_, err = h.db.Exec(ctx, `
		INSERT INTO project_data_addons (project_id, engine, environment, status, k8s_release, max_memory_mb, max_clients, maxmemory_policy, default_key_ttl_sec)
		VALUES ($1, 'redis', 'prod', 'pending', $2, $3, $4, $5, $6)
		ON CONFLICT (project_id, engine, environment) DO UPDATE SET
			max_memory_mb = EXCLUDED.max_memory_mb,
			max_clients = EXCLUDED.max_clients,
			maxmemory_policy = EXCLUDED.maxmemory_policy,
			default_key_ttl_sec = EXCLUDED.default_key_ttl_sec,
			status = CASE WHEN project_data_addons.status IN ('stopped', 'failed') THEN 'pending' ELSE project_data_addons.status END,
			updated_at = now()`,
		p.ID, release, dev.MaxMemoryMB, dev.MaxClients, policy, ttl)
	if err != nil {
		return err
	}
	addon, err := h.getProjectAddon(ctx, p.ID, "redis", "prod")
	if err != nil {
		return err
	}
	return h.provisionRedisAddon(ctx, p, "redis", "prod", addon)
}

func (h *Handler) ensureRedisProdForPromote(ctx context.Context, p projectRow) error {
	if !h.devUsesRedisAddon(ctx, p.ID) {
		return nil
	}
	if h.redisProdAddonReady(ctx, p.ID) {
		return nil
	}
	return h.promoteRedisAddonFromDev(ctx, p)
}

// PromoteRedisAddon POST /projects/{slug}/addons/redis/promote-prod
func (h *Handler) PromoteRedisAddon(w http.ResponseWriter, r *http.Request) {
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
	if !auth.CanWriteProd(u.Role) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Chỉ admin/tech_lead được provision Redis prod"})
		return
	}
	if !h.devUsesRedisAddon(r.Context(), p.ID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Dev chưa bật Redis addon"})
		return
	}
	if err := h.promoteRedisAddonFromDev(r.Context(), p); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "Provision Redis prod thất bại: " + err.Error()})
		return
	}
	addon, err := h.getProjectAddon(r.Context(), p.ID, "redis", "prod")
	if err != nil || addon == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Không đọc được addon prod sau provision"})
		return
	}
	reconciled := h.reconcileRedisAddonStatus(r.Context(), p, addon)
	auditAction(r.Context(), h, r, "addon.redis.promote_prod", slug, map[string]any{
		"by": u.Email,
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"message": "Đã provision Redis prod (quota copy từ dev) — REDIS_URL prod đã sync vào app env",
		"addon":   h.enrichRedisAddonAPIView(r.Context(), p, reconciled, true, ""),
	})
}
