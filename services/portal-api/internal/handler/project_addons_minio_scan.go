package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Thien2026/k8s/services/portal-api/internal/auth"
	"github.com/go-chi/chi/v5"
)

const minioClamAVImage = "clamav/clamav:1.5.3"

func minioScanBucketName(bucket, suffix string) string {
	// Bucket name user giới hạn 63 ký tự; suffix nội bộ phải giữ S3-valid.
	base := strings.TrimRight(bucket, "-.")
	max := 63 - len(suffix)
	if len(base) > max {
		base = base[:max]
		base = strings.TrimRight(base, "-.")
	}
	return base + suffix
}

func randomScanID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// EnableMinioBucketScan prepares private quarantine/infected buckets and distinct
// uploader/scanner credentials before changing the bucket policy to required.
// The public app contract is intentionally not changed here; that is done only
// after the scanner worker is installed and verified.
func (h *Handler) EnableMinioBucketScan(w http.ResponseWriter, r *http.Request) {
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
	var bucketID int64
	var status, mode string
	if err := h.db.QueryRow(r.Context(), `SELECT id,status,scan_mode FROM project_minio_buckets WHERE project_id=$1 AND environment=$2 AND name=$3`,
		p.ID, env, name).Scan(&bucketID, &status, &mode); err != nil {
		writeJSON(w, 404, map[string]string{"error": "bucket không tồn tại"})
		return
	}
	if status != "running" {
		writeJSON(w, 400, map[string]string{"error": "bucket chưa sẵn sàng"})
		return
	}
	if mode == "required" {
		writeJSON(w, 409, map[string]string{"error": "scan required đã được bật"})
		return
	}
	// Để publish không chết giữa chừng: cần tối thiểu 1 GiB free ngoài quota
	// các bucket đã phân bổ. Đây là reserve cho double-write quarantine→clean.
	addon, err := h.getProjectAddon(r.Context(), p.ID, "minio", env)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "MinIO chưa sẵn sàng"})
		return
	}
	allocated, err := h.minioBucketAllocatedGB(r.Context(), p.ID, env)
	if err != nil || addon.StorageGB-allocated < 1 {
		writeJSON(w, 400, map[string]string{"error": "cần tối thiểu 1 GiB dung lượng instance chưa phân bổ để bật scan (reserve quarantine → clean)"})
		return
	}
	qBucket := minioScanBucketName(name, "-quarantine")
	iBucket := minioScanBucketName(name, "-infected")
	release := minioAddonRelease(p.Slug, env)
	scannerSecret := release + "-bucket-" + name + "-scanner"
	uploaderSecret := release + "-bucket-" + name + "-uploader"
	_, err = h.db.Exec(r.Context(), `INSERT INTO project_minio_scan_profiles
		(bucket_id,quarantine_bucket,infected_bucket,scanner_connection_secret,uploader_connection_secret,status)
		VALUES ($1,$2,$3,$4,$5,'provisioning')
		ON CONFLICT (bucket_id) DO UPDATE SET status='provisioning',error_message=NULL,updated_at=now()`,
		bucketID, qBucket, iBucket, scannerSecret, uploaderSecret)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if err := h.provisionMinioScanProfile(r.Context(), p, env, name, qBucket, iBucket, scannerSecret, uploaderSecret); err != nil {
		_, _ = h.db.Exec(r.Context(), `UPDATE project_minio_scan_profiles SET status='failed',error_message=$1,updated_at=now() WHERE bucket_id=$2`, err.Error(), bucketID)
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}
	if err := h.setMinioCleanBucketReadOnly(r.Context(), p, env, name); err != nil {
		_, _ = h.db.Exec(r.Context(), `UPDATE project_minio_scan_profiles SET status='failed',error_message=$1,updated_at=now() WHERE bucket_id=$2`, err.Error(), bucketID)
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}
	_, _ = h.db.Exec(r.Context(), `UPDATE project_minio_scan_profiles SET status='ready',error_message=NULL,updated_at=now() WHERE bucket_id=$1`, bucketID)
	_, _ = h.db.Exec(r.Context(), `UPDATE project_minio_buckets SET scan_mode='required',updated_at=now() WHERE id=$1`, bucketID)
	auditAction(r.Context(), h, r, "minio.bucket.scan.enabled", p.Slug, map[string]any{"environment": env, "bucket": name})
	writeJSON(w, 200, map[string]any{"ok": true, "bucket": name, "scan_mode": "required"})
}

