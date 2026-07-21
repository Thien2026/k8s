package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/Thien2026/k8s/services/portal-api/internal/auth"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/go-chi/chi/v5"
)

const (
	minioObjectsMaxList   = 200
	minioObjectsMaxUpload = 10 << 20 // 10 MiB
)

type minioS3Creds struct {
	Endpoint       string
	AccessKey      string
	SecretKey      string
	Bucket         string
	Region         string
	ForcePathStyle bool
}

func newMinioScanObjectID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (h *Handler) minioScanProfileCreds(ctx context.Context, p projectRow, env, bucket string) (minioS3Creds, int64, error) {
	var bucketID int64
	var secretName, status string
	err := h.db.QueryRow(ctx, `SELECT b.id,sp.uploader_connection_secret,sp.status
		FROM project_minio_buckets b JOIN project_minio_scan_profiles sp ON sp.bucket_id=b.id
		WHERE b.project_id=$1 AND b.environment=$2 AND b.name=$3 AND b.scan_mode='required'`,
		p.ID, env, bucket).Scan(&bucketID, &secretName, &status)
	if err != nil || status != "ready" {
		return minioS3Creds{}, 0, fmt.Errorf("scan profile bucket chưa sẵn sàng")
	}
	data, found, err := h.rancher.GetOpaqueSecretData(ctx, "", h.projectNamespace(p, env), secretName)
	if err != nil || !found {
		return minioS3Creds{}, 0, fmt.Errorf("uploader credential scan chưa sẵn sàng")
	}
	return minioS3Creds{
		Endpoint: strings.TrimSpace(data["S3_ENDPOINT"]), AccessKey: strings.TrimSpace(data["S3_ACCESS_KEY"]),
		SecretKey: strings.TrimSpace(data["S3_SECRET_KEY"]), Bucket: strings.TrimSpace(data["S3_BUCKET"]),
		Region: strings.TrimSpace(data["S3_REGION"]), ForcePathStyle: true,
	}, bucketID, nil
}

func (h *Handler) projectMinioS3Creds(ctx context.Context, projectID int64, env string) (minioS3Creds, error) {
	vars, err := h.envVarsMap(ctx, projectID, env)
	if err != nil {
		return minioS3Creds{}, err
	}
	cfg := minioS3Creds{
		Endpoint:       strings.TrimSpace(vars["S3_ENDPOINT"]),
		AccessKey:      strings.TrimSpace(vars["S3_ACCESS_KEY"]),
		SecretKey:      strings.TrimSpace(vars["S3_SECRET_KEY"]),
		Bucket:         strings.TrimSpace(vars["S3_BUCKET"]),
		Region:         strings.TrimSpace(vars["S3_REGION"]),
		ForcePathStyle: !strings.EqualFold(strings.TrimSpace(vars["S3_FORCE_PATH_STYLE"]), "false"),
	}
	if cfg.Bucket == "" {
		cfg.Bucket = minioDefaultBucket
	}
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	if cfg.Endpoint == "" || cfg.AccessKey == "" || cfg.SecretKey == "" {
		return minioS3Creds{}, fmt.Errorf("S3 chưa sẵn sàng — bật MinIO addon trước")
	}
	return cfg, nil
}

// projectMinioBucketS3Creds resolves only a DB-owned bucket, never a raw client bucket name.
// The default `app` bucket continues using legacy runtime S3_* for compatibility.
func (h *Handler) projectMinioBucketS3Creds(ctx context.Context, p projectRow, env, bucket string) (minioS3Creds, error) {
	if bucket == "" || bucket == minioDefaultBucket {
		return h.projectMinioS3Creds(ctx, p.ID, env)
	}
	name, err := normalizeMinioBucketName(bucket)
	if err != nil {
		return minioS3Creds{}, err
	}
	var secretName, status string
	if err := h.db.QueryRow(ctx, `SELECT connection_secret,status FROM project_minio_buckets
		WHERE project_id=$1 AND environment=$2 AND name=$3`, p.ID, env, name).Scan(&secretName, &status); err != nil {
		return minioS3Creds{}, fmt.Errorf("bucket không tồn tại")
	}
	if status != "running" {
		return minioS3Creds{}, fmt.Errorf("bucket chưa sẵn sàng")
	}
	data, found, err := h.rancher.GetOpaqueSecretData(ctx, "", h.projectNamespace(p, env), secretName)
	if err != nil || !found {
		return minioS3Creds{}, fmt.Errorf("credential bucket chưa sẵn sàng")
	}
	cfg := minioS3Creds{
		Endpoint: strings.TrimSpace(data["S3_ENDPOINT"]), AccessKey: strings.TrimSpace(data["S3_ACCESS_KEY"]),
		SecretKey: strings.TrimSpace(data["S3_SECRET_KEY"]), Bucket: strings.TrimSpace(data["S3_BUCKET"]),
		Region: strings.TrimSpace(data["S3_REGION"]), ForcePathStyle: !strings.EqualFold(strings.TrimSpace(data["S3_FORCE_PATH_STYLE"]), "false"),
	}
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	if cfg.Endpoint == "" || cfg.AccessKey == "" || cfg.SecretKey == "" || cfg.Bucket != name {
		return minioS3Creds{}, fmt.Errorf("credential bucket không hợp lệ")
	}
	return cfg, nil
}

