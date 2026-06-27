# Rancher — Helm values

Cài bằng: `./bootstrap/addons/run.sh rancher`  
Bootstrap API (không cần UI): `bootstrap/addons/rancher/bootstrap-api.sh`

Chart: https://github.com/rancher/rancher/tree/main/chart

**Lưu ý:** Rancher là **engine** — URL `rancher.{domain}` cho DevOps, không ghi vào handover khách (khách chỉ dùng Platform Console).

## Bootstrap một lệnh

```bash
./bootstrap/run.sh 09          # Helm + 09b API token + redeploy Console
./bootstrap/run.sh 03b         # metrics-server (capacity dashboard)
./bootstrap/run.sh 01b         # sync RKE2 join secret (nếu chưa chạy qua 09/08)
```

## Thêm worker từ Console

1. Trên VPS: `cat config/join-gate.env` → copy **JOIN_GATE_SECRET** (PIN, không commit)
2. Console → **Hạ tầng → Thêm worker** → nhập PIN → **Lấy script join**
3. SSH VPS mới, paste script (chứa token — xóa sau khi join)
4. Console → **Nodes** — thấy node mới `Ready`

**Bảo mật token join:**
- Token RKE2 nằm trong Secret `platform/rke2-join`, mount read-only vào portal-api
- API **không** trả token qua GET — chỉ POST `/api/v1/cluster/join-script` + header `X-Join-Gate`
- PIN join (`JOIN_GATE_SECRET`) tách riêng, lưu `config/join-gate.env` trên VPS (chmod 600)

## Backup (handover / disaster recovery)

Sau `./bootstrap/run.sh 09c` hoặc tự động cuối bước 09:

| Việc | Chi tiết |
|------|----------|
| **etcd snapshot** | Toàn cluster (Rancher, Console, app…) qua RKE2 |
| **Config tar** | `config/`, `kubeconfig/`, `bootstrap/state/` |
| **Lịch** | Cron `0 3 * * *` (sửa `BACKUP_CRON` trong env.sh) |
| **Thư mục** | `/var/backups/platform-k8s` |
| **Log** | `/var/log/platform-backup.log` |

```bash
# Backup tay
sudo ./scripts/backup-cluster.sh

# Xem snapshot
sudo ./scripts/restore-etcd.sh list
```

**Khuyến nghị handover:** rsync `/var/backups/platform-k8s` sang NAS/S3 hàng tuần.


UI Rancher ~**7 MB JavaScript**. Tải trực tiếp VN → EU có thể **5–15 phút** hoặc treo spinner.

**Console đã đủ** cho vận hành hàng ngày (`cluster` qua API). Chỉ cần UI khi debug pod/log.

### Cách vào UI nhanh (SSH tunnel)

```bash
chmod +x scripts/rancher-tunnel.sh
./scripts/rancher-tunnel.sh
```

Rồi mở: `https://rancher.{domain}:8443/dashboard/auth/login`  
Mật khẩu admin: `config/rancher-admin.env` trên VPS.
