# Hướng dẫn deploy cho khách — Backend + Frontend (Phase 1)

Tài liệu **ngắn, làm theo từng bước** — dùng khi onboard team hoặc gửi cho khách tự deploy lần đầu.

**Console:** `https://platform.7mlabs.com`  
**Mẫu repo:** `templates/back-front/` trong repo platform.

---

## Trước khi bắt đầu

| Cần có | Ghi chú |
|--------|---------|
| Tài khoản Console | Được cấp quyền ghi project |
| Repo GitHub | Có quyền push branch |
| Cấu trúc repo | `backend/` + `frontend/` + `.platform/services.yaml` |

**Cách nhanh tạo repo mẫu** (từ máy dev, trong repo platform k8s):

```bash
git clone git@github.com:YOUR_ORG/your-app.git /tmp/your-app
./scripts/setup-back-front-pilot.sh /tmp/your-app
cd /tmp/your-app && git push -u origin back-front-pilot
```

File `PLATFORM_DEPLOY.md` trong repo khách là bản rút gọn của doc này.

---

## 4 bước trên Console (bắt buộc đúng thứ tự)

### Bước 1 — Tạo project & nguồn Git

1. **Projects** → **Tạo project** (slug ví dụ `my-shop`).
2. Tab **Deploy / Git**.
3. Chọn **Repository** + **Branch** (branch có `backend/` + `frontend/`).
4. **Deploy env = dev** (để mặc định — đừng chọn prod lần đầu).
5. Bật **Tự deploy lên cluster khi build xong** (sau khi workflow OK).

### Bước 2 — Kiểu chạy: Web + API riêng

1. Chọn **Web + API riêng** (không chọn Một website).
2. Nếu thấy banner **Repo gợi ý · Web + API riêng · api + web** → bấm **「Áp dụng api + web từ repo」**.
3. Kiểm tra bảng service:
   - `api` → thư mục `backend`, ingress `/api`
   - `web` → thư mục `frontend`, ingress `/`

> **Đừng** bỏ qua bước này rồi push — workflow GitHub sẽ build sai kiểu.

### Bước 3 — Lưu & đồng bộ GitHub (bắt buộc)

1. Bấm **「Lưu & đồng bộ GitHub」** (không chỉ 「Chỉ lưu Console」).
2. Đợi vài giây — badge phải **Workflow OK** (không còn **Cần đồng bộ**).
3. Tab **Cấu hình app** → block **Build (dev)**:
   - Phải có `VITE_API_BASE` = `/api` (Console thường tự điền khi chọn Web + API).

Nếu project **đã từng deploy trước khi platform cập nhật** → vào Deploy / Git → **Lưu & đồng bộ GitHub** một lần (không cần đổi gì).

### Bước 4 — Push & kiểm tra

1. Push commit lên branch đã chọn.
2. Tab **Deploy** — đợi pipeline **success** (Build → Harbor → Cluster → Pod).
3. Mở URL dev:

```text
https://{slug}-dev.platform.7mlabs.com/
https://{slug}-dev.platform.7mlabs.com/api/health   → {"status":"ok"}
```

Kiểm tra nhanh từ terminal:

```bash
./scripts/verify-back-front-deploy.sh my-shop dev
```

---

## Lên production

**Khuyến nghị:** tab **Promote Prod** (admin/tech_lead) — cùng image tag dev, không build lại.

1. Dev deploy **success**, api + web **cùng tag SHA**.
2. Promote Prod → checklist xanh → **Promote**.
3. Kiểm tra `https://{slug}-prod.platform.7mlabs.com/` và `/api/health`.

**Không khuyến nghị lần đầu:** Deploy env = prod + push (deploy thẳng production).

---

## Đừng làm (hay gây lỗi / khách hay gặp)

| Sai | Hậu quả |
|-----|---------|
| Push trước khi **Lưu & đồng bộ GitHub** | Build/deploy lệch Console, 503 |
| Chọn **Một website** khi repo có api + web | Ingress sai, không có pod |
| **Deploy lại** bản multi khi site đang single | Rollback không đổi topology — dùng **Đổi kiểu chạy…** |
| Thiếu `VITE_API_BASE=/api` | Frontend gọi API sai URL |
| Chỉ bấm **Chỉ lưu Console** | Workflow GitHub chưa cập nhật |

---

## Lỗi thường gặp

| Triệu chứng | Cách xử lý |
|-------------|------------|
| Badge **Cần đồng bộ** | Bước 3 lại |
| 503, không có pod | **Đổi kiểu chạy…** → sync → deploy mới |
| Build OK, site trắng / API lỗi | Cấu hình app → `VITE_API_BASE=/api` → sync lại |
| GitHub Actions fail | Xem log Actions; thường thiếu secret hoặc sai đường dẫn Dockerfile |

Trên Console: tab Deploy / Git → nút **`?`** → mục **Lỗi thường gặp**.

---

## Hỗ trợ nội bộ (7mlabs)

Gửi kèm:

1. **Slug project** (vd. `my-shop`)
2. **Branch** + link repo
3. Screenshot tab **Deploy / Git** (badge workflow + kiểu chạy)
4. Output: `./scripts/verify-back-front-deploy.sh {slug} dev`

---

## Tài liệu liên quan

- [MICRO_DEPLOY.md](./MICRO_DEPLOY.md) — kỹ thuật chi tiết
- [templates/back-front/DEPLOY.md](../templates/back-front/DEPLOY.md) — checklist E2E pilot
