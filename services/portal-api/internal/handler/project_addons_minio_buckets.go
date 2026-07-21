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
	"github.com/go-chi/chi/v5"
)

var minioBucketNameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)

type minioBucketView struct {
	ID            int64      `json:"id"`
	Name          string     `json:"name"`
	StorageGB     int        `json:"storage_gb"`
	MaxObjectMB   int        `json:"max_object_mb"`
	Status        string     `json:"status"`
	IsDefault     bool       `json:"is_default"`
	Connection    string     `json:"connection_secret,omitempty"`
	ScanMode      string     `json:"scan_mode"`
	ScanReady     bool       `json:"scan_ready"`
	QuotaVerified *time.Time `json:"quota_verified_at,omitempty"`
	QuotaError    *string    `json:"quota_verify_error,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

func normalizeMinioBucketName(raw string) (string, error) {
	name := strings.ToLower(strings.TrimSpace(raw))
	if !minioBucketNameRE.MatchString(name) || strings.Contains(name, "..") {
		return "", fmt.Errorf("tên bucket phải dài 3–63 ký tự, chỉ gồm a-z, 0-9, dấu chấm hoặc gạch ngang")
	}
	return name, nil
}

func (h *Handler) listMinioBuckets(ctx context.Context, projectID int64, env string) ([]minioBucketView, error) {
	rows, err := h.db.Query(ctx, `SELECT b.id,b.name,b.storage_gb,b.max_object_mb,b.status,b.is_default,b.connection_secret,b.quota_verified_at,b.quota_verify_error,b.scan_mode,
		EXISTS(SELECT 1 FROM project_minio_scan_profiles sp WHERE sp.bucket_id=b.id AND sp.status='ready'),b.created_at
		FROM project_minio_buckets b WHERE b.project_id=$1 AND b.environment=$2 ORDER BY b.is_default DESC,b.name`, projectID, env)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []minioBucketView{}
	for rows.Next() {
		var v minioBucketView
		if err := rows.Scan(&v.ID, &v.Name, &v.StorageGB, &v.MaxObjectMB, &v.Status, &v.IsDefault, &v.Connection, &v.QuotaVerified, &v.QuotaError, &v.ScanMode, &v.ScanReady, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (h *Handler) minioBucketAllocatedGB(ctx context.Context, projectID int64, env string) (int, error) {
	var total int
	err := h.db.QueryRow(ctx, `SELECT COALESCE(SUM(storage_gb),0) FROM project_minio_buckets
		WHERE project_id=$1 AND environment=$2 AND status <> 'deleting'`, projectID, env).Scan(&total)
	return total, err
}

// ListMinioBuckets returns bucket metadata only; access keys are never listed.
func (h *Handler) ListMinioBuckets(w http.ResponseWriter, r *http.Request) {
	p, ok := h.requireProjectAccess(w, r, chi.URLParam(r, "slug"))
	if !ok {
		return
	}
	env := strings.TrimSpace(r.URL.Query().Get("environment"))
	if !validAddonEnv(env) {
		env = "dev"
	}
	addon, err := h.getProjectAddon(r.Context(), p.ID, "minio", env)
	if err != nil || addon.Status != "running" {
		writeJSON(w, 400, map[string]string{"error": "MinIO chưa sẵn sàng"})
		return
	}
	items, err := h.listMinioBuckets(r.Context(), p.ID, env)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	allocated, _ := h.minioBucketAllocatedGB(r.Context(), p.ID, env)
	writeJSON(w, 200, map[string]any{"items": items, "instance_storage_gb": addon.StorageGB, "allocated_storage_gb": allocated, "free_storage_gb": max(0, addon.StorageGB-allocated)})
}

// CreateMinioBucket makes an isolated bucket + least-privilege service account.
func (h *Handler) CreateMinioBucket(w http.ResponseWriter, r *http.Request) {
	p, ok := h.requireProjectAccess(w, r, chi.URLParam(r, "slug"))
	if !ok {
		return
	}
	u, _ := auth.UserFromContext(r.Context())
	if !h.canManageProject(r.Context(), u, p.ID) {
		writeAccessDenied(w)
		return
	}
	var body struct {
		Environment string `json:"environment"`
		Name        string `json:"name"`
		StorageGB   int    `json:"storage_gb"`
		MaxObjectMB int    `json:"max_object_mb"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]string{"error": "body JSON không hợp lệ"})
		return
	}
	if !validAddonEnv(body.Environment) {
		writeJSON(w, 400, map[string]string{"error": "environment phải là dev hoặc prod"})
		return
	}
	name, err := normalizeMinioBucketName(body.Name)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	if body.StorageGB < 1 {
		writeJSON(w, 400, map[string]string{"error": "storage_gb phải từ 1 GiB"})
		return
	}
	addon, err := h.getProjectAddon(r.Context(), p.ID, "minio", body.Environment)
	if err != nil || addon.Status != "running" {
		writeJSON(w, 400, map[string]string{"error": "MinIO chưa sẵn sàng"})
		return
	}
	if body.MaxObjectMB == 0 {
		body.MaxObjectMB = minioDefaultMaxObjectMB
	}
	if body.MaxObjectMB < 1 || body.MaxObjectMB > addon.MaxObjectMB {
		writeJSON(w, 400, map[string]string{"error": fmt.Sprintf("max_object_mb phải từ 1 đến %d", addon.MaxObjectMB)})
		return
	}
	allocated, err := h.minioBucketAllocatedGB(r.Context(), p.ID, body.Environment)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if allocated+body.StorageGB > addon.StorageGB {
		writeJSON(w, 400, map[string]any{"error": "vượt ngân sách MinIO instance; hãy giảm quota bucket hiện hữu hoặc tăng storage instance trước", "code": "bucket_quota_exceeded", "free_storage_gb": max(0, addon.StorageGB-allocated)})
		return
	}
	release := minioAddonRelease(p.Slug, body.Environment)
	secretName := release + "-bucket-" + name + "-connection"
	var id int64
	err = h.db.QueryRow(r.Context(), `INSERT INTO project_minio_buckets
		(project_id,environment,name,storage_gb,max_object_mb,connection_secret,status,created_by)
		VALUES ($1,$2,$3,$4,$5,$6,'provisioning',$7) RETURNING id`,
		p.ID, body.Environment, name, body.StorageGB, body.MaxObjectMB, secretName, u.ID).Scan(&id)
	if err != nil {
		writeJSON(w, 409, map[string]string{"error": "bucket đã tồn tại hoặc không thể tạo"})
		return
	}
	creds, err := h.provisionMinioBucket(r.Context(), p, body.Environment, release, name, body.StorageGB, body.MaxObjectMB, secretName)
	if err != nil {
		_, _ = h.db.Exec(r.Context(), `UPDATE project_minio_buckets SET status='failed',quota_verify_error=$1,updated_at=now() WHERE id=$2`, err.Error(), id)
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	_, _ = h.db.Exec(r.Context(), `UPDATE project_minio_buckets SET status='running',quota_verified_at=now(),quota_verify_error=NULL,updated_at=now() WHERE id=$1`, id)
	auditAction(r.Context(), h, r, "minio.bucket.created", p.Slug, map[string]any{"environment": body.Environment, "bucket": name, "storage_gb": body.StorageGB})
	// Secret chỉ trả đúng một lần tại lúc tạo; endpoint/key của bucket cũ không bao giờ list qua API.
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "name": name, "status": "running", "connection": creds})
}

