#!/usr/bin/env bash
# Khôi phục cluster RKE2 từ etcd snapshot — PHÁ HỦY cluster hiện tại.
# Chỉ chạy khi disaster recovery. Đọc kỹ trước khi dùng.
set -euo pipefail

usage() {
  cat <<'EOF'
Khôi phục etcd RKE2 từ snapshot (disaster recovery)

  ./scripts/restore-etcd.sh list
  ./scripts/restore-etcd.sh restore <tên-snapshot>

Ví dụ:
  ./scripts/restore-etcd.sh list
  ./scripts/restore-etcd.sh restore platform-20260623-030000

Lưu ý:
  - Dừng workload trước khi restore
  - Chỉ chạy trên VPS control-plane (rke2-server)
  - Sau restore có thể cần restart rke2-server
EOF
}

export PATH="/var/lib/rancher/rke2/bin:/usr/local/bin:${PATH}"
SNAP_DIR="/var/lib/rancher/rke2/server/db/snapshots"
BACKUP_DIR="${BACKUP_DIR:-/var/backups/platform-k8s}"

cmd="${1:-}"
name="${2:-}"

case "${cmd}" in
  list)
    echo "Snapshots RKE2 (local):"
    rke2 etcd-snapshot list 2>/dev/null || ls -la "${SNAP_DIR}" 2>/dev/null || echo "(trống)"
    echo ""
    echo "Bản copy backup:"
    ls -lah "${BACKUP_DIR}/etcd-snapshots/" 2>/dev/null || echo "(chưa có — chạy backup-cluster.sh)"
    ;;
  restore)
    [[ -n "${name}" ]] || { usage; exit 1; }
    echo "CẢNH BÁO: restore etcd sẽ thay toàn bộ trạng thái cluster."
    echo "Snapshot: ${name}"
    read -r -p "Gõ YES để tiếp tục: " confirm
    [[ "${confirm}" == "YES" ]] || { echo "Hủy."; exit 1; }
    systemctl stop rke2-server 2>/dev/null || true
    rke2 server --cluster-reset --cluster-reset-restore-path="${SNAP_DIR}/${name}" \
      || rke2 etcd-snapshot restore "${name}"
    systemctl start rke2-server
    echo "Restore đã chạy — kiểm tra: kubectl get nodes"
    ;;
  *)
    usage
    exit 1
    ;;
esac
