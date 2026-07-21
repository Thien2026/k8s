package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Thien2026/k8s/services/portal-api/internal/auth"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/go-chi/chi/v5"
)

const backupSecretNamespace = "platform"

type backupTarget struct {
	ID                int64      `json:"id"`
	Name              string     `json:"name"`
	Provider          string     `json:"provider"`
	Endpoint          string     `json:"endpoint"`
	Region            string     `json:"region"`
	Bucket            string     `json:"bucket"`
	Prefix            string     `json:"prefix"`
	CredentialsSecret string     `json:"credentials_secret"`
	ScheduleCron      string     `json:"schedule_cron"`
	RetentionDays     int        `json:"retention_days"`
	RetentionCount    int        `json:"retention_count"`
	EncryptionEnabled bool       `json:"encryption_enabled"`
	Enabled           bool       `json:"enabled"`
	LastTestedAt      *time.Time `json:"last_tested_at,omitempty"`
	LastTestError     *string    `json:"last_test_error,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

type backupRun struct {
	ID             int64      `json:"id"`
	TargetID       int64      `json:"target_id"`
	TargetName     string     `json:"target_name,omitempty"`
	RunKind        string     `json:"run_kind"`
	Status         string     `json:"status"`
	RunPrefix      string     `json:"run_prefix"`
	ManifestKey    *string    `json:"manifest_key,omitempty"`
	ArtifactCount  int        `json:"artifact_count"`
	TotalBytes     int64      `json:"total_bytes"`
	ChecksumSHA256 *string    `json:"checksum_sha256,omitempty"`
	ErrorMessage   *string    `json:"error_message,omitempty"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

func validateBackupTarget(t *backupTarget) error {
	t.Name = strings.TrimSpace(t.Name)
	t.Endpoint = strings.TrimRight(strings.TrimSpace(t.Endpoint), "/")
	t.Region = strings.TrimSpace(t.Region)
	t.Bucket = strings.TrimSpace(t.Bucket)
	t.Prefix = strings.Trim(strings.TrimSpace(t.Prefix), "/")
	t.CredentialsSecret = strings.TrimSpace(t.CredentialsSecret)
	t.ScheduleCron = strings.TrimSpace(t.ScheduleCron)
	if t.Provider != "s3" {
		return fmt.Errorf("Phase 1 chỉ hỗ trợ target S3-compatible")
	}
	u, err := url.Parse(t.Endpoint)
	if err != nil || u.Scheme != "https" && u.Scheme != "http" || u.Host == "" {
		return fmt.Errorf("endpoint phải là URL http(s) hợp lệ")
	}
	if t.Name == "" || t.Bucket == "" || t.CredentialsSecret == "" {
		return fmt.Errorf("name, bucket và credentials_secret là bắt buộc")
	}
	if t.Region == "" {
		t.Region = "us-east-1"
	}
	if t.Prefix == "" {
		t.Prefix = "platform-backups"
	}
	if t.ScheduleCron == "" {
		t.ScheduleCron = "0 3 * * *"
	}
	if len(strings.Fields(t.ScheduleCron)) != 5 {
		return fmt.Errorf("schedule_cron phải có 5 trường cron")
	}
	if t.RetentionDays < 1 || t.RetentionDays > 3650 {
		return fmt.Errorf("retention_days phải trong khoảng 1–3650")
	}
	if t.RetentionCount < 0 || t.RetentionCount > 1000 {
		return fmt.Errorf("retention_count phải trong khoảng 1–1000")
	}
	return nil
}

func scanBackupTarget(row interface{ Scan(...any) error }) (backupTarget, error) {
	var t backupTarget
	err := row.Scan(&t.ID, &t.Name, &t.Provider, &t.Endpoint, &t.Region, &t.Bucket, &t.Prefix,
		&t.CredentialsSecret, &t.ScheduleCron, &t.RetentionDays, &t.RetentionCount, &t.EncryptionEnabled, &t.Enabled,
		&t.LastTestedAt, &t.LastTestError, &t.CreatedAt, &t.UpdatedAt)
	return t, err
}

func (h *Handler) ListBackupTargets(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(r.Context(), `SELECT id,name,provider,endpoint,region,bucket,prefix,credentials_secret,schedule_cron,retention_days,retention_count,encryption_enabled,enabled,last_tested_at,last_test_error,created_at,updated_at FROM backup_targets ORDER BY id`)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	items := []backupTarget{}
	for rows.Next() {
		t, e := scanBackupTarget(rows)
		if e == nil {
			items = append(items, t)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "secret_namespace": backupSecretNamespace, "credential_keys": []string{"access_key_id", "secret_access_key"}})
}

func (h *Handler) UpsertBackupTarget(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	var t backupTarget
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		writeJSON(w, 400, map[string]string{"error": "payload không hợp lệ"})
		return
	}
	t.Provider = "s3"
	if err := validateBackupTarget(&t); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	q := `INSERT INTO backup_targets (name,provider,endpoint,region,bucket,prefix,credentials_secret,schedule_cron,retention_days,retention_count,encryption_enabled,enabled,created_by)
	VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
	ON CONFLICT (name) DO UPDATE SET endpoint=EXCLUDED.endpoint,region=EXCLUDED.region,bucket=EXCLUDED.bucket,prefix=EXCLUDED.prefix,credentials_secret=EXCLUDED.credentials_secret,schedule_cron=EXCLUDED.schedule_cron,retention_days=EXCLUDED.retention_days,retention_count=EXCLUDED.retention_count,encryption_enabled=EXCLUDED.encryption_enabled,enabled=EXCLUDED.enabled,updated_at=now()
	RETURNING id,name,provider,endpoint,region,bucket,prefix,credentials_secret,schedule_cron,retention_days,retention_count,encryption_enabled,enabled,last_tested_at,last_test_error,created_at,updated_at`
	if t.RetentionCount == 0 {
		t.RetentionCount = 3
	}
	out, err := scanBackupTarget(h.db.QueryRow(r.Context(), q, t.Name, t.Provider, t.Endpoint, t.Region, t.Bucket, t.Prefix, t.CredentialsSecret, t.ScheduleCron, t.RetentionDays, t.RetentionCount, t.EncryptionEnabled, t.Enabled, u.ID))
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	auditAction(r.Context(), h, r, "backup.target.upsert", out.Name, map[string]any{"provider": out.Provider, "bucket": out.Bucket, "enabled": out.Enabled})
	writeJSON(w, 200, out)
}