func (h *Handler) minioS3Client(cfg minioS3Creds) *s3.Client {
	endpoint := strings.TrimRight(cfg.Endpoint, "/")
	return s3.New(s3.Options{
		Region: cfg.Region,
		Credentials: credentials.NewStaticCredentialsProvider(
			cfg.AccessKey, cfg.SecretKey, "",
		),
		BaseEndpoint: aws.String(endpoint),
		UsePathStyle: cfg.ForcePathStyle,
	})
}

func normalizeMinioObjectKey(raw string) (string, error) {
	key := strings.TrimSpace(raw)
	key = strings.TrimPrefix(key, "/")
	if key == "" {
		return "", fmt.Errorf("thiếu key")
	}
	if len(key) > 512 {
		return "", fmt.Errorf("key tối đa 512 ký tự")
	}
	if strings.Contains(key, "..") || strings.ContainsAny(key, "\x00\n\r") {
		return "", fmt.Errorf("key không hợp lệ")
	}
	return key, nil
}

func normalizeMinioObjectPrefix(raw string) string {
	p := strings.TrimSpace(raw)
	p = strings.TrimPrefix(p, "/")
	if strings.Contains(p, "..") {
		return ""
	}
	if len(p) > 200 {
		p = p[:200]
	}
	return p
}

// GetMinioAddonObjects GET /projects/{slug}/addons/minio/objects
func (h *Handler) GetMinioAddonObjects(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	p, ok := h.requireProjectAccess(w, r, slug)
	if !ok {
		return
	}
	env := strings.TrimSpace(r.URL.Query().Get("environment"))
	if env == "" {
		env = "dev"
	}
	if !validAddonEnv(env) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "environment phải là dev hoặc prod"})
		return
	}
	cfg, err := h.projectMinioBucketS3Creds(r.Context(), p, env, strings.TrimSpace(r.URL.Query().Get("bucket")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	limit := 50
	if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
		if n, e := strconv.Atoi(v); e == nil && n > 0 && n <= minioObjectsMaxList {
			limit = n
		}
	}
	prefix := normalizeMinioObjectPrefix(r.URL.Query().Get("prefix"))
	client := h.minioS3Client(cfg)
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	out, err := client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket:  aws.String(cfg.Bucket),
		Prefix:  aws.String(prefix),
		MaxKeys: aws.Int32(int32(limit)),
	})
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "list objects: " + err.Error()})
		return
	}
	items := make([]map[string]any, 0, len(out.Contents))
	for _, obj := range out.Contents {
		items = append(items, map[string]any{
			"key":           aws.ToString(obj.Key),
			"size":          aws.ToInt64(obj.Size),
			"last_modified": obj.LastModified,
			"etag":          strings.Trim(aws.ToString(obj.ETag), `"`),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"bucket":       cfg.Bucket,
		"prefix":       prefix,
		"items":        items,
		"is_truncated": aws.ToBool(out.IsTruncated),
		"count":        len(items),
	})
}

func (h *Handler) effectiveMinioConsoleUploadMB(ctx context.Context, projectID int64, env, bucket string, pol platformPolicy) int {
	maxUploadMB := pol.MinioConsoleUploadMB
	if maxUploadMB < 1 {
		maxUploadMB = int(minioObjectsMaxUpload >> 20)
	}
	if addon, err := h.getProjectAddon(ctx, projectID, "minio", env); err == nil && addon != nil {
		objMB := normalizeMinioMaxObjectMB(addon.MaxObjectMB)
		if objMB < maxUploadMB {
			maxUploadMB = objMB
		}
	}
	if bucket != "" && bucket != minioDefaultBucket {
		var bucketMax int
		if err := h.db.QueryRow(ctx, `SELECT max_object_mb FROM project_minio_buckets WHERE project_id=$1 AND environment=$2 AND name=$3 AND status='running'`, projectID, env, bucket).Scan(&bucketMax); err == nil && bucketMax > 0 && bucketMax < maxUploadMB {
			maxUploadMB = bucketMax
		}
	}
	if pol.MinioMaxObjectMB > 0 && pol.MinioMaxObjectMB < maxUploadMB {
		maxUploadMB = pol.MinioMaxObjectMB
	}
	if maxUploadMB < 1 {
		maxUploadMB = 1
	}
	return maxUploadMB
}

