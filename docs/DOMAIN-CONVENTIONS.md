# Quy ước tên miền (Domain Conventions)

> **Mục đích:** Khi triển khai cluster mới, cấu hình DNS / Cloudflare, hoặc tích hợp Cloudflare API trên Console — dùng file này làm **nguồn chuẩn**.  
> Biến môi trường tương ứng: `config/env.sh` + `config/domains.env.example`.  
> Roadmap bảo mật Cloudflare qua UI: [CLOUDFLARE-INTEGRATION.md](./CLOUDFLARE-INTEGRATION.md).

---

## Biến gốc (mỗi cluster / khách)

| Biến | Ví dụ | Ý nghĩa |
|------|--------|---------|
| `DOMAIN` | `7mlabs.com` | Apex domain của khách |
| `NODE_PUBLIC_IP` | `161.97.176.55` | IP public Ingress (A record) |
| `PLATFORM_HOST` | `platform.7mlabs.com` | URL Console (có thể ≠ `platform.${DOMAIN}`) |
| `PLATFORM_DOMAIN` | `platform.7mlabs.com` | Suffix hostname app tự động (portal-api) |

**Quy tắc:** Mọi hostname trong bảng dưới thay `{DOMAIN}` / `{PLATFORM_DOMAIN}` / `{IP}` bằng giá trị thật trong `config/env.sh`.

---

## Bảng hostname theo lớp

### Lớp A — Platform engines (bootstrap addon)

Cài bằng `bootstrap/run.sh` + Helm. DevOps dùng; **không** đưa vào handover URL cho end-user.

| Host pattern | Biến env | Bootstrap | Giao thức | Ai tạo DNS |
|--------------|----------|-----------|-----------|------------|
| `platform.{DOMAIN}` | `PLATFORM_HOST` | `08-portal` | HTTPS :443 | Tay / IaC |
| `rancher.{DOMAIN}` | `RANCHER_HOST` | addon Rancher | HTTPS :443 | Tay / IaC |
| `harbor.{DOMAIN}` | `HARBOR_HOST` | addon Harbor | HTTPS :443 | Tay / IaC |
| `argocd.{DOMAIN}` | `ARGOCD_HOST` | addon Argo CD | HTTPS :443 | Tay / IaC |
| `grafana.{DOMAIN}` | `GRAFANA_HOST` | addon monitoring | HTTPS :443 | Tay / IaC |
| `hello-dev.{DOMAIN}` | `DEMO_HOST` | test Phase 1 | HTTPS :443 | Tuỳ chọn |

**Lab / chưa có domain:** `argocd.{IP}.sslip.io`, `platform.{IP}.sslip.io` — `*.sslip.io` tự resolve về IP, không cần DNS.

---

### Lớp B — Ingress routing (HTTP/HTTPS app khách)

| Host pattern | Ví dụ | Ai tạo | Ghi chú |
|--------------|-------|--------|---------|
| `{slug}-dev.{PLATFORM_DOMAIN}` | `my-shop-dev.platform.7mlabs.com` | Portal (auto) | App dev — Ingress + cert-manager |
| `{slug}-prod.{PLATFORM_DOMAIN}` | `my-shop-prod.platform.7mlabs.com` | Portal (auto) | App prod |
| `{slug}-dev.{DOMAIN}` | Khi `PLATFORM_DOMAIN` = `DOMAIN` | Portal (auto) | Biến thể cũ, vẫn hỗ trợ |
| `ingress.{DOMAIN}` | `ingress.7mlabs.com` | Tay (1 lần) | **CNAME target** cho custom domain BYOD |

Logic hostname tự động: `services/portal-api/internal/domains/platform.go` → `AutoHostname(slug, env)`.

Custom domain (khách mang domain riêng): CNAME → `ingress.{DOMAIN}` hoặc A → `NODE_PUBLIC_IP` (xem tab Domains trên Console).

---

### Lớp C — Data addons (TCP, không qua HTTP Ingress)

| Host pattern | Ví dụ | Giao thức | Ai tạo | Ghi chú |
|--------------|-------|-----------|--------|---------|
| `{slug}-redis-dev.{redis-zone}` | `research-labs-redis-dev.redis.7mlabs.com` | TCP :6379 | Portal (tương lai) / tay | Kết nối từ ngoài cluster |
| Wildcard `*.redis.{DOMAIN}` | `*.redis.7mlabs.com` | TCP :6379 | Tay (wildcard A) | Trỏ về `NODE_PUBLIC_IP`; cần stream/LB phía sau |

**Trong cluster:** Redis dùng `*.svc.cluster.local` — **không** ra internet.