func backupS3Client(t backupTarget, accessKey, secretKey string) *s3.Client {
	return s3.New(s3.Options{
		Region:       t.Region,
		Credentials:  credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""),
		BaseEndpoint: aws.String(t.Endpoint),
		UsePathStyle: true,
	})
}

// TestBackupTarget kiểm tra Secret và quyền truy cập bucket S3 thật. Không ghi/xóa object.
func (h *Handler) TestBackupTarget(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	t, err := scanBackupTarget(h.db.QueryRow(r.Context(), `SELECT id,name,provider,endpoint,region,bucket,prefix,credentials_secret,schedule_cron,retention_days,retention_count,encryption_enabled,enabled,last_tested_at,last_test_error,created_at,updated_at FROM backup_targets WHERE id=$1`, id))
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "backup target không tồn tại"})
		return
	}
	if h.rancher == nil || !h.rancher.Enabled() {
		writeJSON(w, 503, map[string]string{"error": "Rancher chưa sẵn sàng để đọc credential secret"})
		return
	}
	data, found, err := h.rancher.GetOpaqueSecretData(r.Context(), "", backupSecretNamespace, t.CredentialsSecret)
	testErr := ""
	if err != nil {
		testErr = err.Error()
	} else if !found {
		testErr = "không tìm thấy Secret " + t.CredentialsSecret + " trong namespace platform"
	} else if strings.TrimSpace(data["access_key_id"]) == "" || strings.TrimSpace(data["secret_access_key"]) == "" {
		testErr = "Secret cần có access_key_id và secret_access_key"
	}
	if testErr == "" {
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		_, err = backupS3Client(t, data["access_key_id"], data["secret_access_key"]).HeadBucket(ctx, &s3.HeadBucketInput{
			Bucket: aws.String(t.Bucket),
		})
		if err != nil {
			testErr = "không truy cập được S3 bucket: " + err.Error()
		}
	}
	if testErr != "" {
		_, _ = h.db.Exec(r.Context(), `UPDATE backup_targets SET last_tested_at=now(),last_test_error=$2,updated_at=now() WHERE id=$1`, t.ID, testErr)
		writeJSON(w, 400, map[string]string{"error": testErr})
		return
	}
	_, _ = h.db.Exec(r.Context(), `UPDATE backup_targets SET last_tested_at=now(),last_test_error=NULL,updated_at=now() WHERE id=$1`, t.ID)
	auditAction(r.Context(), h, r, "backup.target.test", t.Name, map[string]any{"result": "s3_bucket_ready"})
	writeJSON(w, 200, map[string]any{"status": "s3_bucket_ready", "target": t.Name, "bucket": t.Bucket})
}

