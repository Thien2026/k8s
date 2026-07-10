# Redis addon — hướng dẫn developer

Platform inject Redis vào app qua **runtime env** (Secret `app-env`). Không hardcode host/password trong repo.

---

## Bật Redis

1. Console → project → **Addons** → **Redis** → chọn env **dev** → **+ Thêm**
2. Sau provision: biến `REDIS_URL` (secret) và có thể `REDIS_KEY_TTL_SECONDS` xuất hiện tab **Cấu hình app**
3. Deploy/restart app để pod nhận env mới

---

## Biến môi trường

| Biến | Scope | Mô tả |
|------|-------|--------|
| `REDIS_URL` | runtime, secret | `redis://:password@{release}.{namespace}.svc.cluster.local:6379/0` |
| `REDIS_KEY_TTL_SECONDS` | runtime | TTL mặc định (giây) gợi ý khi app `SET` key — cấu hình ở Quota addon |

Khai báo tuỳ chọn trong `.platform/runtime.yaml`:

```yaml
version: 1
vars:
  REDIS_URL:
    required: false
  REDIS_KEY_TTL_SECONDS:
    required: false
```

---

## Dev vs prod

| | dev | prod |
|---|-----|------|
| Service Redis | NodePort (debug local) + ClusterIP | Chỉ ClusterIP |
| External URL | `*.redis.{domain}` (nếu DNS bật) | Không expose |
| NetworkPolicy | Không | Chỉ pod app (`app` label) trong namespace |
| Dữ liệu | Instance riêng | Instance riêng — **không copy** dev→prod |

**Promote app lên prod:** tab **Promote Prod** → checklist **Redis addon (prod)** → nút **Provision Redis prod** (hoặc tự provision khi bấm Promote nếu dev đã có Redis).

---

## Code mẫu (Go)

Template `templates/back-front/backend/` có sẵn:

- `GET /api/redis/ping` — `PING` → `PONG`
- `GET /api/redis/demo` — `SET console:hello` với TTL từ `REDIS_KEY_TTL_SECONDS`

```go
opt, _ := redis.ParseURL(os.Getenv("REDIS_URL"))
client := redis.NewClient(opt)
client.Set(ctx, "mykey", "val", time.Duration(ttlSec)*time.Second)
```

Prefix route: dùng `API_ROUTE_PREFIX` (mặc định `/api`).

---

## Key naming

- Key debug trên Console: prefix `console:` (Playground, Key browser)
- Production key nên có namespace riêng: `{service}:{entity}:{id}`

---

## Quota & eviction

Tab Redis → **Quota**: RAM, max clients, `maxmemory-policy`, TTL mặc định app.

- `allkeys-lru` — hết RAM xóa key ít dùng (kể cả không TTL)
- `volatile-lru` — chỉ xóa key có TTL
- `noeviction` — hết RAM từ chối ghi mới

Apply quota = re-provision Redis (restart pod, cập nhật env app).

---

## Monitor

Tab **Monitor**: INFO, slow log, link Grafana (`platform-redis-addon` dashboard).

Metrics chi tiết qua **redis_exporter** sidecar (sau re-provision).
