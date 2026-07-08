# Kế hoạch refactor Console — trước Phase 10

> **Trạng thái:** Refactor code-split **R1 + B1–B3 gần xong** (2026-07-08). Còn smoke tay trước deploy VPS; Phase 10a (Redis Helm) có thể mở sau smoke.  
> Checklist tick → [TASKS.md](../TASKS.md#refactor--console-code-health-trước-phase-10).

---

## Vì sao làm ngay?

| Vấn đề | Hậu quả nếu không sửa |
|--------|------------------------|
| `app.js` ~9.000 dòng | Khách/DevOps không onboard, AI review chậm, conflict git |
| `style.css` ~4.700 dòng | Khó tách theme / component khi thêm Addons, Postgres UI |
| `deployments.go` ~1.900 dòng | Mọi thay đổi deploy/runtime đụng file khổng lồ |
| Build concat 1 HTML ~450 KB | Parse JS lúc load; không cache được JS |
| `STACK.md` ghi React nhưng code là vanilla JS | Handover sai kỳ vọng |

**Không** rewrite sang React — refactor **module vanilla + Vite**, giữ hành vi hiện tại.

---

## Audit file lớn (ngưỡng cảnh báo)

### Frontend (`services/portal-web`)

| File | Dòng | Gắn phase / feature | Ưu tiên tách |
|------|------|---------------------|--------------|
| **`src/app.js`** | **~20** (bootstrap) | init + hashchange | ✅ R1 |
| `ui_deploy_*.js` / `ui_pipeline_layout.js` | mỗi file ≤1.2k | Phase 4 deploy | ✅ đã tách |
| `src/style.css` | ~4.695 | Global CSS | **P1** |
| `src/deploy_help.js` | 281 | Phase 4 Deploy | OK (giữ) |
| `src/k8s_ops_help.js` | 163 | Phase 9 K8s ops | OK (giữ) |

**`app.js` — ước lượng theo feature (dòng):**

| Khối | Dòng (approx) | Phase |
|------|---------------|-------|
| Dialog, toast, API, auth | 1–400 | Core |
| Deploy pipeline UI + poll | 400–2.100 | Phase 4 |
| Project wizard / platform projects | 2.100–3.200 | Phase 3 |
| GitOps, registry | 3.200–3.500 | Phase 4 |
| **Project hub `pageProjectHub`** | **5.688–6.750** | 1c + 4 + 8 + 9 + 10a |
| → tab `deploy` | 6.007–6.315 | Phase 4 |
| → tab `env` | 6.315–6.459 | Phase 4 |
| → tab `domains` | 6.459–6.554 | Phase 3 |
| → tab `settings` | 6.554–6.747 | Phase 3 |
| Platform overview, infra | 6.747–7.420 | Phase 2, 8 |
| K8s list + detail | 7.418–8.370 | Phase 2, 9 |
| **K8s ops terminal** | **7.695–8.050** | Phase 9 |
| Sidebar, router, navigate | 8.320–8.980 | Core |

### Backend (`services/portal-api`)

| File | Dòng | Gắn phase | Ưu tiên tách |
|------|------|-----------|--------------|
| **`handler/deployments.go`** | **~1.901** | Phase 4 deploy activity | **P0** |
| `handler/projects.go` | ~903 | Phase 1c, 3 | P1 |
| `handler/deploy.go` | ~887 | Phase 4 apply/promote | P1 |
| `rancher/k8s.go` | ~867 | Phase 2 K8s proxy | P1 |
| `handler/runtime_health.go` | ~498 | Phase 4 runtime | P2 (đã tách một phần) |
| `handler/build_config.go` | ~482 | Phase 4 env contract | P2 |
| `handler/deploy_snapshot.go` | ~481 | Phase 4 | P2 |
| `handler/envvars.go` | ~432 | Phase 4 | P2 |
| `handler/fleet_runtime.go` | ~379 | Phase 4 multi-service | P2 |
| `handler/github.go` + `github_project.go` | ~700 | Phase 4 GitHub | P2 |
| Các file còn lại | &lt;350 | — | Ổn |

**Handler tổng:** ~40 file — cấu trúc Go **ổn hơn frontend**; điểm nóng là `deployments.go` + cụm deploy.

---

## Nguyên tắc refactor

1. **Không đổi hành vi** — mỗi bước ship được, `npm run build` / `./build.sh` / test pass.
2. **Không nhảy React** — ES modules + Vite; TypeScript tuỳ chọn sau (R3).
3. **Freeze file God** — không thêm feature lớn vào `app.js` / `deployments.go` (chỉ hotfix).
4. **Một PR / milestone = một mảng** — dễ review, dễ rollback.
5. **Sửa `STACK.md`** — ghi đúng “vanilla JS + Vite modules”, không React (trừ khi quyết định đổi sau).

---

## Frontend — lộ trình

### R0 — Freeze & chuẩn bị (0.5 ngày)

- [ ] Ghi nhận trong TASKS: Phase 10a **paused**
- [ ] Sửa `STACK.md` mô tả frontend thực tế
- [ ] Quy ước thư mục `src/` (bên dưới)
- [ ] `build.sh` gọi `vite build` song song concat cũ (hoặc thay hẳn sau R2)

### R1 — Tách module, vẫn ship (2–3 ngày) ⭐

**Mục tiêu:** Không còn file &gt;1.500 dòng; logic tách theo domain.

```
services/portal-web/src/
  core/
    api.js          # fetch, auth header, qs
    state.js        # state, projectEnv
    router.js       # parseRoute, navigate, routes
    format.js       # esc, fmtTime, badges
  ui/
    dialog.js       # openDialog, uiConfirm, uiAlert
    table.js        # renderTable, statBox
    sidebar.js      # buildPlatform/Project/Addons sidebar
    toast.js
  pages/
    platform/       # overview, addons admin, gitops, users
    project/
      hub.js        # pageProjectHub switch (mỏng)
      overview.js
      monitoring.js
      runtime.js
      deploy.js     # tab deploy + poll
      env.js
      domains.js
      settings.js
      addons.js
    k8s/
      list.js
      detail.js
      ops.js        # terminal Phase 9
  app.js            # &lt;150 dòng: init, login, hashchange
  deploy_help.js
  k8s_ops_help.js
  style.css         # chưa tách — R2
```

**Thứ tự tách (ít rủi ro → nhiều phụ thuộc):**

1. `core/api.js`, `core/format.js`, `ui/dialog.js`
2. `ui/sidebar.js`, `core/router.js`
3. `pages/k8s/ops.js`
4. `pages/project/runtime.js`, `addons.js`
5. `pages/project/deploy.js` (lớn nhất — cuối)

**Xong R1 khi:** `app.js` &lt;200 dòng; không file JS page &gt;1.200 dòng; `./build.sh` OK; smoke manual các tab project chính.

### R2 — Vite production + CSS (1–2 ngày)

- [ ] `vite build` → `dist/` (HTML + hashed JS hoặc inline tùy Docker/nginx)
- [ ] Cập nhật `Dockerfile` / `bootstrap/run.sh 08` nếu đổi output
- [ ] Tách `style.css` → `styles/base.css`, `styles/project.css`, `styles/deploy.css`, `styles/k8s-terminal.css`
- [ ] Minify; đo lại kích thước first load

**Xong R2 khi:** Deploy VPS giống hành vi cũ; first load ≤ hoặc nhỏ hơn 450 KB tương đương.

### R3 — Lazy route (1 ngày, tuỳ chọn sau R2)

- [ ] `import()` theo `parseRoute().tab` — vào Deploy mới tải `deploy.js`
- [ ] Giảm parse time tab Tổng quan / Monitoring

### R4 — TypeScript (sau Phase 10b, không block)

- Chuyển dần `core/*.ts`; giữ JS pages được.

---

## Backend — lộ trình

### B0 — Freeze (song song R0)

- [ ] Không thêm handler mới vào `deployments.go`
- [ ] Addon/Postgres API mới → file riêng từ đầu (`project_addons.go` ✅)

### B1 — Tách `deployments.go` (1–2 ngày) ⭐

```
handler/
  deployments.go            # index stub
  deployment_model.go       # deploymentRow, stages
  deployment_pick.go        # pickCurrent, dedupe, seal
  deployment_store.go       # upsert / mark phases
  deployment_runtime.go     # enrichRuntime, pods, logs
  deployment_list.go        # listProjectDeployments + attach GH run
  deployment_github.go      # merge GH, build live, Harbor scan
  deployment_activity.go    # GetProjectDeployActivity
  deployment_hooks.go       # DeployEventHook
```

- [x] Tách xong; test pass; không file `deployment_*.go` &gt;600 dòng

**Xong B1 khi:** không file handler deployment God-file; `go test ./internal/handler/...` pass. ✅

### B2 — Tách cụm deploy (1 ngày)

- [ ] `deploy.go` → giữ orchestration; move promote-readiness, smoke gate nếu còn dính
- [ ] `projects.go` → tách `project_members.go`, `project_overview.go` (overview API đã nặng)

### B3 — `rancher/k8s.go` (0.5 ngày)

- [x] `k8s_parse.go` — parseK8sItem, enrichWorkloadRow
- [x] `k8s_list.go` — ListK8s, CountK8s
- [x] `k8s.go` — types + ListClusters/Projects

### B4 — Package `addons/` (khi quay Phase 10)

```
internal/addons/
  catalog.go
  redis/
    provision.go
    helm.go
    health.go
```

Postgres sau → `internal/addons/postgres/` — **không** nhét vào `handler/projects.go`.

---

## Thứ tự thực hiện (đề xuất)

```
Tuần 1
  R0 + B0          Freeze, STACK.md, cấu trúc thư mục
  R1 (core/ui)     api, dialog, router, sidebar
  B1 (bắt đầu)     deployment_model + deployment_pick

Tuần 2
  R1 (pages)       deploy.js, k8s/ops.js, project tabs
  B1 (xong)        deployment_runtime + activity
  R2               Vite + CSS split

→ Sau đó mới Phase 10a (Redis Helm provision)
```

---

## Tiêu chí xong Refactor (gate quay Phase 10)

- [x] `app.js` &lt; 200 dòng (~20)
- [x] Không file frontend `.js` &gt; 1.500 dòng (trừ generated)
- [x] Không file `handler/deployment_*.go` / deploy / projects God-file &gt; 600 dòng
- [x] `deployments.go` stub index (&lt;20 dòng)
- [x] `./build.sh` pass; `go test ./...` pass
- [ ] Smoke: login → project → deploy / runtime / addons hub / k8s ops (trước deploy VPS)
- [x] `STACK.md` khớp code (vanilla JS modules)

**Refactor code-split (R1 + B1 + B2 + B3) ✅** — còn smoke tay + (tuỳ chọn) R2 Vite trước/cùng Phase 10a.

---

## Rủi ro & giảm thiểu

| Rủi ro | Giảm thiểu |
|--------|------------|
| Tách module gãy thứ tự load | `app.js` import order cố định; `node --check` trên bundle |
| Vite đổi path deploy | Giữ `dist/index.html` tương thích nginx hiện tại |
| Regression deploy poll | Không đổi logic — chỉ move file; test thủ công tab Deploy |
| Refactor kéo dài | Timebox R1=3 ngày; feature freeze nghiêm |

---

## Không làm trong đợt này

- Đổi React / Next.js
- Đổi Go framework
- Tối ưu API performance (đã làm overview — làm riêng nếu cần)
- Phase 10 Redis/Postgres provision (chờ gate trên)

---

*Cập nhật: 2026-07-08*
