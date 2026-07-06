# Phase 2 — Polyglot fleet (nhiều service / nhiều stack)

Monorepo **3+ service** — api, web, worker, node, dotnet… Mỗi service một image, build Docker hoặc Buildpack theo stack.

> **Phase 1** (api+web thuần): [KHACH_DEPLOY.md](./KHACH_DEPLOY.md)  
> **Pilot E2E:** `./scripts/polyglot-e2e.sh` trên VPS

---

## Kiến trúc

```
1 repo
├── backend/          → api (Go, dockerfile)
├── frontend/         → web (nginx)
├── backend-node/     → node (buildpack)
├── worker/           → worker (python buildpack)
└── .platform/
    └── services.yaml # layout: multi, N service + stack
```

Console → namespace `{slug}-dev` → N Deployment + Ingress (chỉ service public).

---

## 4 bước Console (giống Phase 1, khác ở bước 2)

1. **Deploy / Git** — repo + branch (vd. `multi-polyglot-full`) · **Deploy env = dev**
2. **Web + API riêng** (multi) → **「Áp dụng fleet từ repo」** — đọc `services.yaml`
3. **Lưu & đồng bộ GitHub** → Workflow OK
4. **Push** → CI build từng service · path-filter: chỉ build service đổi (retag service còn lại)

---

## services.yaml mẫu

```yaml
version: 1
layout: multi
services:
  - name: api
    path: backend
    ingress: /api
    health: /health
    build_mode: dockerfile
  - name: web
    path: frontend
    ingress: /
    health: /
  - name: worker
    path: worker
    expose: false
    stack: python
    build_mode: buildpack
  - name: node
    path: backend-node
    expose: false
    stack: node
    build_mode: buildpack
```

---

## Stack Buildpack

| Stack | Marker | Ghi chú |
|-------|--------|---------|
| python | requirements.txt, pyproject.toml | worker, cron |
| node | package.json | sidecar API |
| go | go.mod | có thể dockerfile thay buildpack |
| dotnet | *.csproj | backend-dotnet |

Console tự quét GitHub khi **Áp dụng từ repo** hoặc **Lưu & sync**.

---

## Path-filter (Phase 2B)

Push chỉ sửa `frontend/` → CI **retag** api image cũ sang SHA mới + **build** web.

Trigger full rebuild: sửa `.platform/**` hoặc workflow.

---

## Kiểm tra dev

```bash
./scripts/verify-polyglot-deploy.sh my-fleet dev
```

Kỳ vọng: pod mọi service **Running**, URL public (`/`, `/api/health`) → 200.

---

## Submodule (L4C)

Contract:

```yaml
git:
  submodules: recursive
```

Script pilot: `./scripts/setup-test-k8s-submodules-pilot.sh /path/to/test-k8s`

---

## Không thuộc Phase 2

- **ArgoCD GitOps** — Phase 4 (`templates/gitops/`, addon bootstrap)
- **Promote prod** — khi dev ổn (admin)

---

## Lỗi thường gặp

| Triệu chứng | Xử lý |
|-------------|--------|
| Pod api 404 probe | `health` trong services.yaml + ingress prefix (`/api/health`) |
| Chỉ build 1 image | Layout single — chọn multi + sync |
| Buildpack fail | Kiểm marker file (package.json, requirements.txt) |
| Deploy thiếu image | Path-filter lần đầu trên branch — push lại hoặc workflow_dispatch |
