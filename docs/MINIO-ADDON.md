# MinIO addon — hướng dẫn developer

Platform inject object storage S3-compatible vào app qua **runtime env** (Secret `app-env`).

---

## Bật MinIO

1. Console → project → **Addons** → **MinIO** → env **dev** → **+ Thêm**
2. Sau provision: `S3_ENDPOINT`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`, `S3_BUCKET`, `S3_REGION`, `S3_USE_SSL`
3. Deploy/restart app để pod nhận env

---

## Topology

| Mode | Trạng thái |
|------|------------|
| `standalone` | **MVP hiện tại** — 1 pod + 1 PVC |
| `distributed` | Phase sau — khi `ha_capable` (Longhorn + ≥2 node) + upgrade có xác nhận |

API trả `ha_capability` / `ha_capable`. **Không** auto-migrate instance đang chạy.

---

## Biến môi trường

| Biến | Mô tả |
|------|--------|
| `S3_ENDPOINT` | `http://{release}.{ns}.svc.cluster.local:9000` |
| `S3_ACCESS_KEY` / `S3_SECRET_KEY` | Credentials (secret) |
| `S3_BUCKET` | Mặc định `app` (Job `mc` tạo bucket) |
| `S3_REGION` | `us-east-1` |
| `S3_USE_SSL` | `false` (trong cluster) |

---

## Dev vs prod

| | dev | prod |
|---|-----|------|
| Instance | Riêng | Riêng — **không copy object** |
| Service | **NodePort** + DNS `*.minio.{domain}` | Chỉ ClusterIP |
| External | `{slug}-minio-dev.minio.{domain}:{nodePort}` | Không expose |
| NetworkPolicy | Không | Chỉ pod app trong namespace |
| Promote | Checklist + auto provision prod nếu dev đã có MinIO | |

Ví dụ local (DNS only Cloudflare):

```text
http://test-harbor-minio-dev.minio.7mlabs.com:3xxxx
```

SDK cần **path-style** (`forcePathStyle: true`). `S3_ENDPOINT` trong pod vẫn là URL cluster nội bộ.

---

## Code mẫu

```go
endpoint := os.Getenv("S3_ENDPOINT")
// dùng AWS SDK / minio-go với endpoint + path-style
```