// UploadMinioAddonObject POST /projects/{slug}/addons/minio/objects (multipart: file, key?, environment?)
func (h *Handler) UploadMinioAddonObject(w http.ResponseWriter, r *http.Request) {
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
	pol, _, _ := h.loadPlatformPolicy(r.Context())
	parseCeilMB := pol.MinioConsoleUploadMB
	if parseCeilMB < 1 {
		parseCeilMB = int(minioObjectsMaxUpload >> 20)
	}
	parseCeil := int64(parseCeilMB) << 20
	r.Body = http.MaxBytesReader(w, r.Body, parseCeil+1<<20)
	if err := r.ParseMultipartForm(parseCeil + 1<<20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("file quá lớn (tối đa %d MiB) hoặc form không hợp lệ", parseCeilMB),
		})
		return
	}
	env := strings.TrimSpace(r.FormValue("environment"))
	if env == "" {
		env = "dev"
	}
	if !validAddonEnv(env) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "environment phải là dev hoặc prod"})
		return
	}
	bucket := strings.TrimSpace(r.FormValue("bucket"))
	maxUploadMB := h.effectiveMinioConsoleUploadMB(r.Context(), p.ID, env, bucket, pol)
	maxUpload := int64(maxUploadMB) << 20
	file, hdr, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "thiếu file upload"})
		return
	}
	defer file.Close()
	if hdr.Size > maxUpload {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("file tối đa %d MiB (quota / platform policy)", maxUploadMB),
		})
		return
	}
	key := strings.TrimSpace(r.FormValue("key"))
	if key == "" {
		key = path.Base(hdr.Filename)
	}
	key, err = normalizeMinioObjectKey(key)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	cleanBucket := bucket
	if cleanBucket == "" {
		cleanBucket = minioDefaultBucket
	}
	var scanMode string
	_ = h.db.QueryRow(r.Context(), `SELECT scan_mode FROM project_minio_buckets WHERE project_id=$1 AND environment=$2 AND name=$3`,
		p.ID, env, cleanBucket).Scan(&scanMode)
	if scanMode == "required" {
		cfg, bucketID, err := h.minioScanProfileCreds(r.Context(), p, env, cleanBucket)
		if err != nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		scanID, err := newMinioScanObjectID()
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "không tạo được scan id"})
			return
		}
		stagedKey := "__quarantine/" + scanID + "/" + key
		client := h.minioS3Client(cfg)
		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()
		contentType := hdr.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		if _, err := client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(cfg.Bucket), Key: aws.String(stagedKey), Body: file, ContentType: aws.String(contentType)}); err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "stage quarantine: " + err.Error()})
			return
		}
		_, err = h.db.Exec(r.Context(), `INSERT INTO project_minio_object_scans
			(id,project_id,environment,bucket_id,quarantine_key,destination_key,object_size,content_type,status,requested_by)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'queued',$9)`,
			scanID, p.ID, env, bucketID, stagedKey, key, hdr.Size, contentType, u.ID)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "lưu scan state: " + err.Error()})
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "scan_id": scanID, "status": "queued", "bucket": cleanBucket, "key": key, "size": hdr.Size})
		return
	}
	cfg, err := h.projectMinioBucketS3Creds(r.Context(), p, env, bucket)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	client := h.minioS3Client(cfg)
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	contentType := hdr.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	_, err = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(cfg.Bucket),
		Key:         aws.String(key),
		Body:        file,
		ContentType: aws.String(contentType),
	})
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "upload: " + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"bucket": cfg.Bucket,
		"key":    key,
		"size":   hdr.Size,
	})
}

// DeleteMinioAddonObject DELETE /projects/{slug}/addons/minio/objects?key=&environment=
func (h *Handler) DeleteMinioAddonObject(w http.ResponseWriter, r *http.Request) {
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
	env := strings.TrimSpace(r.URL.Query().Get("environment"))
	if env == "" {
		env = "dev"
	}
	if !validAddonEnv(env) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "environment phải là dev hoặc prod"})
		return
	}
	key, err := normalizeMinioObjectKey(r.URL.Query().Get("key"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	cfg, err := h.projectMinioBucketS3Creds(r.Context(), p, env, strings.TrimSpace(r.URL.Query().Get("bucket")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	client := h.minioS3Client(cfg)
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	_, err = client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(cfg.Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "delete: " + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "key": key, "bucket": cfg.Bucket})
}

// DownloadMinioAddonObject GET /projects/{slug}/addons/minio/objects/download?key=&environment=
func (h *Handler) DownloadMinioAddonObject(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	p, ok := h.requireProjectAccess(w, r, slug)
	if !ok {
		return
	}
	env := strings.TrimSpace(r.URL.Query().Get("environment"))
	if env == "" {
		env = "dev"
	}
	if !validAddonEnv(env) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "environment phải là dev hoặc prod"})
		return
	}
	key, err := normalizeMinioObjectKey(r.URL.Query().Get("key"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	cfg, err := h.projectMinioBucketS3Creds(r.Context(), p, env, strings.TrimSpace(r.URL.Query().Get("bucket")))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	client := h.minioS3Client(cfg)
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	out, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(cfg.Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "download: " + err.Error()})
		return
	}
	defer out.Body.Close()
	ct := aws.ToString(out.ContentType)
	if ct == "" {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, path.Base(key)))
	if out.ContentLength != nil && *out.ContentLength > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(*out.ContentLength, 10))
	}
	_, _ = io.Copy(w, out.Body)
}
