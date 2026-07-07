# Data per project — Fork Recipe & snapshot branch

> **Trạng thái:** phương án đã chốt (discussion 2026-07-07). **Chưa triển khai** — implement ở Phase 10 (sau Phase 8–9).  
> Checklist → [TASKS.md](../TASKS.md#phase-10--data-per-project-postgres-redis-minio). Kiến trúc tổng → [STACK.md](./STACK.md).

---

## Mục tiêu

- Môi trường **khép kín**, không phụ thuộc Neon Cloud hay DB SaaS ngoài.
- Mỗi **project** có data addon (Postgres trước, sau Redis / MinIO app / MySQL / Arango…).
- UX lấy **ý tưởng Neon** (prod → fork env dev/preview) nhưng **không** clone kiến trúc Neon (pageserver / safekeepers).
- Một **API platform thống nhất** (`DataEnvironment` + `ForkRecipe`) — mọi engine cắm **adapter** riêng.

---

## Không chọn

| Hướng | Lý do |
|-------|--------|
| **Neon Cloud API** | Không khép kín; data / quota vendor |
| **Neon OSS** (mặc định) | Nặng (+3–6 GB RAM), operator early; chỉ xem xét sau nếu cần CoW Postgres và đủ RAM (24 GB+ hoặc node DB riêng) |
| **Restore full từ MinIO** làm “branch nhanh” | Chậm theo kích thước DB — chỉ dùng cho DR / fork `full` chấp nhận chờ |
| **`local-path` cho PVC DB app** | Không `VolumeSnapshot` — không có fast path snap → instance mới |

---

## Mô hình platform

### `DataEnvironment`

Một **database / data instance** gắn project + (tuỳ chọn) microservice — prod / dev / preview:

```
Project A
  └── DataPlane: 1 CNPG cluster (mặc định Tier S)
        ├── orders   db_orders_prod     role: prod
        ├── users    db_users_prod      role: prod
        ├── orders   db_orders_dev      role: dev      parent: db_orders_prod
        └── (preview plane) cluster snap → mọi DB cùng điểm T
```

Metadata (portal DB, migration sau): `data_environments`, `data_snapshots`, `data_fork_jobs`.

Trường gợi ý trên `data_environments`:

| Trường | Vai trò |
|--------|---------|
| `project_id` | Project platform |
| `service_slug` | Micro: `orders`, `users`, `catalog` — optional nếu monolith 1 DB |
| `engine` | `postgres`, `mysql`, … |
| `database_name` | Tên DB trong instance (`db_orders_prod`) |
| `cnpg_cluster_ref` | Cluster K8s chứa DB (nhiều DB / 1 cluster) |
| `role` | `prod` \| `dev` \| `preview` |
| `parent_env_id` | Fork từ env nào |

**Neon** cho phép nhiều database + branch **per DB** trong 1 project. Platform mình **làm được tương đương** cho micro “mỗi service 1 DB” — **không cần Neon**:

- **Provision:** “Thêm DB cho service `orders`” → `CREATE DATABASE` trên CNPG cluster của project (1 pod, N database).
- **Secret:** `DATABASE_URL` per service → GitOps overlay / env service tương ứng.

### `ForkRecipe` (chọn trên Console)

| Recipe | Nội dung | Pod mới? | Tốc độ | Dùng khi |
|--------|----------|----------|--------|----------|
| `empty` | DB trống + migration GitOps | Có (lần đầu) | Nhanh | Greenfield |
| `schema_only` | Chỉ structure | Không* | Nhanh | Dev hàng ngày |
| `schema_and_seed` | Structure + bảng whitelist / sample / anonymize | Không* | Nhanh–vừa | Dev có data mẫu |
| `snapshot` | Điểm T — fast: VolumeSnapshot; slow: restore MinIO | Có (preview) | Nhanh (Longhorn) / chậm (MinIO) | PR preview, giống prod tại T |
| `full` | Clone gần 100% data | Có | Chậm (trừ Neon OSS optional) | Debug hiếm, admin, TTL ngắn |

\* `schema_only` / `schema_and_seed` trên **cùng CNPG cluster** (DB mới hoặc schema) — không spawn thêm pod.

### Fork khi có nhiều DB (micro)

| Recipe | Phạm vi fork | Phù hợp micro |
|--------|--------------|---------------|
| `schema_only` / `schema_and_seed` | **Từng database** (`pg_dump` 1 DB) | ✅ Dev từng service độc lập |
| `empty` | Từng database mới | ✅ Service mới trong project |
| `volume_snapshot` | **Cả CNPG cluster** (mọi DB trên PVC) | Preview **full stack** cùng điểm T |
| Refresh dev ← prod | Per `service_slug` / per DB | ✅ Cron hoặc on-demand từng service |

**Quy tắc platform:**

- Dev hàng ngày → logical fork **per database** (mặc định Tier S).
- Preview PR cả monorepo micro → 1 snap **cả DataPlane** (mọi service cùng T).
- Preview **chỉ 1 service** → `schema_and_seed` từ prod DB đó; hoặc Tier L: 1 CNPG cluster / service (tốn RAM).

### Tier capacity (micro)

| Tier | Mô hình | RAM 12 GB | Ghi chú |
|------|---------|-----------|---------|
| **S** (mặc định) | 1 CNPG / project, N databases | ✅ | Micro khuyến nghị |
| **M** | prod cluster + preview cluster snap (TTL) | 24 GB+ | Full preview mượt |
| **L** | 1 CNPG / service (isolation) | Ít project | Snap per service; RAM phình |

So với Neon: thiếu **branch CoW instant per DB**; đủ **multi-DB + fork logical per service** — đủ cho micro self-host.

### `schema_and_seed` — options (JSON)

```yaml
fork_recipe: schema_and_seed
data_options:
  mode: include_tables | sample | reference_only
  tables: [users, roles, config]
  sample:
    max_rows_per_table: 500
  anonymize:
    users.email: fake_email()
  exclude_tables: [audit_log, events]
```

---

## Ba động cơ fork (engine-agnostic)

```
ForkRecipe
    ├── A. Logical      pg_dump / mysqldump / export — universal, nhẹ
    ├── B. Storage snap VolumeSnapshot → clone PVC → pod mới — cần Longhorn
    └── C. Native fast  Neon CoW — chỉ Postgres optional, không MVP
```

**Ưu tiên triển khai:** A (mặc định dev) → B (preview) → C (không bắt buộc).

### Luồng snap → branch (ý tưởng chính)

```
DB A (prod, PVC trên Longhorn)
    → VolumeSnapshot @ T        # vài giây
    → PVC clone từ snap
    → CNPG cluster / pod DB A.1
    → credential + inject GitOps env preview
    → TTL: xóa A.1 sau merge / 24–72h
```

**Lưu ý:** MinIO chung cluster **không** biến backup object thành branch nhanh. MinIO = backup DR + export logical (recipe A).

---

## Hạ tầng cần (so với hiện tại)

| Thành phần | Hiện tại | Phase 10 |
|------------|----------|----------|
| Storage DB app | `local-path` | **Longhorn** (`VolumeSnapshot` + clone) |
| Storage stateless | `local-path` | Giữ `local-path` (Harbor, cache…) |
| Postgres app | Chưa có | **CloudNativePG** operator |
| Postgres Console | `platform-postgresql` (07) | **Giữ nguyên** — không fork app |
| Object store | Harbor registry | **MinIO** bucket `db-backups` / `db-exports` |
| RAM VPS 12 GB | Platform + engine | Quota: preview snap **tối đa 1–2** / cluster; dev = schema+seed |

Longhorn overhead ~1–2 GB RAM — tính vào capacity trước khi bật nhiều preview.

---

## Multi-engine (sau Postgres pilot)

Cùng API `ForkFromParent(parent, recipe)`; adapter khác quiesce / start. **Nhiều DB / project** tương tự Postgres trên engine hỗ trợ multi-database:

| Engine | Nhiều DB / project | Fork per DB (logical) | Snap → instance mới |
|--------|-------------------|----------------------|---------------------|
| Postgres | ✅ CNPG multi-DB | ✅ | ✅ cả cluster |
| MySQL / MariaDB | ✅ | ✅ mysqldump 1 DB | ✅ cả instance |
| ArangoDB | ⚠️ model database/collection | ✅ export per DB | ✅ |
| MSSQL | ✅ | ✅ | ✅ (nặng RAM) |
| Redis | ⚠️ DB index / instance riêng | ⚠️ UX khác SQL | instance / RDB |

Console:

- **“Thêm database”** per service (micro) hoặc 1 DB monolith.
- **“Fork từ prod”** per `DataEnvironment` + dropdown recipe.
- Hiển thị **capability** (fork per DB vs snap cả plane) và thời gian ước tính.

---

## Snapshot vs backup/restore

| | Storage snapshot (Longhorn) | Backup → MinIO → restore |
|--|----------------------------|---------------------------|
| Tạo điểm T | Nhanh (metadata CoW) | Chậm (đọc data) |
| Spawn env mới | Clone PVC — nhanh hơn nhiều | Copy lại — chậm theo size |
| Dùng trong recipe | `snapshot` fast_path | `snapshot` slow_path, `full`, DR |

---

## RAM & quota (12 GB)

```
Mặc định dev     → schema_only / schema_and_seed (0 pod thêm)
Preview PR       → volume_snapshot → A.1 + TTL
Tránh            → nhiều CNPG cluster / nhiều preview song song
Nâng 24 GB+      → thoải mái hơn cho snap-branch
```

---

## GitOps tích hợp

- Preview deploy (PR) ↔ `DataEnvironment` role `preview` (vd. `db-a.1`).
- Secret `DATABASE_URL` inject theo env overlay (`dev` / `preview` / `prod`).
- Refresh dev từ prod: chạy lại cùng `ForkRecipe` (on-demand hoặc cron `schema_only`).

---

## Thứ tự implement (khi tới Phase 10)

1. Longhorn + StorageClass `longhorn` cho data addon  
2. MinIO (backup/export) + CNPG pilot 1 project  
3. Migration `data_environments` (+ `service_slug`, `cnpg_cluster_ref`) + API fork job (logical: `schema_only`, `schema_and_seed`)  
4. Tab **Data** Console: thêm DB per service, fork per DB, inject `DATABASE_URL` per service  
5. ForkJob `volume_snapshot` → clone PVC → `db-a.1` + TTL  
6. Adapter engine thứ hai (MySQL / …) — sau khi Postgres ổn  

---

## Tham chiếu discussion

- Không bắt buộc Neon; branch instant on-prem = Longhorn snap, không phải restore MinIO.
- Fork Recipe là abstraction platform — không phải “Neon branch” storage.
- Micro **mỗi service 1 DB**: 1 CNPG / project, N `DataEnvironment`; logical fork per DB; snap cả plane cho preview full stack.
- Phase 10 triển khai **cuối** (sau Phase 8 monitor ∥ Phase 9 terminal).

*Ghi nhận: 2026-07-07 (bổ sung multi-DB / micro cùng ngày)*
