# Cloudflare — tích hợp bảo mật qua Platform Console

> **Trạng thái:** Ý tưởng / roadmap — **chưa code**.  
> Quy ước hostname & proxy mặc định: [DOMAIN-CONVENTIONS.md](./DOMAIN-CONVENTIONS.md).

---

## Vấn đề

| Lớp | Rủi ro | Giải pháp hiện tại |
|-----|--------|---------------------|
| Web / API (`platform.*`, `*.platform.*`) | DDoS, bot, scan | Cloudflare proxy + WAF (cấu hình tay trên dashboard CF) |
| TCP (`*.redis.*`) | Brute force, flood port 6379 | **Không** được bảo vệ bởi CF free (chỉ HTTP/S). Cần DNS only + firewall IP / Spectrum (trả phí) / không expose |
| Engine admin (`rancher`, `harbor`…) | Lộ surface | DNS only, VPN, hoặc CF Access |

**Không** tự viết anti-DDoS engine trong cluster — dùng Cloudflare (và NetworkPolicy / firewall VPS) cho phù hợp.

---

## Mục tiêu sản phẩm

Tích hợp Cloudflare **giống GitHub OAuth** — khách connect một lần, Console quản lý cấu hình bảo mật mặc định.

```
Platform Console
  └── Settings → Integrations → Cloudflare
        ├── Connect API Token (zone DNS + WAF edit)
        ├── Chọn zone (= DOMAIN)
        └── Security (template theo DOMAIN-CONVENTIONS)
              ├── Proxy on/off theo loại host
              ├── WAF managed rules
              ├── Rate limiting (API / login)
              └── IP Access rules (admin engines)
```

---

## Phạm vi MVP

### Phase CF-1 — Kết nối & đồng bộ DNS (read + suggest)

- [ ] Lưu `CF_API_TOKEN` encrypted (platform DB / K8s secret) — scope tối thiểu: `Zone:Read`, `DNS:Edit` cho 1 zone
- [ ] UI: trạng thái connected, zone name, account email
- [ ] **Đối chiếu** record hiện có với bảng trong [DOMAIN-CONVENTIONS.md](./DOMAIN-CONVENTIONS.md) → hiển thị thiếu / sai proxy mode
- [ ] Nút **Apply defaults** — tạo/cập nhật record chuẩn (không đụng record ngoài convention)

### Phase CF-2 — Security templates

- [ ] Template `platform-web`: proxied + SSL Full (strict) + basic WAF
- [ ] Template `app-subdomains`: `*.platform.{DOMAIN}` proxied + rate limit `/api/*`
- [ ] Template `admin-engines`: DNS only + gợi ý CF Access hoặc IP whitelist
- [ ] Template `redis-tcp`: cảnh báo “DNS only, không proxy”; gợi ý whitelist IP trên VPS firewall

### Phase CF-3 — WAF / Rate limit qua API

- [ ] CRUD rate limit rules (login, API burst)
- [ ] Bật managed ruleset (OWASP cơ bản) cho zone / host cụ thể
- [ ] IP Access rules: chặn quốc gia, allowlist office IP

### Không nằm MVP

- Cloudflare Spectrum (TCP proxy trả phí) — document only
- Tự host tương đương CF WAF trong K8s
- Sửa DNS Cloudflare của **custom domain khách** (BYOD) — khách tự trỏ CNAME; Console chỉ hướng dẫn

---

## API Cloudflare tham chiếu

| Việc | API |
|------|-----|
| Verify token | `GET /user/tokens/verify` |
| List zones | `GET /zones?name={DOMAIN}` |
| DNS records | `GET/POST/PATCH /zones/{id}/dns_records` |
| Rate limits | `POST /zones/{id}/rate_limits` |
| WAF packages | Zone WAF / Rulesets API (plan-dependent) |

Token tạo tại: Cloudflare Dashboard → My Profile → API Tokens → Custom token.

**Scope khuyến nghị (MVP):**

- Zone → DNS → Edit
- Zone → Zone Settings → Read
- Zone → Firewall Services → Edit *(nếu plan hỗ trợ)*

---

## Default rules (áp dụng khi “Apply defaults”)

Ánh xạ từ [DOMAIN-CONVENTIONS.md](./DOMAIN-CONVENTIONS.md):

```yaml
# Pseudocode — config/domains.env.example
records:
  - name: "platform"
    type: A
    content: "${NODE_PUBLIC_IP}"
  - name: "*.platform"      # wildcard nếu zone hỗ trợ
    type: A
    content: "${NODE_PUBLIC_IP}"
  - name: "ingress"
    type: A
    content: "${NODE_PUBLIC_IP}"
  - name: "*.redis"
    type: A
    content: "${NODE_PUBLIC_IP}"

cloudflare_proxy:
  proxied: ["platform", "*.platform", "grafana"]
  dns_only: ["rancher", "harbor", "argocd", "ingress", "*.redis"]
```

Rate limit gợi ý (tunable):

| Path | Ngưỡng | Ghi chú |
|------|--------|---------|
| `/api/v1/auth/login` | 10 req / phút / IP | Chống brute force |
| `/api/v1/*` | 120 req / phút / IP | API Console |
| `/*` (app khách) | 300 req / phút / IP | Tuỳ project |

---

## Bảo mật token

- Không commit token; lưu encrypted trong DB platform hoặc `platform-cloudflare` K8s Secret
- Rotate token định kỳ; UI có “Disconnect” + xóa secret
- Audit log: ai bấm Apply defaults, thời điểm

---

## Liên quan code hiện có

| Thành phần | File | Ghi chú |
|------------|------|---------|
| Hostname app auto | `internal/domains/platform.go` | `AutoHostname`, `IngressCNAMETarget` |
| DNS hint BYOD | `internal/domains/dns.go` | CNAME → `ingress.{DOMAIN}` |
| GitHub OAuth pattern | `internal/handler/github_*.go` | Tham khảo flow connect integration |
| Env bootstrap | `bootstrap/core/steps/08-portal.sh` | `PLATFORM_DOMAIN`, `GRAFANA_*` inject secret |

---

## Task tracking

Khi bắt đầu implement, thêm section **Phase CF** vào [TASKS.md](../TASKS.md) và tick từng mục CF-1 → CF-3.

*Ghi nhận: 2026-07-09 — sau thảo luận DDoS, Redis TCP, NetworkPolicy whitelist.*
