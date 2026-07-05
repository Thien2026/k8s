# Deploy trên 7mlabs Platform (api + web)

Hướng dẫn rút gọn — **đặt trong repo app** để team dev luôn thấy khi clone.

---

## Cấu trúc repo (bắt buộc)

```
backend/Dockerfile
frontend/Dockerfile
frontend/dist/index.html    # hoặc build SPA vào dist/
.platform/services.yaml     # layout multi: api + web
.platform/build.yaml        # VITE_API_BASE
```

---

## 4 bước Console

1. **Deploy / Git** — chọn repo + branch này · **Deploy env = dev**
2. **Web + API riêng** → **Áp dụng api + web từ repo** (nếu có banner)
3. **Lưu & đồng bộ GitHub** → badge **Workflow OK**
4. **Push** → đợi success → mở `https://{slug}-dev.../` và `/api/health`

---

## Env

| Biến | Giá trị |
|------|---------|
| `VITE_API_BASE` (build dev) | `/api` |
| `API_ROUTE_PREFIX` (runtime) | `/api` |

Tab **Cấu hình app** trên Console.

---

## Prod

Dev OK → tab **Promote Prod** → checklist xanh → Promote (cùng tag api + web).

---

## Đừng

- Push trước khi sync workflow
- Chọn **Một website** khi repo multi
- Dùng rollback để đổi single ↔ multi (dùng **Đổi kiểu chạy…**)

Chi tiết: doc **KHACH_DEPLOY** trên repo platform k8s hoặc nút **`?`** trên Console.