**Quy ước zone Redis (đề xuất):**

```
redis.{DOMAIN}          → zone (không bắt buộc record riêng)
*.redis.{DOMAIN}        → A wildcard → NODE_PUBLIC_IP
{project}-redis-{env}.redis.{DOMAIN}  → hostname cụ thể (portal sinh sau)
```

---

## Cloudflare — cấu hình mặc định theo loại host

Dùng khi zone đã add vào Cloudflare. Chi tiết API / template rule: [CLOUDFLARE-INTEGRATION.md](./CLOUDFLARE-INTEGRATION.md).

| Loại host | Record DNS | Proxy CF | SSL mode | WAF / Rate limit |
|-----------|------------|----------|----------|------------------|
| `platform.*` | A → `NODE_PUBLIC_IP` | **Proxied** (cam) | Full (strict) | Bật — web + API Console |
| `*.{PLATFORM_DOMAIN}` (app) | A hoặc CNAME `ingress.*` | **Proxied** | Full (strict) | Bật — surface khách |
| `grafana.*` | A | Proxied (khuyến nghị) | Full | Bật + hạn chế IP / Access |
| `rancher.*`, `harbor.*`, `argocd.*` | A | **DNS only** (xám) hoặc Access | LE trên origin | Không public rộng |
| `ingress.*` | A → `NODE_PUBLIC_IP` | **DNS only** | — | Chỉ làm CNAME target |
| `*.redis.*` | A wildcard | **DNS only** (bắt buộc) | — | **Không** có WAF free cho TCP; whitelist IP / Spectrum |

**Lưu ý Let's Encrypt:** Lần đầu cấp cert, record engine (`rancher`, `harbor`…) nên **DNS only** để HTTP-01 qua Ingress thành công. Sau khi cert ổn có thể bật proxy (trừ Redis TCP).

---

## Checklist DNS — triển khai server mới

Thay `example.com` và IP thật:

```text
# Bắt buộc — Console
platform.example.com          A    <NODE_PUBLIC_IP>    [CF: proxied]

# Engines (DevOps)
rancher.example.com           A    <NODE_PUBLIC_IP>    [CF: DNS only lúc đầu]
harbor.example.com            A    <NODE_PUBLIC_IP>    [CF: DNS only lúc đầu]
argocd.example.com            A    <NODE_PUBLIC_IP>    [CF: DNS only lúc đầu]
grafana.example.com           A    <NODE_PUBLIC_IP>    [CF: proxied hoặc DNS only]

# App auto-hostname (wildcard — khuyến nghị)
*.platform.example.com        A    <NODE_PUBLIC_IP>    [CF: proxied]
# Hoặc từng record khi portal sync (không wildcard)

# CNAME target cho custom domain khách
ingress.example.com           A    <NODE_PUBLIC_IP>    [CF: DNS only]

# Redis external (nếu bật)
*.redis.example.com           A    <NODE_PUBLIC_IP>    [CF: DNS only — không proxy]
```

Sau khi sửa `config/env.sh`, chạy lại bootstrap bước liên quan (`08` portal, addon monitoring, v.v.).

---

## Map biến env ↔ hostname

| Biến `config/env.sh` | Host mặc định |
|----------------------|---------------|
| `DOMAIN` | `example.com` |
| `PLATFORM_HOST` | `platform.${DOMAIN}` |
| `PLATFORM_DOMAIN` | `platform.${DOMAIN}` |
| `RANCHER_HOST` | `rancher.${DOMAIN}` |
| `HARBOR_HOST` | `harbor.${DOMAIN}` |
| `ARGOCD_HOST` | `argocd.${DOMAIN}` |
| `GRAFANA_HOST` | `grafana.${DOMAIN}` |
| `DEMO_HOST` | `hello-dev.${DOMAIN}` |
| *(derived)* Ingress CNAME | `ingress.${DOMAIN}` |
| *(derived)* App dev | `{slug}-dev.${PLATFORM_DOMAIN}` |
| *(derived)* Redis dev | `{slug}-redis-dev.redis.${DOMAIN}` *(đề xuất)* |

File machine-readable: [`config/domains.env.example`](../config/domains.env.example).

---

## Cập nhật khi nào?

- Thêm engine mới (vd MinIO console, Loki gateway)
- Thêm data addon expose TCP (Postgres, MySQL…)
- Đổi quy ước `PLATFORM_DOMAIN` hoặc zone Redis
- Cloudflare integration bắt đầu sync record tự động

*Cập nhật: 2026-07-09 — ghi quy ước sau Phase 10a Redis + thảo luận Cloudflare.*
