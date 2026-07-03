# GitOps template cho Argo CD

Template này là điểm khởi đầu cho repo GitOps (khuyến nghị tách repo riêng).

## Cấu trúc

```text
gitops/
  apps/
    demo/
      base/
      overlays/
        dev/
        prod/
```

## Cách dùng nhanh

1. Copy toàn bộ thư mục `templates/gitops` sang repo GitOps của khách.
2. Đổi `demo` thành slug project thật.
3. Đổi `ghcr.io/example/demo:latest` thành image đúng.
4. Tạo `Application` ArgoCD trỏ vào `apps/<slug>/overlays/dev`.
5. CI chỉ cập nhật `images.newTag` trong `kustomization.yaml` (dev/prod).

## Nguyên tắc

- `dev`: auto sync.
- `prod`: sync thủ công hoặc cần approval.
- Mọi thay đổi deploy phải đi qua Git commit.
