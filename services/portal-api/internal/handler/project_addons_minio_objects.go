package handler

import (
	"context"
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
	cfg, err := h.projectMinioS3Creds(r.Context(), p.ID, env)
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

func (h *Handler) effectiveMinioConsoleUploadMB(ctx context.Context, projectID int64, env string, pol platformPolicy) int {
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
	maxUploadMB := h.effectiveMinioConsoleUploadMB(r.Context(), p.ID, env, pol)
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
	cfg, err := h.projectMinioS3Creds(r.Context(), p.ID, env)
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
	cfg, err := h.projectMinioS3Creds(r.Context(), p.ID, env)
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
	cfg, err := h.projectMinioS3Creds(r.Context(), p.ID, env)
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
