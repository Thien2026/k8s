# Deploy micro thường (Backend + Frontend)

Hướng dẫn **Phase 1** — monorepo **api + web** trong **một project Console**, một namespace mỗi môi trường.

> **Gửi khách / junior:** [KHACH_DEPLOY.md](./KHACH_DEPLOY.md) — 4 bước ngắn, đừng làm gì hay lỗi.

> **Chưa phải microservice phức tạp** (polyglot 5 service, GitOps, scale). Xem lộ trình đầy đủ ở cuối file.

---

## Kiến trúc

```
1 repo Git
├── backend/          → image harbor/.../api:sha
├── frontend/         → image harbor/.../web:sha
└── .platform/
    ├── services.yaml # Console đọc layout multi
    ├── build.yaml    # VITE_API_BASE=/api
    └── runtime.yaml  # (tùy chọn) API_ROUTE_PREFIX

Console project (vd. my-shop)
├── namespace dev:   my-shop-dev    → Deployment api + web + Ingress
└── namespace prod:  my-shop-prod   → cùng tag sau Promote
```

**Một domain:** `https://my-shop-dev.platform.example/` → web, `/api/...` → api.

---

## 4 bước trên Console (junior nhớ flow này)

> **Trên UI:** tab **Deploy / Git** hoặc **Promote Prod** → nút **`?`** (góc phải tiêu đề hoặc cạnh card Pipeline) → hướng dẫn tương tác ngay trong Console.

### Bước 1 — Nguồn GitHub

1. Tab **Deploy / Git** → chọn **Repository** + **Branch** (branch phải có `backend/` + `frontend/`).
2. **Deploy env:** để **dev** (push chỉ lên dev). Chọn **prod** chỉ khi chủ đích deploy thẳng production.
3. Bật **Tự deploy lên cluster khi build xong** (sau khi workflow OK).

### Bước 2 — Chốt kiểu chạy

1. Chọn **Web + API riêng** (multi).
2. Kiểm tra service **api** (`backend/`, ingress `/api`) và **web** (`frontend/`, ingress `/`).
3. Nếu repo có `.platform/services.yaml` → bấm **Áp dụng từ repo** hoặc để Console gợi ý.
4. **Đã deploy rồi mà muốn đổi single ↔ multi?** → **Đổi kiểu chạy…** (không dùng Deploy lại / rollback).

### Bước 3 — Lưu & đồng bộ GitHub (bắt buộc)

1. Bấm **Lưu & đồng bộ GitHub** — push workflow + `services.yaml` lên repo.
2. Badge phải **Workflow OK** (không còn **Cần đồng bộ**).
3. Chỉ **Chỉ lưu Console** khi nháp cấu hình — **chưa được push** cho đến khi sync.

### Bước 4 — Push & theo dõi

1. Push code lên branch đã chọn → GitHub Actions build **2 image** cùng tag SHA.
2. Console tab Deploy: 4 bước Build → Harbor → Cluster → Pod runtime.
3. Mở URL dev → web `/`, thử `/api/health`.

---

## Env bắt buộc (template back-front)

| Biến | Scope | Giá trị gợi ý |
|------|--------|----------------|
| `VITE_API_BASE` | Build (dev) | `/api` |
| `API_ROUTE_PREFIX` | Runtime (dev/prod) | `/api` (mặc đnh backend) |

Tab **Cấu hình app** → block **Build** / **Pod** → thêm nếu thiếu. Contract đọc từ `.platform/build.yaml` trên repo.

---

## Lên production

### Cách A — Promote (khuyến nghị)

1. Dev deploy **success** (multi, cùng tag api+web).
2. Tab **Promote Prod** → checklist xanh (workflow, env prod, domain…).
3. **Promote** — cùng image tag, không build lại.

### Cách B — Deploy thẳng prod

1. Deploy / Git → **Deploy env: prod** → **Lưu & đồng bộ GitHub**.
2. Mỗi push lên branch = build + deploy prod. **Cẩn thận.**

---

## Lỗi thường gặp

| Triệu chứng | Nguyên nhân | Cách xử lý |
|-------------|-------------|------------|
| 503, chỉ có Ingress | Fleet cũ bị xóa / layout lệch | **Đổi kiểu chạy** + sync + deploy mới |
| Build multi nhưng Console single | Chưa sync sau đổi layout | **Lưu & đồng bộ GitHub** |
| Badge **Cần đồng bộ** | Workflow lệch Console | Bước 3 lại |
| Deploy lại bản multi khi đang single | Rollback chỉ cùng kiểu | **Đổi kiểu chạy…** |
| Frontend gọi API sai | Thiếu `VITE_API_BASE=/api` | Cấu hình app → Build |

---

## Copy template vào repo mới

```bash
# Từ repo platform k8s
./scripts/setup-back-front-pilot.sh /path/to/your-repo-clone
```

Hoặc thủ công:

```bash
cp -R templates/back-front/backend templates/back-front/frontend templates/back-front/.platform /path/to/repo/
git add backend frontend .platform
git commit -m "chore: add back-front platform template"
git push
```

Chi tiết pilot: [templates/back-front/DEPLOY.md](../templates/back-front/DEPLOY.md)

Kiểm tra sau deploy:

```bash
./scripts/verify-back-front-deploy.sh my-shop dev
```

---

## Lộ trình tiếp (sau Phase 1)

| Phase | Nội dung |
|-------|----------|
| **1** ✅ | Micro thường api+web, workflow profile, doc |
| **2** | Polyglot fleet (node, python, dotnet…) — dev pilot |
| **3** | Build theo path (chỉ service đổi) |
| **4** | ArgoCD GitOps, HPA/KEDA |
