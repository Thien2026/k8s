# Rancher — Helm values

Cài bằng: `bootstrap/steps/09-rancher.sh`  
Bootstrap API (không cần UI): `bootstrap/steps/09b-rancher-bootstrap-api.sh`

Chart: https://github.com/rancher/rancher/tree/main/chart

**Lưu ý:** Rancher là **engine** — URL `rancher.{domain}` cho DevOps, không ghi vào handover khách (khách chỉ dùng Platform Console).

## UI Rancher chậm / không vào được từ VN

UI Rancher ~**7 MB JavaScript**. Tải trực tiếp VN → EU có thể **5–15 phút** hoặc treo spinner.

**Console đã đủ** cho vận hành hàng ngày (`cluster` qua API). Chỉ cần UI khi debug pod/log.

### Cách vào UI nhanh (SSH tunnel)

```bash
chmod +x scripts/rancher-tunnel.sh
./scripts/rancher-tunnel.sh
```

Rồi mở: `https://rancher.{domain}:8443/dashboard/auth/login`  
Mật khẩu admin: `config/rancher-admin.env` trên VPS.