// UpdateMinioBucketQuota only accepts a new partition that fits the instance budget.
// Native MinIO quota is updated and read back before the database is changed.
func (h *Handler) UpdateMinioBucketQuota(w http.ResponseWriter, r *http.Request) {
	p, ok := h.requireProjectAccess(w, r, chi.URLParam(r, "slug"))
	if !ok {
		return
	}
	u, _ := auth.UserFromContext(r.Context())
	if !h.canManageProject(r.Context(), u, p.ID) {
		writeAccessDenied(w)
		return
	}
	env := strings.TrimSpace(r.URL.Query().Get("environment"))
	if !validAddonEnv(env) {
		writeJSON(w, 400, map[string]string{"error": "environment phải là dev hoặc prod"})
		return
	}
	name, err := normalizeMinioBucketName(chi.URLParam(r, "bucket"))
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	var body struct {
		StorageGB int `json:"storage_gb"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.StorageGB < 1 {
		writeJSON(w, 400, map[string]string{"error": "storage_gb phải từ 1 GiB"})
		return
	}
	var current int
	err = h.db.QueryRow(r.Context(), `SELECT storage_gb FROM project_minio_buckets WHERE project_id=$1 AND environment=$2 AND name=$3 AND status='running'`,
		p.ID, env, name).Scan(&current)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "bucket không tồn tại hoặc chưa sẵn sàng"})
		return
	}
	addon, err := h.getProjectAddon(r.Context(), p.ID, "minio", env)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "MinIO chưa sẵn sàng"})
		return
	}
	allocated, err := h.minioBucketAllocatedGB(r.Context(), p.ID, env)
	if err != nil || allocated-current+body.StorageGB > addon.StorageGB {
		free := addon.StorageGB - (allocated - current)
		writeJSON(w, 400, map[string]any{"error": "quota bucket vượt ngân sách instance", "max_storage_gb": max(0, free)})
		return
	}
	if err := h.setMinioBucketQuota(r.Context(), p, env, name, body.StorageGB); err != nil {
		_, _ = h.db.Exec(r.Context(), `UPDATE project_minio_buckets SET quota_verify_error=$1,updated_at=now() WHERE project_id=$2 AND environment=$3 AND name=$4`, err.Error(), p.ID, env, name)
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}
	_, _ = h.db.Exec(r.Context(), `UPDATE project_minio_buckets SET storage_gb=$1,quota_verified_at=now(),quota_verify_error=NULL,updated_at=now()
		WHERE project_id=$2 AND environment=$3 AND name=$4`, body.StorageGB, p.ID, env, name)
	auditAction(r.Context(), h, r, "minio.bucket.quota.updated", p.Slug, map[string]any{"environment": env, "bucket": name, "storage_gb": body.StorageGB})
	writeJSON(w, 200, map[string]any{"ok": true, "bucket": name, "storage_gb": body.StorageGB})
}

// GetMinioBucketScanStatus returns only workflow metadata; quarantined bytes are
// never exposed as downloadable objects through this endpoint.
func (h *Handler) GetMinioBucketScanStatus(w http.ResponseWriter, r *http.Request) {
	p, ok := h.requireProjectAccess(w, r, chi.URLParam(r, "slug"))
	if !ok {
		return
	}
	env := strings.TrimSpace(r.URL.Query().Get("environment"))
	if !validAddonEnv(env) {
		env = "dev"
	}
	name, err := normalizeMinioBucketName(chi.URLParam(r, "bucket"))
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	var bucketID int64
	var scanMode string
	if err := h.db.QueryRow(r.Context(), `SELECT id,scan_mode FROM project_minio_buckets WHERE project_id=$1 AND environment=$2 AND name=$3`,
		p.ID, env, name).Scan(&bucketID, &scanMode); err != nil {
		writeJSON(w, 404, map[string]string{"error": "bucket không tồn tại"})
		return
	}
	rows, err := h.db.Query(r.Context(), `SELECT id,destination_key,object_size,content_type,status,attempts,detection_name,error_message,created_at,started_at,finished_at
		FROM project_minio_object_scans WHERE bucket_id=$1 ORDER BY created_at DESC LIMIT 100`, bucketID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, key, contentType, status string
		var size int64
		var attempts int
		var detection, scanErr *string
		var created time.Time
		var started, finished *time.Time
		if err := rows.Scan(&id, &key, &size, &contentType, &status, &attempts, &detection, &scanErr, &created, &started, &finished); err == nil {
			items = append(items, map[string]any{"id": id, "key": key, "size": size, "content_type": contentType, "status": status, "attempts": attempts, "detection_name": detection, "error_message": scanErr, "created_at": created, "started_at": started, "finished_at": finished})
		}
	}
	writeJSON(w, 200, map[string]any{"bucket": name, "scan_mode": scanMode, "items": items})
}

func (h *Handler) setMinioBucketQuota(ctx context.Context, p projectRow, env, bucket string, storageGB int) error {
	ns := h.projectNamespace(p, env)
	release := minioAddonRelease(p.Slug, env)
	authSecret := release + "-auth"
	jobName := fmt.Sprintf("%s-bucket-quota-%d", release, time.Now().UnixNano()%10000000000)
	job := map[string]any{"apiVersion": "batch/v1", "kind": "Job",
		"metadata": map[string]any{"name": jobName, "labels": map[string]string{"app.kubernetes.io/name": "minio-bucket-quota", "app.kubernetes.io/instance": release}},
		"spec": map[string]any{"ttlSecondsAfterFinished": 300, "backoffLimit": 3, "template": map[string]any{"spec": map[string]any{
			"restartPolicy": "OnFailure", "containers": []map[string]any{{"name": "mc", "image": minioMCImage,
				"env": []map[string]any{
					{"name": "MINIO_ROOT_USER", "valueFrom": map[string]any{"secretKeyRef": map[string]string{"name": authSecret, "key": "rootUser"}}},
					{"name": "MINIO_ROOT_PASSWORD", "valueFrom": map[string]any{"secretKeyRef": map[string]string{"name": authSecret, "key": "rootPassword"}}},
					{"name": "MINIO_ENDPOINT", "value": h.minioEndpoint(release, ns)}, {"name": "MINIO_BUCKET", "value": bucket}, {"name": "MINIO_STORAGE_GB", "value": fmt.Sprintf("%d", storageGB)},
				},
				"command": []string{"/bin/sh", "-c"}, "args": []string{`set -eu
i=0; until mc alias set local "$MINIO_ENDPOINT" "$MINIO_ROOT_USER" "$MINIO_ROOT_PASSWORD"; do i=$((i+1)); [ "$i" -gt 30 ] && exit 1; sleep 2; done
mc quota set "local/$MINIO_BUCKET" --size "${MINIO_STORAGE_GB}gi" || mc admin bucket quota "local/$MINIO_BUCKET" --hard "${MINIO_STORAGE_GB}gb"
mc quota info "local/$MINIO_BUCKET"`}}}}}}}
	raw, _ := json.Marshal(job)
	if err := h.rancher.ApplyNamespacedObject(ctx, "", "/apis/batch/v1/jobs", ns, raw); err != nil {
		return fmt.Errorf("tạo job quota bucket: %w", err)
	}
	return h.waitMinioInitJob(ctx, ns, jobName)
}

func (h *Handler) provisionMinioBucket(ctx context.Context, p projectRow, env, release, bucket string, storageGB, maxObjectMB int, secretName string) (map[string]string, error) {
	ns := h.projectNamespace(p, env)
	authSecret := release + "-auth"
	if _, found, err := h.rancher.GetOpaqueSecretData(ctx, "", ns, authSecret); err != nil || !found {
		return nil, fmt.Errorf("MinIO root credential chưa sẵn sàng")
	}
	accessKey, err := randomMinioAccessKey()
	if err != nil {
		return nil, err
	}
	secretKey, err := randomMinioSecretKey()
	if err != nil {
		return nil, err
	}
	endpoint := h.minioEndpoint(release, ns)
	jobName := fmt.Sprintf("%s-bucket-%s-%d", release, bucket, time.Now().UnixNano()%10000000000)
	job := map[string]any{
		"apiVersion": "batch/v1", "kind": "Job",
		"metadata": map[string]any{"name": jobName, "labels": map[string]string{"app.kubernetes.io/name": "minio-bucket-init", "app.kubernetes.io/instance": release}},
		"spec": map[string]any{
			"ttlSecondsAfterFinished": 300, "backoffLimit": 3,
			"template": map[string]any{"spec": map[string]any{
				"restartPolicy": "OnFailure",
				"containers": []map[string]any{{"name": "mc", "image": minioMCImage,
					"env": []map[string]any{
						{"name": "MINIO_ROOT_USER", "valueFrom": map[string]any{"secretKeyRef": map[string]string{"name": authSecret, "key": "rootUser"}}},
						{"name": "MINIO_ROOT_PASSWORD", "valueFrom": map[string]any{"secretKeyRef": map[string]string{"name": authSecret, "key": "rootPassword"}}},
						{"name": "MINIO_ENDPOINT", "value": endpoint}, {"name": "MINIO_BUCKET", "value": bucket},
						{"name": "MINIO_STORAGE_GB", "value": fmt.Sprintf("%d", storageGB)},
						{"name": "APP_ACCESS_KEY", "value": accessKey}, {"name": "APP_SECRET_KEY", "value": secretKey},
					},
					"command": []string{"/bin/sh", "-c"},
					"args": []string{`set -eu
i=0; until mc alias set local "$MINIO_ENDPOINT" "$MINIO_ROOT_USER" "$MINIO_ROOT_PASSWORD"; do i=$((i+1)); [ "$i" -gt 30 ] && exit 1; sleep 2; done
mc mb -p "local/$MINIO_BUCKET"
mc anonymous set none "local/$MINIO_BUCKET"
mc quota set "local/$MINIO_BUCKET" --size "${MINIO_STORAGE_GB}gi" || mc admin bucket quota "local/$MINIO_BUCKET" --hard "${MINIO_STORAGE_GB}gb"
mc quota info "local/$MINIO_BUCKET"
cat >/tmp/policy.json <<EOF
{"Version":"2012-10-17","Statement":[
{"Effect":"Allow","Action":["s3:ListBucket","s3:GetBucketLocation","s3:ListBucketMultipartUploads"],"Resource":["arn:aws:s3:::${MINIO_BUCKET}"]},
{"Effect":"Allow","Action":["s3:GetObject","s3:PutObject","s3:DeleteObject","s3:AbortMultipartUpload","s3:ListMultipartUploadParts"],"Resource":["arn:aws:s3:::${MINIO_BUCKET}/*"]}
]}
EOF
mc admin user svcacct add local "$MINIO_ROOT_USER" --access-key "$APP_ACCESS_KEY" --secret-key "$APP_SECRET_KEY" --policy /tmp/policy.json
mc admin user svcacct info local "$APP_ACCESS_KEY" >/dev/null
`}}}}}},
	}
	raw, _ := json.Marshal(job)
	if err := h.rancher.ApplyNamespacedObject(ctx, "", "/apis/batch/v1/jobs", ns, raw); err != nil {
		return nil, fmt.Errorf("tạo job init bucket: %w", err)
	}
	if err := h.waitMinioInitJob(ctx, ns, jobName); err != nil {
		return nil, err
	}
	conn := map[string]any{"apiVersion": "v1", "kind": "Secret", "metadata": map[string]any{"name": secretName, "labels": map[string]string{"app.kubernetes.io/name": "minio-bucket", "app.kubernetes.io/instance": release}},
		"type": "Opaque", "stringData": map[string]string{"S3_ENDPOINT": endpoint, "S3_ACCESS_KEY": accessKey, "S3_SECRET_KEY": secretKey, "S3_BUCKET": bucket, "S3_REGION": "us-east-1", "S3_USE_SSL": "false", "S3_MAX_OBJECT_MB": fmt.Sprintf("%d", maxObjectMB)}}
	raw, _ = json.Marshal(conn)
	if err := h.rancher.ApplyNamespacedObject(ctx, "", "/api/v1/secrets", ns, raw); err != nil {
		return nil, fmt.Errorf("lưu connection secret: %w", err)
	}
	return map[string]string{"endpoint": endpoint, "access_key": accessKey, "secret_key": secretKey, "bucket": bucket, "region": "us-east-1"}, nil
}
