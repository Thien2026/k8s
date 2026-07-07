# Công nghệ & quyết định kiến trúc

> **Mục đích file này:** ghi *chọn gì, vì sao* — để sau vài tháng / khi handover DevOps khách vẫn hiểu.  
> Cách cài từng bước → [TASKS.md](../TASKS.md). Script → [bootstrap/README.md](../bootstrap/README.md).

---

## Tổng quan

Đây là **Kubernetes Platform Foundation** — không phải “cài K8s”, mà chuẩn hóa cluster + GitOps + observability + **một Console quản lý** cho khách.

```
Khách chỉ thấy:  https://platform.{domain}  (Platform Console)
Phía sau (engine): RKE2, Rancher, Harbor, Argo CD, Grafana, PostgreSQL
```

---

## Các lớp (layer)

| Lớp | Công nghệ | Vai trò |
|-----|-----------|---------|
| **0 — Hạ tầng** | RKE2 (mặc định) | Kubernetes chuẩn, CNCF-compatible |
| **1 — Cluster core** | Ingress (RKE2 bundle), cert-manager, StorageClass | HTTPS, routing, PVC |
| **2 — Engine** | Rancher, Harbor, Argo CD, Prometheus, Loki, Grafana | Ops K8s, image, GitOps, metric/log |
| **3 — Console** | portal-web + portal-api (tự code) | **1 dashboard** giao khách |
| **4 — DB Console** | PostgreSQL (bootstrap `07`) | User, project, metadata — **không** phải DB app khách |
| **4b — DB app** (Phase 10) | CNPG + Longhorn + MinIO | Postgres/Redis/… per project — xem [DATA-FORK.md](./DATA-FORK.md) |
| **5 — CI** | GitHub Actions | Build image → Harbor → cập nhật GitOps repo |

---

## Quyết định chính

### Kubernetes: RKE2, không K3s (bàn giao)

| Chọn | Lý do |
|------|--------|
| **RKE2** | K8s production, hợp Rancher, DevOps quen |
| Không K3s handover | Khách enterprise thường không chấp nhận |
| Không kubeadm đầu tiên | RKE2 cài nhanh hơn, vẫn chuẩn |

### Không giao khách nhiều URL

| Chọn | Lý do |
|------|--------|
| **Platform Console** (UI tự code) | Một điểm vào, branding, handover đẹp |
| Rancher/Harbor/Argo **ẩn sau API** | Engine chuẩn, không rebuild |
| Không Keycloak giai đoạn đầu | Phức tạp; login JWT trong Console đủ cho MVP |

### Frontend: React + Vite + TypeScript

| Chọn | Không chọn |
|------|------------|
| **React + Vite** (SPA) | Next.js — thừa SSR/SEO cho admin panel |
| TypeScript | — |

### Backend Console: Go

| Chọn | Lý do |
|------|--------|
| **Go** (chi / Fiber) | Bền lâu, image nhỏ, cùng hệ K8s, ít phụ thuộc |
| Không NestJS cho core | Tốt MVP nhưng npm tree nặng, vận hành lâu kém hơn Go |

### Database: PostgreSQL

| Chọn | Lý do |
|------|--------|
| **PostgreSQL** | Metadata quan hệ, chuẩn, dễ handover |
| Cài qua bootstrap `07` | Đồng bộ GitOps, tái sử dụng mọi khách |
| `config/postgres.env` | Connection cho portal-api, không commit |

**Lưu ý:** DB trên K8s **không auto-scale** như API. Portal metadata nhỏ → 1 instance + **backup** (`pg_dump` / Velero). App khách dùng DB riêng (managed hoặc operator per-app).

### Data app khách: Fork Recipe (Phase 10 — đã chốt phương án)

| Chọn | Không chọn |
|------|------------|
| **CNPG** + **Longhorn** (VolumeSnapshot) + **MinIO** (backup/export) | Neon Cloud (không khép kín) |
| **Fork Recipe** platform: schema / seed / snap / full — một API, nhiều engine adapter | Clone kiến trúc Neon OSS mặc định (nặng 12 GB) |
| Dev mặc định: `schema_and_seed` (nhẹ RAM) | Mỗi branch = CNPG cluster riêng trên 12 GB |
| **Micro:** 1 CNPG / project, N DB (`service_slug`) | Bắt buộc Neon để multi-DB |
| Preview: snap PVC → instance mới + TTL | Restore MinIO làm “branch nhanh” |

Chi tiết luồng `db-a` → snapshot → `db-a.1`, multi-DB, quota RAM → **[DATA-FORK.md](./DATA-FORK.md)**.

### Bootstrap: Bash từng file

| Chọn | Lý do |
|------|--------|
| `bootstrap/steps/XX-*.sh` | SSH rớt vẫn tiếp tục, idempotent, tick state |
| Không Ansible/Terraform ngay | Đủ cho 1 VPS; thêm sau khi multi-cloud |

### GitOps: Argo CD

| Chọn | Lý do |
|------|--------|
| **Argo CD** | Chuẩn handover, sync dev/prod, DevOps tự quản sau bàn giao |
| App deploy qua Git | Actions không `kubectl apply` trực tiếp prod |

### Registry: Harbor

| Chọn | Lý do |
|------|--------|
| **Harbor** | Registry nội bộ, scan image, robot account CI |

---

## Cấu trúc repo (mục tiêu)

```
k8s/
├── bootstrap/steps/     # Cài hạ tầng (bash)
├── platform/            # Helm / manifest engine
├── services/
│   ├── portal-web/      # React + Vite
│   └── portal-api/      # Go BFF
├── templates/new-project/
└── docs/
    └── STACK.md         # file này
```

---

## Cố ý chưa dùng (có thể thêm sau)

| Công nghệ | Khi nào thêm |
|-----------|----------------|
| Keycloak / Authentik | Khách yêu cầu SSO + LDAP/AD |
| K3s | Chỉ lab local |
| Coolify | Startup nhỏ, không enterprise handover |
| Next.js | Cần landing SEO chung repo |
| MongoDB | Không cần cho metadata Console |
| Service mesh (Istio) | Scale lớn, chưa cần phase 1 |

---

## Dev / Prod

- **Mặc định:** 1 cluster, namespace `{project}-dev` / `{project}-prod`
- **Domain:** `{app}-{env}.{domain}`
- **Promote prod:** đổi image tag GitOps → Argo CD sync (manual prod)

---

## Cập nhật file này khi nào?

- Đổi stack (vd thêm Redis, đổi operator Postgres)
- Chốt version quan trọng (pin Helm chart)
- Quyết định mới từ discussion với khách

*Cập nhật lần cuối: 2026-07-07 — thêm phương án Phase 10 [DATA-FORK.md](./DATA-FORK.md).*