func scanBackupRun(row interface{ Scan(...any) error }) (backupRun, error) {
	var v backupRun
	err := row.Scan(&v.ID, &v.TargetID, &v.TargetName, &v.RunKind, &v.Status, &v.RunPrefix,
		&v.ManifestKey, &v.ArtifactCount, &v.TotalBytes, &v.ChecksumSHA256, &v.ErrorMessage,
		&v.StartedAt, &v.FinishedAt, &v.CreatedAt)
	return v, err
}

// ListBackupRuns GET /admin/backups/runs — history chỉ có metadata/checksum, không lộ credential.
func (h *Handler) ListBackupRuns(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(r.Context(), `
		SELECT r.id,r.target_id,t.name,r.run_kind,r.status,r.run_prefix,r.manifest_key,
		       r.artifact_count,r.total_bytes,r.checksum_sha256,r.error_message,r.started_at,r.finished_at,r.created_at
		FROM backup_runs r JOIN backup_targets t ON t.id=r.target_id
		ORDER BY r.created_at DESC LIMIT 100`)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	items := []backupRun{}
	for rows.Next() {
		if v, err := scanBackupRun(rows); err == nil {
			items = append(items, v)
		}
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

// PostBackupRun tạo run queued cho backup worker. Worker độc lập sẽ atomically chuyển running/success/failed.
// Không chạy command trong API pod để tránh đọc host path/etcd hoặc lộ credential qua HTTP process.
func (h *Handler) PostBackupRun(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	var body struct {
		TargetID int64 `json:"target_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.TargetID < 1 {
		writeJSON(w, 400, map[string]string{"error": "target_id hợp lệ là bắt buộc"})
		return
	}
	t, err := scanBackupTarget(h.db.QueryRow(r.Context(), `
		SELECT id,name,provider,endpoint,region,bucket,prefix,credentials_secret,schedule_cron,retention_days,retention_count,encryption_enabled,enabled,last_tested_at,last_test_error,created_at,updated_at
		FROM backup_targets WHERE id=$1`, body.TargetID))
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "backup target không tồn tại"})
		return
	}
	if !t.Enabled {
		writeJSON(w, 400, map[string]string{"error": "backup target đang tắt"})
		return
	}
	if t.LastTestedAt == nil || t.LastTestError != nil {
		writeJSON(w, 400, map[string]string{"error": "cần Test target thành công trước khi chạy backup"})
		return
	}
	prefix := fmt.Sprintf("%s/runs/%s", t.Prefix, time.Now().UTC().Format("20060102T150405Z"))
	var run backupRun
	err = h.db.QueryRow(r.Context(), `
		INSERT INTO backup_runs (target_id,run_kind,status,run_prefix,started_by)
		VALUES ($1,'manual','queued',$2,$3)
		RETURNING id,target_id,''::text,run_kind,status,run_prefix,manifest_key,artifact_count,total_bytes,checksum_sha256,error_message,started_at,finished_at,created_at`,
		t.ID, prefix, u.ID).Scan(&run.ID, &run.TargetID, &run.TargetName, &run.RunKind, &run.Status, &run.RunPrefix,
		&run.ManifestKey, &run.ArtifactCount, &run.TotalBytes, &run.ChecksumSHA256, &run.ErrorMessage, &run.StartedAt, &run.FinishedAt, &run.CreatedAt)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	run.TargetName = t.Name
	auditAction(r.Context(), h, r, "backup.run.queued", t.Name, map[string]any{"run_id": run.ID, "prefix": prefix})
	writeJSON(w, http.StatusAccepted, map[string]any{
		"run":     run,
		"message": "Backup đã xếp hàng. Backup worker sẽ xử lý PostgreSQL, etcd/config và MinIO.",
	})
}
