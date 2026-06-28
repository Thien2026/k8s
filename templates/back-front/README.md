# Template: Backend + Frontend (7mlabs Platform)

Monorepo chuẩn platform — **1 domain**, web `/`, API `/api`.

## Cấu trúc

```
backend/          # API — route dưới /api
frontend/         # SPA — gọi VITE_API_BASE (/api)
.platform/
  build.yaml       # biến build (Console điền value)
  runtime.yaml     # biến runtime pod
  services.yaml    # fleet layout (L4A — Console tự nhận api + web)
```

## Dev local

```bash
# Terminal 1 — backend :8080
cd backend && go run .

# Terminal 2 — frontend :5173, proxy /api → backend
cd frontend && npm run dev
```

Frontend **không** hardcode `localhost:8080` — dùng `/api` + proxy trong `vite.config`.

## Prod (platform)

- Layout: **Backend + Frontend**
- Console **Cấu hình app → Build (dev)**: `VITE_API_BASE=/api`
- URL: `https://{project}-dev.{domain}/` và `.../api/...`

## Copy vào repo mới

```bash
cp -R templates/back-front/backend templates/back-front/frontend templates/back-front/.platform /path/to/your-repo/
```

Sửa code mẫu theo stack của bạn, giữ contract env và prefix `/api`.
