# Test nhanh — 5 phút

Chạy trên VPS (hoặc máy có `curl` + DNS trỏ đúng):

```bash
cd /root/k8s
./scripts/test-platform-pilots.sh
```

---

## Mở trên trình duyệt

| Mục | URL |
|-----|-----|
| **Console** | https://platform.7mlabs.com |
| **Phase 1** (api+web) | https://back-front-demo-dev.platform.7mlabs.com/ |
| **Phase 2** (polyglot) | https://polyglot-demo-dev.platform.7mlabs.com/ |
| **Pilot cũ** | https://test-harbor-dev.platform.7mlabs.com/ |

API health mỗi site: `…/api/health` → `{"status":"ok"}`

---

## Trên Console — kiểm tra nhanh

### Project `back-front-demo` (giao khách Phase 1)
1. Tab **Deploy / Git** → badge **Workflow OK**
2. Card **Hướng dẫn deploy** + nút `?`
3. Tab **Deploy** → pipeline **success**
4. 2 pod: `api`, `web`

### Project `polyglot-demo` (Phase 2)
1. Tab **Deploy / Git** → multi · api+web+node+dotnet+worker
2. Tab **Deploy** → success
3. 5 pod Running

### Project `test-harbor` (pilot nội bộ)
- Dev multi 5 service — không dùng demo khách Phase 1

---

## Test flow khách (Phase 1)

1. Đọc card **Hướng dẫn deploy** hoặc dialog `?`
2. Tạo project mới + repo `back-front-pilot` (hoặc copy template)
3. 4 bước: Git → Web+API → Lưu & sync → push
4. `./scripts/verify-back-front-deploy.sh {slug} dev`

---

## Promote prod

Tab **Promote Prod** — khi dev OK (admin). Chưa bắt buộc lúc test.

---

## Lỗi thường gặp khi test

| Triệu chứng | Xử lý |
|-------------|--------|
| Badge **Cần đồng bộ** | Deploy/Git → **Lưu & đồng bộ GitHub** |
| 503 | Kiểu chạy lệch → Áp dụng từ repo + sync |
| Pod api CrashLoop | Đã fix probe `/api/health` — sync workflow lại |