// setMinioCleanBucketReadOnly removes Put/Delete from the app service account.
// It is deliberately called before scan_mode=required, closing the direct-clean
// bypass before any caller can receive a "scan required" response.
func (h *Handler) setMinioCleanBucketReadOnly(ctx context.Context, p projectRow, env, bucket string) error {
	ns := h.projectNamespace(p, env)
	release := minioAddonRelease(p.Slug, env)
	authSecret := release + "-auth"
	var connSecret string
	if err := h.db.QueryRow(ctx, `SELECT connection_secret FROM project_minio_buckets
		WHERE project_id=$1 AND environment=$2 AND name=$3 AND status='running'`, p.ID, env, bucket).Scan(&connSecret); err != nil {
		return fmt.Errorf("không tìm thấy credential bucket clean")
	}
	jobName := fmt.Sprintf("%s-scan-lock-clean-%d", release, time.Now().UnixNano()%10000000000)
	script := `set -eu
i=0; until mc alias set local "$MINIO_ENDPOINT" "$MINIO_ROOT_USER" "$MINIO_ROOT_PASSWORD"; do i=$((i+1)); [ "$i" -gt 30 ] && exit 1; sleep 2; done
cat >/tmp/readonly.json <<EOF
{"Version":"2012-10-17","Statement":[
{"Effect":"Allow","Action":["s3:ListBucket","s3:GetBucketLocation","s3:ListBucketMultipartUploads"],"Resource":["arn:aws:s3:::${CLEAN_BUCKET}"]},
{"Effect":"Allow","Action":["s3:GetObject","s3:ListMultipartUploadParts"],"Resource":["arn:aws:s3:::${CLEAN_BUCKET}/*"]}
]}
EOF
mc admin user svcacct edit local "$APP_KEY" --secret-key "$APP_SECRET" --policy /tmp/readonly.json
mc admin user svcacct info local "$APP_KEY" >/dev/null`
	job := map[string]any{"apiVersion": "batch/v1", "kind": "Job",
		"metadata": map[string]any{"name": jobName, "labels": map[string]string{"app.kubernetes.io/name": "minio-scan-lock-clean", "app.kubernetes.io/instance": release}},
		"spec": map[string]any{"ttlSecondsAfterFinished": 300, "backoffLimit": 2, "template": map[string]any{"metadata": map[string]any{"labels": map[string]string{"platform/minio-access": "scanner"}}, "spec": map[string]any{
			"restartPolicy": "OnFailure", "containers": []map[string]any{{"name": "mc", "image": minioMCImage, "command": []string{"/bin/sh", "-ec"}, "args": []string{script},
				"env": []map[string]any{
					{"name": "MINIO_ROOT_USER", "valueFrom": map[string]any{"secretKeyRef": map[string]string{"name": authSecret, "key": "rootUser"}}},
					{"name": "MINIO_ROOT_PASSWORD", "valueFrom": map[string]any{"secretKeyRef": map[string]string{"name": authSecret, "key": "rootPassword"}}},
					{"name": "APP_KEY", "valueFrom": map[string]any{"secretKeyRef": map[string]string{"name": connSecret, "key": "S3_ACCESS_KEY"}}},
					{"name": "APP_SECRET", "valueFrom": map[string]any{"secretKeyRef": map[string]string{"name": connSecret, "key": "S3_SECRET_KEY"}}},
					{"name": "MINIO_ENDPOINT", "value": h.minioEndpoint(release, ns)}, {"name": "CLEAN_BUCKET", "value": bucket},
				}}}}}}}
	raw, _ := json.Marshal(job)
	if err := h.rancher.ApplyNamespacedObject(ctx, "", "/apis/batch/v1/jobs", ns, raw); err != nil {
		return fmt.Errorf("tạo job khóa quyền clean: %w", err)
	}
	return h.waitMinioInitJob(ctx, ns, jobName)
}

