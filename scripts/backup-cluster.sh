#!/usr/bin/env bash
# Backup etcd (RKE2) + config platform — chạy trên VPS (root).
# Cron: cài bởi bootstrap/addons/rancher/backup.sh
set -euo pipefail

ROOT_DIR="${ROOT_DIR:-$(cd "$(dirname "$0")/.." && pwd)}"
BACKUP_DIR="${BACKUP_DIR:-/var/backups/platform-k8s}"
RETENTION_DAYS="${BACKUP_RETENTION_DAYS:-14}"
STAMP="$(date '+%Y%m%d-%H%M%S')"
LOG="${BACKUP_LOG:-/var/log/platform-backup.log}"
KUBECTL="${KUBECTL:-/var/lib/rancher/rke2/bin/kubectl}"
SNAP_NAME=""

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
    # Chỉ lấy snapshot vừa tạo. Copy cả thư mục snapshot mỗi run sẽ nhân bản toàn bộ
    # lịch sử etcd lên offsite và làm dung lượng tăng lũy tiến vô ích.
    SNAP_FILES=("${SNAP_SRC}/${SNAP_NAME}"*)
    if [[ -e "${SNAP_FILES[0]}" ]]; then
      cp -a "${SNAP_FILES[@]}" "${BACKUP_DIR}/etcd-snapshots/" 2>>"${LOG}"
    else
      log "LỖI: không tìm thấy file snapshot vừa tạo (${SNAP_NAME})"
      exit 1
    fi
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

# Manifest + checksum chuẩn cho worker upload/restore. Mọi artifact local phải được hash
# trước khi được phép đẩy sang target offsite ở bước upload.
MANIFEST_DIR="${BACKUP_DIR}/manifests"
mkdir -p "${MANIFEST_DIR}"
MANIFEST="${MANIFEST_DIR}/manifest-${STAMP}.json"
CHECKSUMS="${MANIFEST_DIR}/checksums-${STAMP}.sha256"
{
  [[ -f "${CONFIG_ARCHIVE}" ]] && sha256sum "${CONFIG_ARCHIVE}"
  [[ -d "${BACKUP_DIR}/etcd-snapshots" ]] && find "${BACKUP_DIR}/etcd-snapshots" -maxdepth 1 -type f -printf '%p\n' | sort | xargs -r sha256sum
} >"${CHECKSUMS}"
python3 - "${MANIFEST}" "${STAMP}" "${CHECKSUMS}" "${CONFIG_ARCHIVE}" <<'PY'
import json, os, sys
manifest, stamp, checksums, config = sys.argv[1:]
items = []
for line in open(checksums, encoding="utf-8"):
    sha, path = line.strip().split(maxsplit=1)
    items.append({"path": path, "sha256": sha, "bytes": os.path.getsize(path)})
with open(manifest, "w", encoding="utf-8") as f:
    json.dump({"schema_version": 1, "kind": "platform-core-staging", "created_at": stamp,
               "config_archive": config, "artifacts": items}, f, indent=2, sort_keys=True)
    f.write("\n")
PY
log "Manifest + checksum: ${MANIFEST}"

# Manifest tóm tắt (không secret)
SUMMARY="${BACKUP_DIR}/last-backup.txt"
{
  echo "timestamp=${STAMP}"
  echo "backup_dir=${BACKUP_DIR}"
  echo "config_archive=${CONFIG_ARCHIVE}"
  echo "manifest=${MANIFEST}"
  echo "checksums=${CHECKSUMS}"
  echo "etcd_snapshots=${BACKUP_DIR}/etcd-snapshots"
  du -sh "${BACKUP_DIR}" 2>/dev/null || true
} >"${SUMMARY}"

log "Dọn backup config cũ hơn ${RETENTION_DAYS} ngày"
find "${BACKUP_DIR}" -maxdepth 1 -name 'config-*.tar.gz' -mtime "+${RETENTION_DAYS}" -delete 2>>"${LOG}" || true

log "=== Backup xong ==="
log "Xem: ls -lah ${BACKUP_DIR}"
