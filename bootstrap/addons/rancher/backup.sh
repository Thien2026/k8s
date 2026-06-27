#!/usr/bin/env bash
# Backup tự động: etcd RKE2 (Rancher + toàn cluster) + config platform.
set -euo pipefail
source "$(dirname "$0")/../../lib/common.sh"

if [[ "$(id -u)" -ne 0 ]]; then
  log "Chạy trên VPS với root (hoặc sudo)."
  exit 1
fi

BACKUP_DIR="${BACKUP_DIR:-/var/backups/platform-k8s}"
BACKUP_CRON="${BACKUP_CRON:-0 3 * * *}"
BACKUP_RETENTION_DAYS="${BACKUP_RETENTION_DAYS:-14}"

chmod +x "${ROOT_DIR}/scripts/backup-cluster.sh"
chmod +x "${ROOT_DIR}/scripts/restore-etcd.sh"

mkdir -p "${BACKUP_DIR}"
touch /var/log/platform-backup.log
chmod 640 /var/log/platform-backup.log

# RKE2: lịch snapshot etcd tích hợp (backup thêm lớp ngoài cron)
RKE2_CONFIG="/etc/rancher/rke2/config.yaml"
if [[ -f "${RKE2_CONFIG}" ]] && ! grep -q 'etcd-snapshot-schedule-cron' "${RKE2_CONFIG}" 2>/dev/null; then
  log "Thêm lịch etcd snapshot vào ${RKE2_CONFIG}"
  cat >>"${RKE2_CONFIG}" <<EOF

# Platform backup (09c-backup.sh)
etcd-snapshot-schedule-cron: "${BACKUP_CRON}"
etcd-snapshot-retention: 10
EOF
  log "RKE2 sẽ áp dụng lịch snapshot sau lần restart rke2-server tiếp theo (không restart ngay)."
fi

CRON_FILE="/etc/cron.d/platform-k8s-backup"
log "Cài cron hàng ngày: ${BACKUP_CRON}"
cat >"${CRON_FILE}" <<EOF
# Platform K8s backup — config + copy etcd snapshots
SHELL=/bin/bash
PATH=/var/lib/rancher/rke2/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
${BACKUP_CRON} root ROOT_DIR=${ROOT_DIR} BACKUP_DIR=${BACKUP_DIR} BACKUP_RETENTION_DAYS=${BACKUP_RETENTION_DAYS} ${ROOT_DIR}/scripts/backup-cluster.sh >>/var/log/platform-backup.log 2>&1
EOF
chmod 644 "${CRON_FILE}"

log "Chạy backup lần đầu (kiểm tra)..."
ROOT_DIR="${ROOT_DIR}" BACKUP_DIR="${BACKUP_DIR}" BACKUP_RETENTION_DAYS="${BACKUP_RETENTION_DAYS}" \
  "${ROOT_DIR}/scripts/backup-cluster.sh"

log "Backup dir: ${BACKUP_DIR}"
log "Log: /var/log/platform-backup.log"
log "Khôi phục: ${ROOT_DIR}/scripts/restore-etcd.sh list"
log "Khuyến nghị: rsync ${BACKUP_DIR} sang storage ngoài (S3/NAS) định kỳ"

mark_step_done "$0"
