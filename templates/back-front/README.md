# Template: Backend + Frontend (7mlabs Platform)

Monorepo chuẩn **micro thường (Phase 1)** — **1 domain**, web `/`, API `/api`, **2 image** (api + web).

## Cấu trúc

```
backend/          # API Go — route dưới /api
frontend/         # SPA nginx — gọi VITE_API_BASE (/api)
.platform/
  services.yaml   # layout: multi — Console tự nhận api + web
  build.yaml      # VITE_API_BASE required
  runtime.yaml    # API_ROUTE_PREFIX (tùy chọn)
```

## Dev local

```bash
# Terminal 1 — backend :8080
cd backend && go run .

# Terminal 2 — mở frontend/dist/index.html hoặc serve static
# (prod dùng Docker; local có thể proxy qua vite — xem vite.config.js)
```

Frontend **không** hardcode `localhost:8080` — prod dùng `/api` trên cùng domain.

## Deploy trên Platform (4 bước)

1. **Deploy / Git** — chọn repo + branch, **Deploy env = dev**
2. **Web + API riêng** — api `backend/`, web `frontend/`
3. **Lưu & đồng bộ GitHub** — bắt buộc (workflow + services.yaml)
4. **Push** → theo dõi deploy; kiểm tra `/` và `/api/health`

Chi tiết: [DEPLOY.md](./DEPLOY.md) · [docs/MICRO_DEPLOY.md](../../docs/MICRO_DEPLOY.md)

## Env

| Biến | Scope | Giá trị |
|------|--------|---------|
| `VITE_API_BASE` | Build | `/api` |
| `API_ROUTE_PREFIX` | Runtime | `/api` (mặc định) |

## Copy vào repo

```bash
# Từ repo k8s
./scripts/setup-back-front-pilot.sh /path/to/your-repo-clone
git -C /path/to/your-repo-clone push -u origin back-front-pilot
```

Hoặc thủ công:

```bash
cp -R templates/back-front/backend templates/back-front/frontend templates/back-front/.platform /path/to/your-repo/
```
