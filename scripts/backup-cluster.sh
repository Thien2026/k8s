#!/usr/bin/env bash
# Backup etcd (RKE2) + config platform — chạy trên VPS (root).
# Cron: cài bởi bootstrap/addons/rancher/backup.sh
set -euo pipefail

ROOT_DIR="${ROOT_DIR:-$(cd "$(dirname "$0")/.." && pwd)}"
BACKUP_DIR="${BACKUP_DIR:-/var/backups/platform-k8s}"
RETENTION_DAYS="${BACKUP_RETENTION_DAYS:-14}"
STAMP="$(date '+%Y%m%d-%H%M%S')"
LOG="${BACKUP_LOG:-/var/log/platform-backup.log}"

log() {
  echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*" | tee -a "${LOG}"
}

if [[ "$(id -u)" -ne 0 ]]; then
  echo "Chạy với root trên VPS." >&2
  exit 1
fi

export PATH="/var/lib/rancher/rke2/bin:/usr/local/bin:${PATH}"

mkdir -p "${BACKUP_DIR}"

log "=== Backup bắt đầu ==="

if command -v rke2 >/dev/null 2>&1; then
  SNAP_NAME="platform-${STAMP}"
  log "etcd snapshot: ${SNAP_NAME}"
  rke2 etcd-snapshot save --name "${SNAP_NAME}" >>"${LOG}" 2>&1 || {
    log "LỖI: rke2 etcd-snapshot thất bại"
    exit 1
  }
  SNAP_SRC="/var/lib/rancher/rke2/server/db/snapshots"
  if [[ -d "${SNAP_SRC}" ]]; then
    mkdir -p "${BACKUP_DIR}/etcd-snapshots"
    cp -a "${SNAP_SRC}/." "${BACKUP_DIR}/etcd-snapshots/" 2>>"${LOG}" || true
  fi
else
  log "Cảnh báo: không tìm thấy rke2 — bỏ qua etcd snapshot"
fi

CONFIG_ARCHIVE="${BACKUP_DIR}/config-${STAMP}.tar.gz"
log "Đóng gói config + kubeconfig → ${CONFIG_ARCHIVE}"
tar czf "${CONFIG_ARCHIVE}" \
  -C "${ROOT_DIR}" \
  config \
  kubeconfig \
  bootstrap/state \
  2>>"${LOG}" || log "Cảnh báo: một số path config không có"

# Manifest tóm tắt (không secret)
SUMMARY="${BACKUP_DIR}/last-backup.txt"
{
  echo "timestamp=${STAMP}"
  echo "backup_dir=${BACKUP_DIR}"
  echo "config_archive=${CONFIG_ARCHIVE}"
  echo "etcd_snapshots=${BACKUP_DIR}/etcd-snapshots"
  du -sh "${BACKUP_DIR}" 2>/dev/null || true
} >"${SUMMARY}"

log "Dọn backup config cũ hơn ${RETENTION_DAYS} ngày"
find "${BACKUP_DIR}" -maxdepth 1 -name 'config-*.tar.gz' -mtime "+${RETENTION_DAYS}" -delete 2>>"${LOG}" || true

log "=== Backup xong ==="
log "Xem: ls -lah ${BACKUP_DIR}"
