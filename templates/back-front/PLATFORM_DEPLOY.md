# Deploy trên 7mlabs Platform (api + web)

Hướng dẫn rút gọn — **đặt trong repo app** để team dev luôn thấy khi clone.

---

## Cấu trúc repo (bắt buộc)

```
backend/Dockerfile
frontend/Dockerfile
frontend/dist/index.html    # hoặc build SPA vào dist/
.platform/services.yaml     # layout multi: api + web
.platform/build.yaml        # VITE_API_BASE
```

---

## 4 bước Console

1. **Deploy / Git** — chọn repo + branch này · **Deploy env = dev**
2. **Web + API riêng** → **Áp dụng api + web từ repo** (nếu có banner)
3. **Lưu & đồng bộ GitHub** → badge **Workflow OK**
4. **Push** → đợi success → mở `https://{slug}-dev.../` và `/api/health`

---

## Env

| Biến | Giá trị |
|------|---------|
| `VITE_API_BASE` (build dev) | `/api` |
| `API_ROUTE_PREFIX` (runtime) | `/api` |
| `REDIS_URL` (runtime) | Platform inject khi bật **Addons → Redis** |
| `REDIS_KEY_TTL_SECONDS` (runtime) | TTL mặc định gợi ý khi SET key (tuỳ quota addon) |

Tab **Cấu hình app** trên Console.

### Redis (tuỳ chọn)

1. Console → **Addons** → bật Redis **dev**
2. App đọc `REDIS_URL` từ runtime env (secret `app-env`)
3. Thử: `GET /api/redis/ping` · `GET /api/redis/demo` (template backend)
4. Promote prod → checklist **Redis addon (prod)** → Platform provision instance prod riêng

Chi tiết: `docs/REDIS-ADDON.md` trên repo platform.

---

## Prod

Dev OK → tab **Promote Prod** → checklist xanh → Promote (cùng tag api + web).

---

## Đừng

- Push trước khi sync workflow
- Chọn **Một website** khi repo multi
- Dùng rollback để đổi single ↔ multi (dùng **Đổi kiểu chạy…**)

Chi tiết: doc **KHACH_DEPLOY** trên repo platform k8s hoặc nút **`?`** trên Console.
