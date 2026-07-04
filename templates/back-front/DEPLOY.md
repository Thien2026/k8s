# Pilot E2E — Backend + Frontend trên Platform

Checklist triển khai **Phase 1** (dev → prod) cho team vận hành.

---

## Chuẩn bị repo

### Cách nhanh (script)

```bash
git clone git@github.com:YOUR_ORG/your-app.git /tmp/your-app
cd /path/to/k8s
./scripts/setup-back-front-pilot.sh /tmp/your-app
# → tạo branch back-front-pilot, push GitHub
```

### Cấu trúc bắt buộc trên branch

```
backend/Dockerfile
frontend/Dockerfile
frontend/dist/index.html    # hoặc build SPA vào dist/
.platform/services.yaml     # layout: multi, api + web
.platform/build.yaml        # VITE_API_BASE required
```

---

## Trên Console — checklist

- [ ] Tạo project mới (Harbor + namespace dev/prod)
- [ ] **Deploy / Git** → repo + branch `back-front-pilot` (hoặc branch của bạn)
- [ ] **Deploy env = dev**
- [ ] Kiểu chạy: **Web + API riêng**
- [ ] **Áp dụng từ repo** (nếu có banner services.yaml)
- [ ] **Cấu hình app** → Build dev: `VITE_API_BASE` = `/api`
- [ ] **Lưu & đồng bộ GitHub** → badge **Workflow OK**
- [ ] Push commit nhỏ → đợi deploy success

---

## Kiểm tra dev

| Kiểm tra | Kỳ vọng |
|----------|---------|
| Tab Deploy | Status **success**, profile **multi · api+web** |
| URL `https://{slug}-dev.{domain}/` | Trang frontend, nút gọi API |
| `.../api/health` | `{"status":"ok"}` |
| Rancher / Pods | `api` + `web` Running, cùng tag SHA |
| Harbor | `{slug}/api:sha` và `{slug}/web:sha` |

---

## Lên prod (Promote)

- [ ] Tab **Promote Prod** — checklist:
  - Workflow GitHub OK
  - Build contract dev OK
  - Env prod (nếu dev có runtime vars)
  - Domain prod
- [ ] **Promote** tag dev mới nhất
- [ ] Kiểm tra `https://{slug}-prod.{domain}/` và `/api/health`

**Lưu ý:** Promote dùng **cùng tag** cho cả api và web — đúng mô hình fleet monorepo.

---

## Project cũ (migration workflow profile)

Project đã sync workflow **trước** bản profile mới:

1. Vào **Deploy / Git**
2. **Lưu & đồng bộ GitHub** một lần (không cần đổi cấu hình)
3. Xác nhận badge **Workflow OK** trước khi push tiếp

---

## Smoke script (tùy chọn)

```bash
DEV=https://my-shop-dev.platform.7mlabs.com
curl -sf "$DEV/api/health" | grep -q ok
curl -sf -o /dev/null -w "%{http_code}" "$DEV/" | grep -q 200
echo "dev OK"
```

---

## Không thuộc Phase 1

- Polyglot 5+ service (`multi-polyglot-full`)
- Git submodule (L4C)
- Deploy thẳng prod (`Deploy env: prod`) trừ khi test có chủ đích
- Rollback / đổi layout lẫn lộn — xem [docs/MICRO_DEPLOY.md](../../docs/MICRO_DEPLOY.md)
