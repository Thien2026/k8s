# MinIO addon — hướng dẫn developer

Platform inject object storage S3-compatible vào app qua **runtime env** (Secret `app-env`).

---

## Bật MinIO

1. Console → project → **Addons** → **MinIO** → env **dev** → **+ Thêm**
2. Sau provision: `S3_ENDPOINT`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`, `S3_BUCKET`, `S3_REGION`, `S3_USE_SSL`
3. Deploy/restart app để pod nhận env

Khi cluster **ha_capable** (Longhorn + ≥2 node), form enable cho chọn topology `standalone` | `distributed`.

---

## Topology

| Mode | Điều kiện | Mô tả |
|------|-----------|--------|
| `standalone` | Luôn | 1 pod + PVC (local-path / default SC) |
| `distributed` | `ha_capable` | 4 pods + headless + PVC **Longhorn**; erasure coding |

API trả `ha_capability` / `ha_capable` / `upgrade_available`.

**Không auto-migrate** object từ PVC standalone khi upgrade.

### Upgrade HA

1. Instance đang `standalone` + `ha_capable` → nút **Upgrade HA**
2. Confirm (danger): object trên PVC cũ **không** tự copy
3. API `POST /addons/minio/upgrade-ha` với `{ "confirm": true }`
4. Xóa STS cũ → provision distributed (giữ credentials auth secret)

PVC standalone cũ có thể còn orphan trên cluster (cứu dữ liệu tay nếu cần).

---

## Biến môi trường

| Biến | Mô tả |
|------|--------|
| `S3_ENDPOINT` | `http://{release}.{ns}.svc.cluster.local:9000` |
| `S3_ACCESS_KEY` / `S3_SECRET_KEY` | Service account riêng (chỉ read/write bucket `app`, KHÔNG có quyền admin) |
| `S3_BUCKET` | Mặc định `app` (Job `mc` tạo bucket) |
| `S3_REGION` | `us-east-1` |
| `S3_USE_SSL` | `false` (trong cluster) |
| `S3_MAX_OBJECT_MB` | Max 1 object (MiB) — app nên tự check; Console enforce cứng |

---

## Quota & bảo vệ platform

| Lớp | Ý nghĩa |
|-----|---------|
| **App key ≠ root** | App nhận service account riêng (inline policy: chỉ read/write bucket `app`). Root key chỉ ở tầng platform — app không tạo/xóa bucket, không đổi quota, không admin |
| **Storage GB** | PVC + `mc quota set --size` (hard) ≈ tổng data bucket |
| **Max object (MiB)** | Trần 1 file trên Quota project (≤ Platform Policy). Console chặn cứng; app nhận `S3_MAX_OBJECT_MB` |
| **Upload Console** | Trần riêng Files trên Console (thường nhỏ hơn max object) |
| **NetworkPolicy (prod)** | Chỉ pod app trong namespace gọi MinIO |

Hai trần Storage và Max object **độc lập**: còn 2GB trống mà max object = 5GB → upload 5GB vẫn fail vì hết chỗ.

**App key policy** (service account, gắn khi init job chạy):

```
s3:ListBucket, s3:GetBucketLocation, s3:ListBucketMultipartUploads   → arn:aws:s3:::app
s3:GetObject, s3:PutObject, s3:DeleteObject,
s3:AbortMultipartUpload, s3:ListMultipartUploadParts                 → arn:aws:s3:::app/*
```

Re-provision giữ nguyên app key cũ (trong connection secret) để app đang chạy không mất kết nối. Lần đầu migrate từ secret cũ (app key = root) sẽ tự sinh key mới + rollout app.

**Phase 2 còn lại (tùy chọn):** allowlist đuôi file / Content-Type trên IAM condition — chặn cứng loại file trên đường S3 trực tiếp (hiện `S3_MAX_OBJECT_MB` chỉ là gợi ý cho app).

---

## Dev vs prod

| | dev | prod |
|---|-----|------|
| Instance | Riêng | Riêng — **không copy object** |
| Service | **NodePort** + DNS `*.minio.{domain}` | Chỉ ClusterIP |
| External | `{slug}-minio-dev.minio.{domain}:{nodePort}` | Không expose |
| NetworkPolicy | Không | Chỉ pod app trong namespace |
| Promote | Checklist + auto provision prod nếu dev đã có MinIO | Topology theo ha_capable |

---

## Ops

- **Files** (Console): list / upload (≤10 MiB) / download / xóa object trong bucket mặc định `app`
- **Restart pod** / **Xem logs** trên trang addon (parity Redis)
- **Monitor**: Prometheus scrape `/minio/v2/metrics/cluster` (ServiceMonitor) + dashboard Grafana `platform-minio-addon`
- File/bucket nâng cao: dùng **MinIO Console** (link external trên trang addon)

Sau khi bật monitoring lần đầu: **Re-provision** MinIO để gắn `MINIO_PROMETHEUS_AUTH_TYPE=public` + ServiceMonitor, chờ ~1 phút.

---

## Code mẫu

```go
endpoint := os.Getenv("S3_ENDPOINT")
// dùng AWS SDK / minio-go với endpoint + path-style
```
