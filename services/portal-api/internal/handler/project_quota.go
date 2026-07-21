package handler

import (
	"context"
	"encoding/json"
	"fmt"
)

// projectEnvQuota — trần tài nguyên 1 (project × env). Admin quản; thực thi bằng K8s ResourceQuota (B2).
type projectEnvQuota struct {
	ProjectID   int64  `json:"project_id"`
	Environment string `json:"environment"`
	StorageGB   int    `json:"storage_gb"`
	MemoryMB    int    `json:"memory_mb"`
	CPUm        int    `json:"cpu_m"`
	MaxPods     int    `json:"max_pods"`
	MaxPVCs     int    `json:"max_pvcs"`
	IsOverride  bool   `json:"is_override"`
	UpdatedBy   *int64 `json:"updated_by,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

// defaultQuotaForEnv trả về quota mặc định cho môi trường từ platform policy.
func (p platformPolicy) defaultQuotaForEnv(env string) projectEnvQuota {
	q := projectEnvQuota{
		Environment: env,
		MaxPods:     p.DefaultQuotaMaxPods,
		MaxPVCs:     p.DefaultQuotaMaxPVCs,
	}
	if env == "prod" {
		q.StorageGB = p.DefaultQuotaStorageGBProd
		q.MemoryMB = p.DefaultQuotaMemoryMBProd
		q.CPUm = p.DefaultQuotaCPUmProd
	} else {
		q.StorageGB = p.DefaultQuotaStorageGBDev
		q.MemoryMB = p.DefaultQuotaMemoryMBDev
		q.CPUm = p.DefaultQuotaCPUmDev
	}
	return q
}

// seedProjectEnvQuota tạo 2 row quota (dev+prod) mặc định cho project mới. Idempotent.
func (h *Handler) seedProjectEnvQuota(ctx context.Context, projectID int64) error {
	pol, _, err := h.loadPlatformPolicy(ctx)
	if err != nil {
		return err
	}
	for _, env := range []string{"dev", "prod"} {
		q := pol.defaultQuotaForEnv(env)
		if _, err := h.db.Exec(ctx, `
			INSERT INTO project_env_quota
				(project_id, environment, storage_gb, memory_mb, cpu_m, max_pods, max_pvcs)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (project_id, environment) DO NOTHING`,
			projectID, env, q.StorageGB, q.MemoryMB, q.CPUm, q.MaxPods, q.MaxPVCs); err != nil {
			return fmt.Errorf("seed quota %s: %w", env, err)
		}
	}
	return nil
}

const (
	projectResourceQuotaName = "project-quota"
	projectLimitRangeName     = "project-default-limits"
)

// applyProjectLimitRange bảo đảm container không khai resources vẫn có request/limit an toàn.
// Giá trị khớp preset "Mặc định platform" của deploy manifest.
func (h *Handler) applyProjectLimitRange(ctx context.Context, namespace string) error {
	if h.rancher == nil || !h.rancher.Enabled() {
		return fmt.Errorf("Rancher chưa sẵn sàng")
	}
	if namespace == "" {
		return nil
	}
	lr := map[string]any{
		"apiVersion": "v1",
		"kind":       "LimitRange",
		"metadata": map[string]any{
			"name":      projectLimitRangeName,
			"namespace": namespace,
			"labels": map[string]string{
				"app.kubernetes.io/managed-by": "platform-console",
				"platform/quota":               "project-env",
			},
		},
		"spec": map[string]any{
			"limits": []map[string]any{
				{
					"type": "Container",
					"defaultRequest": map[string]string{
						"cpu":    "100m",
						"memory": "128Mi",
					},
					"default": map[string]string{
						"cpu":    "500m",
						"memory": "512Mi",
					},
				},
			},
		},
	}
	raw, _ := json.Marshal(lr)
	return h.rancher.ApplyNamespacedObject(ctx, "", "/api/v1/limitranges", namespace, raw)
}

// applyProjectEnvResourceQuota tạo/cập nhật trần tổng storage/CPU/RAM/số lượng cho namespace.
// LimitRange phải được apply trước để pod không khai resources vẫn được admission bổ sung mặc định.
func (h *Handler) applyProjectEnvResourceQuota(ctx context.Context, namespace string, q projectEnvQuota) error {
	if h.rancher == nil || !h.rancher.Enabled() {
		return fmt.Errorf("Rancher chưa sẵn sàng")
	}
	if namespace == "" {
		return nil
	}
	rq := map[string]any{
		"apiVersion": "v1",
		"kind":       "ResourceQuota",
		"metadata": map[string]any{
			"name":      projectResourceQuotaName,
			"namespace": namespace,
			"labels": map[string]string{
				"app.kubernetes.io/managed-by": "platform-console",
				"platform/quota":               "project-env",
			},
		},
		"spec": map[string]any{
			"hard": map[string]string{
				"requests.storage":       fmt.Sprintf("%dGi", q.StorageGB),
				"requests.cpu":           fmt.Sprintf("%dm", q.CPUm),
				"limits.cpu":             fmt.Sprintf("%dm", q.CPUm),
				"requests.memory":        fmt.Sprintf("%dMi", q.MemoryMB),
				"limits.memory":          fmt.Sprintf("%dMi", q.MemoryMB),
				"persistentvolumeclaims": fmt.Sprintf("%d", q.MaxPVCs),
				"count/pods":             fmt.Sprintf("%d", q.MaxPods),
			},
		},
	}
	raw, _ := json.Marshal(rq)
	return h.rancher.ApplyNamespacedObject(ctx, "", "/api/v1/resourcequotas", namespace, raw)
}

// syncProjectEnvQuota đọc quota DB rồi apply ResourceQuota lên namespace tương ứng.
func (h *Handler) syncProjectEnvQuota(ctx context.Context, p projectRow, env string) error {
	q, err := h.loadProjectEnvQuota(ctx, p.ID, env)
	if err != nil {
		return err
	}
	ns := h.projectNamespace(p, env)
	if err := h.applyProjectLimitRange(ctx, ns); err != nil {
		return fmt.Errorf("limitrange %s: %w", env, err)
	}
	return h.applyProjectEnvResourceQuota(ctx, ns, q)
}

// minioStorageAllocatedGB — tổng storage_gb các MinIO addon cùng (project, env), TRỪ 1 addon (excludeEngineEnv).
// Hiện mỗi env chỉ 1 MinIO nên thường = 0; viết dạng SUM để mở sẵn cho multi-bucket/instance sau này.
// Redis không tạo PVC nên không tính. App PVC do dev tự khai — K8s ResourceQuota chặn trực tiếp.
func (h *Handler) minioStorageAllocatedGB(ctx context.Context, projectID int64, env string) (int, error) {
	var total int
	err := h.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(storage_gb), 0)
		FROM project_data_addons
		WHERE project_id = $1 AND environment = $2 AND engine = 'minio'
		  AND status NOT IN ('stopped', 'failed')`,
		projectID, env).Scan(&total)
	return total, err
}