func (h *Handler) provisionMinioScanProfile(ctx context.Context, p projectRow, env, cleanBucket, quarantineBucket, infectedBucket, scannerSecret, uploaderSecret string) error {
	ns := h.projectNamespace(p, env)
	release := minioAddonRelease(p.Slug, env)
	authSecret := release + "-auth"
	scannerKey, err := randomMinioAccessKey()
	if err != nil {
		return err
	}
	scannerPass, err := randomMinioSecretKey()
	if err != nil {
		return err
	}
	uploaderKey, err := randomMinioAccessKey()
	if err != nil {
		return err
	}
	uploaderPass, err := randomMinioSecretKey()
	if err != nil {
		return err
	}
	endpoint := h.minioEndpoint(release, ns)
	jobName := fmt.Sprintf("%s-scan-profile-%d", release, time.Now().UnixNano()%10000000000)
	script := `set -eu
i=0; until mc alias set local "$MINIO_ENDPOINT" "$MINIO_ROOT_USER" "$MINIO_ROOT_PASSWORD"; do i=$((i+1)); [ "$i" -gt 30 ] && exit 1; sleep 2; done
for b in "$QUARANTINE_BUCKET" "$INFECTED_BUCKET"; do mc mb -p "local/$b"; mc anonymous set none "local/$b"; done
cat >/tmp/uploader.json <<EOF
{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:PutObject","s3:AbortMultipartUpload","s3:ListMultipartUploadParts"],"Resource":["arn:aws:s3:::${QUARANTINE_BUCKET}/*"]}]}
EOF
cat >/tmp/scanner.json <<EOF
{"Version":"2012-10-17","Statement":[
{"Effect":"Allow","Action":["s3:ListBucket","s3:GetBucketLocation"],"Resource":["arn:aws:s3:::${QUARANTINE_BUCKET}","arn:aws:s3:::${CLEAN_BUCKET}","arn:aws:s3:::${INFECTED_BUCKET}"]},
{"Effect":"Allow","Action":["s3:GetObject","s3:DeleteObject"],"Resource":["arn:aws:s3:::${QUARANTINE_BUCKET}/*"]},
{"Effect":"Allow","Action":["s3:PutObject","s3:GetObject","s3:DeleteObject","s3:AbortMultipartUpload","s3:ListMultipartUploadParts"],"Resource":["arn:aws:s3:::${CLEAN_BUCKET}/*","arn:aws:s3:::${INFECTED_BUCKET}/*"]}
]}
EOF
mc admin user svcacct add local "$MINIO_ROOT_USER" --access-key "$UPLOADER_KEY" --secret-key "$UPLOADER_SECRET" --policy /tmp/uploader.json
mc admin user svcacct add local "$MINIO_ROOT_USER" --access-key "$SCANNER_KEY" --secret-key "$SCANNER_SECRET" --policy /tmp/scanner.json
`
	job := map[string]any{"apiVersion": "batch/v1", "kind": "Job",
		"metadata": map[string]any{"name": jobName, "labels": map[string]string{"app.kubernetes.io/name": "minio-scan-profile", "app.kubernetes.io/instance": release}},
		"spec": map[string]any{"ttlSecondsAfterFinished": 300, "backoffLimit": 2, "template": map[string]any{"metadata": map[string]any{"labels": map[string]string{"platform/minio-access": "scanner"}}, "spec": map[string]any{
			"restartPolicy": "OnFailure", "containers": []map[string]any{{"name": "mc", "image": minioMCImage, "command": []string{"/bin/sh", "-ec"}, "args": []string{script},
				"env": []map[string]any{
					{"name": "MINIO_ROOT_USER", "valueFrom": map[string]any{"secretKeyRef": map[string]string{"name": authSecret, "key": "rootUser"}}},
					{"name": "MINIO_ROOT_PASSWORD", "valueFrom": map[string]any{"secretKeyRef": map[string]string{"name": authSecret, "key": "rootPassword"}}},
					{"name": "MINIO_ENDPOINT", "value": endpoint}, {"name": "CLEAN_BUCKET", "value": cleanBucket}, {"name": "QUARANTINE_BUCKET", "value": quarantineBucket}, {"name": "INFECTED_BUCKET", "value": infectedBucket},
					{"name": "SCANNER_KEY", "value": scannerKey}, {"name": "SCANNER_SECRET", "value": scannerPass}, {"name": "UPLOADER_KEY", "value": uploaderKey}, {"name": "UPLOADER_SECRET", "value": uploaderPass},
				}}}}}}}
	raw, _ := json.Marshal(job)
	if err := h.rancher.ApplyNamespacedObject(ctx, "", "/apis/batch/v1/jobs", ns, raw); err != nil {
		return err
	}
	if err := h.waitMinioInitJob(ctx, ns, jobName); err != nil {
		return err
	}
	for secretName, values := range map[string][3]string{
		scannerSecret:  {scannerKey, scannerPass, cleanBucket},
		uploaderSecret: {uploaderKey, uploaderPass, quarantineBucket},
	} {
		accessKey, secretKey, bucket := values[0], values[1], values[2]
		secret := map[string]any{"apiVersion": "v1", "kind": "Secret", "metadata": map[string]any{"name": secretName, "labels": map[string]string{"app.kubernetes.io/name": "minio-scan", "app.kubernetes.io/instance": release}},
			"type": "Opaque", "stringData": map[string]string{"S3_ENDPOINT": endpoint, "S3_ACCESS_KEY": accessKey, "S3_SECRET_KEY": secretKey, "S3_BUCKET": bucket, "S3_REGION": "us-east-1", "S3_USE_SSL": "false"}}
		raw, _ := json.Marshal(secret)
		if err := h.rancher.ApplyNamespacedObject(ctx, "", "/api/v1/secrets", ns, raw); err != nil {
			return err
		}
	}
	return nil
}
