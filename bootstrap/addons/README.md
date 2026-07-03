# Addon bootstrap

Cài **sau** core (`./bootstrap/run.sh` đến bước 08).

## Lệnh

```bash
./bootstrap/addons/run.sh list
./bootstrap/addons/run.sh rancher
./bootstrap/addons/run.sh harbor
./bootstrap/addons/run.sh argocd
```

## Cấu trúc

```
addons/
  run.sh                 # runner addon
  install-rancher.sh     # entry + preflight
  install-harbor.sh
  install-argocd.sh
  rancher/
    install.sh           # Helm Rancher
    bootstrap-api.sh     # API token (không cần UI)
    backup.sh            # etcd backup cron
  harbor/
    install.sh
    bootstrap-api.sh
  argocd/
    install.sh
```

## An toàn phiên bản & tài nguyên

- Pin trong `config/env.sh`: `RANCHER_CHART_VERSION`, `HARBOR_CHART_VERSION`, `ARGOCD_CHART_VERSION`
- Preflight từ chối upgrade lệch chart
- **Trước khi cài:** kiểm tra RAM + disk (có thể xem trước không cài)

```bash
./bootstrap/addons/run.sh check harbor   # chỉ đo, không helm install
./bootstrap/addons/run.sh harbor         # check + cài
./bootstrap/addons/run.sh check argocd
```

| Addon | RAM (MemAvailable) | Disk trống |
|-------|-------------------|------------|
| Rancher | ≥ 2 GiB | ≥ 15 GiB |
| Harbor | ≥ 3 GiB | ≥ 40 GiB (PVC 30Gi) |
| Argo CD | ≥ 1.5 GiB | ≥ 10 GiB |

Override: `ADDON_HARBOR_MIN_MEM_MB` trong `config/env.sh`  
Bỏ qua: `SKIP_RESOURCE_CHECK=1` | Cài dù thiếu: `FORCE_RESOURCE=1`

## Console

1. Bật addon tại `#/addons` → menu cập nhật ngay
2. SSH + chạy script cài
3. **Làm mới** trên trang Addons → badge Sẵn sàng