// checkProjectStorageQuota kiểm tra cấp thêm wantGB (thay cho currentGB của addon đang sửa)
// có vượt quota storage của project (env) không. Trả về (ok, quotaGB, allocatedOther).
func (h *Handler) checkProjectStorageQuota(ctx context.Context, projectID int64, env string, wantGB, currentGB int) (bool, int, int, error) {
	q, err := h.loadProjectEnvQuota(ctx, projectID, env)
	if err != nil {
		return false, 0, 0, err
	}
	allocated, err := h.minioStorageAllocatedGB(ctx, projectID, env)
	if err != nil {
		return false, q.StorageGB, 0, err
	}
	// allocated đã bao gồm addon hiện tại (nếu đang running) → trừ currentGB để không double-count.
	other := allocated - currentGB
	if other < 0 {
		other = 0
	}
	return other+wantGB <= q.StorageGB, q.StorageGB, other, nil
}

// backfillAllProjectQuotas apply ResourceQuota cho mọi project hiện có (dev+prod).
// Dùng khi rollout B2 lần đầu — namespace cũ chưa có ResourceQuota.
func (h *Handler) backfillAllProjectQuotas(ctx context.Context) (int, []string) {
	var warns []string
	applied := 0
	rows, err := h.db.Query(ctx, `SELECT id, slug, namespace_dev, namespace_prod FROM projects ORDER BY id`)
	if err != nil {
		return 0, []string{err.Error()}
	}
	var projs []projectRow
	for rows.Next() {
		var p projectRow
		if err := rows.Scan(&p.ID, &p.Slug, &p.NamespaceDev, &p.NamespaceProd); err != nil {
			warns = append(warns, err.Error())
			continue
		}
		projs = append(projs, p)
	}
	rows.Close()
	for _, p := range projs {
		for _, env := range []string{"dev", "prod"} {
			if err := h.syncProjectEnvQuota(ctx, p, env); err != nil {
				warns = append(warns, fmt.Sprintf("%s/%s: %s", p.Slug, env, err.Error()))
				continue
			}
			applied++
		}
	}
	return applied, warns
}

// loadProjectEnvQuota đọc quota 1 (project × env). Nếu chưa có row (project cũ), tự seed từ default rồi trả về.
func (h *Handler) loadProjectEnvQuota(ctx context.Context, projectID int64, env string) (projectEnvQuota, error) {
	q := projectEnvQuota{ProjectID: projectID, Environment: env}
	err := h.db.QueryRow(ctx, `
		SELECT storage_gb, memory_mb, cpu_m, max_pods, max_pvcs, is_override, updated_by
		FROM project_env_quota WHERE project_id = $1 AND environment = $2`,
		projectID, env).Scan(
		&q.StorageGB, &q.MemoryMB, &q.CPUm, &q.MaxPods, &q.MaxPVCs, &q.IsOverride, &q.UpdatedBy)
	if err != nil {
		// Project cũ chưa có quota → seed default rồi thử lại 1 lần.
		if seedErr := h.seedProjectEnvQuota(ctx, projectID); seedErr != nil {
			return q, err
		}
		if err2 := h.db.QueryRow(ctx, `
			SELECT storage_gb, memory_mb, cpu_m, max_pods, max_pvcs, is_override, updated_by
			FROM project_env_quota WHERE project_id = $1 AND environment = $2`,
			projectID, env).Scan(
			&q.StorageGB, &q.MemoryMB, &q.CPUm, &q.MaxPods, &q.MaxPVCs, &q.IsOverride, &q.UpdatedBy); err2 != nil {
			return q, err2
		}
	}
	return q, nil
}
