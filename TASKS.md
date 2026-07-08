# Checklist triển khai Platform

Tick `[x]` khi xong. Làm **theo thứ tự**, không nhảy phase.

---

## Phase 0 — Chuẩn bị (làm trước khi đụng K8s)

- [ ] Chọn profile K8s: **`rke2`** (mặc định) / `kubeadm` / `eks` | `gke` | `aks`
- [ ] Liệt kê server: IP, RAM, disk (prod ≥ 3 node nếu HA)
- [ ] Mua/chuẩn bị domain + quyền DNS
- [ ] Tạo GitHub org/repo:
  - [ ] `k8s-platform` (platform components)
  - [ ] `company-gitops` (manifest app — repo khách sở hữu)
- [ ] Copy `config/env.example.yaml` → `config/env.yaml`, điền hết
- [ ] Cài local: `kubectl`, `helm`, `k9s` (optional)

**Xong phase 0 khi:** có server, domain, repo, file `env.yaml`.

---

## Phase 1 — Cluster cơ bản ⭐ BẮT ĐẦU TỪ ĐÂY

- [ ] Provision node (Terraform/Ansible/manual SSH)
- [ ] Cài Kubernetes chuẩn (RKE2 hoặc kubeadm — không K3s)
- [ ] Verify: `kubectl version`, API server + etcd + containerd chạy ổn
- [ ] `kubectl get nodes` → tất cả Ready
- [ ] Cài StorageClass (Longhorn hoặc cloud CSI)
- [ ] Cài NGINX Ingress Controller
- [ ] Cài cert-manager + ClusterIssuer (Let's Encrypt)
- [ ] Deploy app test `hello-world`:
  - [ ] Deployment + Service
  - [ ] Ingress + TLS
  - [ ] Truy cập HTTPS được qua browser
- [ ] Ghi kubeconfig + runbook backup kubeconfig

**Xong phase 1 khi:** 1 URL HTTPS public chạy ổn 24h.

---

## Phase 1b — PostgreSQL (Platform Console)

- [ ] Đặt `POSTGRES_PASSWORD` trong `config/env.sh`
- [ ] `./bootstrap/run.sh 07` — cài Postgres namespace `platform`
- [ ] Kiểm tra `config/postgres.env` sinh ra (connection cho portal-api)
- [ ] Document backup PVC (Velero hoặc pg_dump định kỳ)

**Lưu ý:** DB này chỉ lưu user/project của Console — **không** thay DB app khách.

**Xong phase 1b khi:** `platform-postgresql` pod Running, portal-api connect được.

---

## Phase 1c — Platform Console (portal-api + portal-web)

- [ ] Bước 07 PostgreSQL đã xong
- [ ] `./bootstrap/run.sh 08` — build Docker image + deploy K8s
- [ ] Mở `https://platform.{domain}` — thấy Console, DB connected

**Xong phase 1c khi:** HTTPS Console chạy, `/health` và `/api/v1/health/db` OK.

---

## Phase 2 — Rancher

- [ ] `RANCHER_CHART_VERSION` trong `config/env.sh`
- [ ] `./bootstrap/run.sh 09`
- [ ] Mở `https://rancher.{domain}` — đăng nhập bootstrap password (xem log bước 09)
- [ ] Đặt admin password + tạo **API Key** → dán vào `config/rancher.env`
- [ ] `FORCE_BUILD=1 ./bootstrap/run.sh 08 --force` (portal-api đọc Rancher token)
- [ ] Console hiển thị **Cluster (Rancher)** trên dashboard
- [ ] Import cluster local trong Rancher UI (nếu chưa auto)

**Xong phase 2 khi:** Rancher UI OK + Console hiện cluster summary (hoặc message chờ token).

---

## Phase 3 — Harbor

- [ ] Cài Harbor (Helm) + PVC
- [ ] Domain: `harbor.{domain}` + TLS
- [ ] Tạo project `demo`, robot account cho CI
- [ ] Test push image thủ công: `docker push harbor.{domain}/demo/hello:v1`
- [ ] Bật scan image (Trivy)
- [ ] Document retention policy (giữ bao nhiêu tag)

**Xong phase 3 khi:** push/pull image từ máy dev được.

---

## Phase 4 — Argo CD + app mẫu GitOps

- [ ] Cài Argo CD
- [ ] Domain: `argocd.{domain}` + TLS
- [ ] Connect repo `company-gitops`
- [ ] Tạo cấu trúc app mẫu:
  ```
  apps/demo/
    base/
    overlays/dev/
    overlays/prod/
  ```
- [ ] Argo CD Application: `demo-dev` (auto sync)
- [ ] Argo CD Application: `demo-prod` (manual sync)
- [ ] GitHub Actions workflow mẫu:
  - [ ] build → push Harbor
  - [ ] update image tag trong repo gitops (dev)
- [ ] Chạy thử full pipeline: push code → dev deploy tự động
- [ ] Promote prod: đổi tag → sync thủ công ArgoCD

**Xong phase 4 khi:** 1 app chạy full CI/CD dev, prod deploy tay qua ArgoCD.

---

## Phase 5 — Monitoring

- [ ] Cài kube-prometheus-stack (Prometheus + Grafana)
- [ ] Domain: `grafana.{domain}` + TLS
- [ ] Import dashboard: node, pod, ingress
- [ ] Cấu hình Alertmanager (Slack/email tối thiểu):
  - [ ] Node NotReady
  - [ ] Pod CrashLoopBackOff
  - [ ] Cert sắp hết hạn
- [ ] Test alert (scale deployment sai → xem có báo không)

**Xong phase 5 khi:** Grafana có metric, nhận được 1 alert test.

---

## Phase 6 — Logging

- [ ] Cài Loki + Promtail (hoặc Fluent Bit)
- [ ] Grafana datasource → Loki
- [ ] Test query log theo namespace/pod
- [ ] Set retention (vd: 7–14 ngày dev, 30 ngày prod)

**Xong phase 6 khi:** search log app demo trên Grafana được.

---

## Phase 7 — Hardening & bàn giao

- [ ] Sealed Secrets hoặc External Secrets Operator
- [ ] Kyverno policy cơ bản (bắt buộc label `env`, limit resource)
- [ ] NetworkPolicy giữa namespace dev/prod (nếu cần)
- [ ] Velero backup (etcd + PVC quan trọng)
- [ ] Pin version tất cả Helm chart vào git
- [ ] Viết/viết xong:
  - [ ] `docs/onboarding-new-app.md` (thêm project mới 5 bước)
  - [ ] `docs/runbook-incident.md`
  - [ ] `docs/handover-checklist.md`
- [ ] Workshop handover 2–4h với DevOps khách
- [ ] Hypercare 2 tuần sau bàn giao

**Xong phase 7 khi:** DevOps khách tự thêm app mới không cần bạn.

---

## Sau bàn giao — thêm project mới (5 bước)

1. Copy `templates/new-project/` → `apps/{tên-app}/`
2. Sửa tên, image, domain, resource trong overlay dev/prod
3. Thêm ArgoCD Application (hoặc ApplicationSet tự pick)
4. Copy GitHub Actions workflow, đổi secret Harbor
5. Push → kiểm tra dev → promote prod

---

## Ghi chú / blocker

| Ngày | Ghi chú |
|------|---------|
| 2026-07-07 | Phase 4 GitOps (Console UI + gitopt) live VPS. Pilot smoke OK. |
| 2026-07-07 | Chốt phương án Phase 10 → [docs/DATA-FORK.md](docs/DATA-FORK.md) (Fork Recipe, Longhorn snap, multi-DB/micro, không Neon Cloud). |

---

## Phase 8 — Monitoring & Grafana (ưu tiên sau GitOps)

> Gộp “Grafana” + “monitor” — một stack. Script: `bootstrap/addons/install-monitoring.sh`

- [x] `PROMETHEUS_STACK_CHART_VERSION` trong `config/env.sh`
- [x] `./bootstrap/addons/run.sh check monitoring` → `./bootstrap/addons/run.sh monitoring`
- [x] `grafana.{domain}` + TLS; `config/grafana.env` (password)
- [x] `FORCE_BUILD=1 ./bootstrap/run.sh 08 --force` — portal link Grafana
- [x] `./scripts/test-monitoring-pilot.sh` — smoke pass
- [x] Alert test: PrometheusRule `platform-monitoring-test` → Alertmanager
- [x] Console: card Hạ tầng Grafana + link dashboard namespace trên project overview

**Xong khi:** Grafana có metric + 1 alert test + smoke script pass. ✅ (2026-07-07)

---

## Phase 9 — Terminal an toàn + sổ lệnh K8s

- [x] Cheat sheet theo role; **Platform** vs **Project** scope
- [x] Chạy lệnh qua API Console (read-only trước), không SSH shell
- [x] Copy lệnh có placeholder `{namespace}`, `{pod}`

**Xong khi:** dev xem pod/log project; admin xem cluster không SSH. ✅ (2026-07-07)

---

## Refactor — Console code health (trước Phase 10)

> **Chi tiết:** [docs/REFACTOR-PLAN.md](docs/REFACTOR-PLAN.md)  
> **Tạm dừng Phase 10a** (Redis Helm) cho đến khi xong gate Refactor.

### R0 — Freeze

- [ ] Cập nhật `STACK.md` (frontend thực tế = vanilla JS → modules)
- [ ] Quy ước thư mục `portal-web/src/{core,ui,pages}`
- [ ] Freeze: không feature mới trong `app.js` / `deployments.go`

### R1 — Frontend tách module (P0)

> **Concat modules** (chưa Vite). Gate R1 FE ✅.

- [x] `core_state.js`, `core_format.js`, `core_api.js`, `core_router.js`
- [x] `ui_dialog.js`, `ui_sidebar.js`, `ui_auth.js`, `ui_common.js`
- [x] Project: hub / tabs / env / domains / settings / deploy / overview
- [x] Deploy: `ui_deploy_render.js`, `ui_deploy_history.js`, `ui_deploy_activity.js`, `ui_deploy_pipeline.js`, `ui_pipeline_layout.js`
- [x] Platform: `ui_platform_pages.js`, `ui_overview_charts.js`
- [x] K8s: `ui_k8s.js`
- [x] Env helpers: `ui_env_helpers.js`
- [x] `app.js` &lt; 200 dòng (~20, bootstrap only)
- [x] Không file JS page &gt;1.200 dòng
- [ ] Đổi tên/folder `core/` `ui/` `pages/` đúng quy ước (cosmetic, không block)
- [ ] Smoke manual tab project chính (trước deploy VPS)

### R2 — Vite + CSS

- [ ] Production build qua Vite
- [ ] Tách `style.css` theo domain

### B1 — Backend tách `deployments.go` (P0)

- [x] `deployment_model.go`, `deployment_pick.go`, `deployment_store.go`, `deployment_runtime.go`
- [x] `deployment_list.go`, `deployment_github.go`, `deployment_activity.go`, `deployment_hooks.go`
- [x] `deployments.go` stub index (&lt;20 dòng)
- [x] `go test ./internal/handler/...` pass; mỗi file deployment_* ≤600 dòng

### B2 — Backend deploy / projects / rancher

- [x] Tách `deploy.go` → `deploy_apply.go`, `deploy_promote.go`, `deploy_rollback.go`
- [x] Tách `projects.go` → model / overview / crud / members / repo / domains
- [x] `rancher/k8s.go` → `k8s_list.go` + `k8s_parse.go`

**Gate quay Phase 10a:** đủ tick trong REFACTOR-PLAN.md § Tiêu chí xong Refactor.

---

## Phase 10a — Data addon: Redis (làm trước Phase 10 full)

> Slice nhỏ, ship được trước Postgres/Longhorn/Fork. UX: **shell Addons riêng** trong project (sidebar lồng nhau).  
> Không cần admin bật plugin — catalog built-in.

### Console (shell + Redis MVP)

- [x] Ghi spec Phase 10a (file này)
- [x] Migration `project_data_addons`
- [x] API catalog + list / get / create (status `pending`)
- [x] Route `#/project/{slug}/addons` — hub catalog + addon đã gắn
- [x] Route `#/project/{slug}/addons/redis` — dashboard Redis (UI shell)
- [x] Sidebar addons riêng (không nhét Pods/Deployments vào menu project)
- [ ] **⏸ Chờ Refactor gate** — các mục dưới làm sau R1+B1
- [ ] Helm provision Redis trong namespace project (Bitnami)
- [ ] Sinh Secret `REDIS_URL` + copy / inject env dev
- [ ] Quota UI: `max_memory_mb`, `max_clients` → apply Helm
- [ ] Health: status, restart, logs (reuse Runtime pattern)
- [ ] Prod env + NetworkPolicy

### API

- `GET /projects/{slug}/addons` — catalog + instances
- `GET /projects/{slug}/addons/{engine}?environment=dev`
- `POST /projects/{slug}/addons/{engine}` — ghi metadata, provision (bước sau)

**Xong 10a khi:** 1 project bật Redis dev, có connection string, quota, restart — không cần fork.

---

## Phase 10 — Data per project (Postgres, Redis, MinIO)

> **Phương án chi tiết (đã chốt, chưa code):** [docs/DATA-FORK.md](docs/DATA-FORK.md)  
> Ý tưởng: **Fork Recipe** platform (lấy cảm hứng Neon, không clone Neon) — `schema_only` / `schema+seed` / `volume_snapshot` / `full`.  
> **Không** Neon Cloud. **Không** Neon OSS mặc định (12 GB). Fast fork = **Longhorn VolumeSnapshot**, không phải restore MinIO.

### Hạ tầng (trước tab Data)

- [ ] Cài **Longhorn** (Phase 1 backlog) — PVC DB app dùng SC `longhorn`, không `local-path`
- [ ] **MinIO** platform: bucket backup/export (`db-backups`, `db-exports`)
- [ ] Cài **CloudNativePG** operator
- [ ] `platform-postgresql` (bước 07) **giữ nguyên** — chỉ metadata Console

### Platform API + DB metadata

- [ ] Migration `data_environments` (+ `service_slug`, `cnpg_cluster_ref`), `data_snapshots`, `data_fork_jobs`
- [ ] Multi-DB / micro: 1 CNPG cluster / project, N database; provision “thêm DB cho service”
- [ ] `ForkRecipe`: `empty` | `schema_only` | `schema_and_seed` | `snapshot` | `full`
- [ ] Adapter Postgres (CNPG): logical fork trước, `volume_snapshot` sau
- [ ] ForkJob: snap PVC → clone → instance `db-a.1` + **TTL** preview (24–72h)
- [ ] Quota RAM 12 GB: dev = schema+seed (0 pod thêm); preview snap tối đa 1–2

### Console

- [ ] Tab **Data** — thêm DB per service, fork wizard (per DB + snap cả plane), capability theo engine
- [ ] Inject `DATABASE_URL` per service → GitOps overlay (dev / preview / prod)
- [ ] Refresh dev từ prod (on-demand hoặc cron `schema_only`)

### Pilot

- [ ] 1 project: `db-a` (prod) → fork `schema_and_seed` → dev; `volume_snapshot` → preview pilot
- [ ] Backup CNPG → MinIO (DR + slow path `full`)

### Sau pilot (không block MVP)

- [ ] ~~Redis addon~~ → chuyển **Phase 10a** (làm trước)
- [ ] Adapter MySQL / Arango (cùng Fork API, snap PVC)
- [ ] Neon OSS **optional** — chỉ khi 24 GB+ và cần CoW Postgres native

**Xong khi:** 1 project pilot có Postgres qua Console + ít nhất 1 logical fork và 1 snap fork thành công.

---

## Phase 11 — MCP (IDE / AI)

- [ ] API token; MCP wrap portal-api (deploy, env, gitops)
- [ ] Sau phase 10: provision DB/Redis

**Thứ tự:** 8 ∥ 9 → **Refactor** → 10a → 10 → 11
