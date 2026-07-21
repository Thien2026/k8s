#!/usr/bin/env bash
# Disaster recovery helper: tải một backup run từ S3-compatible, verify checksum và
# chuẩn bị artifact. Không tự restore etcd hay ghi đè production — các bước đó cần
# xác nhận operator vì có tính phá hủy cluster.
set -euo pipefail

ROOT_DIR="${ROOT_DIR:-$(cd "$(dirname "$0")/.." && pwd)}"
KUBECTL="${KUBECTL:-/var/lib/rancher/rke2/bin/kubectl}"
NS="platform"
TARGET_NAME="${1:-}"
RUN_PREFIX="${2:-}"
DEST_DIR="${3:-}"

usage() {
  cat <<'EOF'
Khôi phục backup offsite (prepare + verify, không ghi đè production):

  sudo ./scripts/restore-offsite.sh <target-name> <run-prefix> [restore-dir]

Ví dụ:
  sudo ./scripts/restore-offsite.sh primary-offsite \
    platform-backups/runs/20260715T030000Z /var/tmp/platform-restore

Sau khi verify, xem RESTORE.md trong thư mục restore và thực hiện bước destructive
(etcd / replace MinIO) theo runbook, chỉ sau khi xác nhận.
EOF
}

[[ "$(id -u)" -eq 0 ]] || { echo "Chạy với root trên control-plane." >&2; exit 1; }
[[ -n "${TARGET_NAME}" && -n "${RUN_PREFIX}" ]] || { usage; exit 2; }
command -v rclone >/dev/null || { echo "Thiếu rclone." >&2; exit 1; }

export KUBECONFIG="${KUBECONFIG:-${ROOT_DIR}/kubeconfig/rke2.yaml}"
PG_POD="$("${KUBECTL}" -n "${NS}" get pod -l app=platform-postgresql -o jsonpath='{.items[0].metadata.name}')"
[[ -n "${PG_POD}" ]] || { echo "Không tìm thấy PostgreSQL pod." >&2; exit 1; }
POSTGRES_DB="$("${KUBECTL}" -n "${NS}" get secret platform-postgresql-auth -o jsonpath='{.data.POSTGRES_DB}' | base64 -d)"
POSTGRES_USER="$("${KUBECTL}" -n "${NS}" get secret platform-postgresql-auth -o jsonpath='{.data.POSTGRES_USER}' | base64 -d)"

row="$("${KUBECTL}" -n "${NS}" exec "${PG_POD}" -- psql -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" -Atq -F $'\t' -c \
  "SELECT endpoint,region,bucket,credentials_secret FROM backup_targets WHERE name=$(printf "'%s'" "$(sed "s/'/''/g" <<<"${TARGET_NAME}")")")"
IFS=$'\t' read -r ENDPOINT REGION BUCKET CREDENTIALS_SECRET <<<"${row}"
[[ -n "${BUCKET:-}" && -n "${CREDENTIALS_SECRET:-}" ]] || { echo "Target không tồn tại: ${TARGET_NAME}" >&2; exit 1; }

export RCLONE_CONFIG_TARGET_TYPE=s3
export RCLONE_CONFIG_TARGET_PROVIDER=Other
export RCLONE_CONFIG_TARGET_ENDPOINT="${ENDPOINT}"
export RCLONE_CONFIG_TARGET_REGION="${REGION}"
export RCLONE_CONFIG_TARGET_FORCE_PATH_STYLE=true
export RCLONE_CONFIG_TARGET_ACCESS_KEY_ID="$("${KUBECTL}" -n "${NS}" get secret "${CREDENTIALS_SECRET}" -o jsonpath='{.data.access_key_id}' | base64 -d)"
export RCLONE_CONFIG_TARGET_SECRET_ACCESS_KEY="$("${KUBECTL}" -n "${NS}" get secret "${CREDENTIALS_SECRET}" -o jsonpath='{.data.secret_access_key}' | base64 -d)"

DEST_DIR="${DEST_DIR:-/var/tmp/platform-restore-$(date -u +%Y%m%dT%H%M%SZ)}"
[[ ! -e "${DEST_DIR}" ]] || { echo "Thư mục đích đã tồn tại: ${DEST_DIR}" >&2; exit 1; }
install -d -m 700 "${DEST_DIR}"
rclone copy "target:${BUCKET}/${RUN_PREFIX}" "${DEST_DIR}"
(cd "${DEST_DIR}" && sha256sum -c checksums.sha256)

python3 - "${DEST_DIR}/manifest.json" <<'PY'
import json, sys
m = json.load(open(sys.argv[1], encoding="utf-8"))
i = m.get("infrastructure", {})
print("Verified run:", m.get("run_id"))
print("Created:", m.get("created_at"))
print("RKE2:", i.get("rke2_version") or "(manifest cũ)")
print("PostgreSQL:", i.get("postgres_image") or "(manifest cũ)")
PY

cat >"${DEST_DIR}/RESTORE.md" <<EOF
# Restore prepared — manual confirmation required

Checksum đã được xác minh. Không tự chạy các bước dưới đây trên production:

1. Review \`manifest.json\` và version hạ tầng.
2. PostgreSQL: restore \`platform-postgresql.dump\` vào DB cô lập trước.
3. MinIO: restore \`minio/<namespace>/app\` vào prefix/bucket tạm trước.
4. etcd: chỉ dùng snapshot \`core/etcd-snapshots/\` theo \`scripts/restore-etcd.sh\`
   sau khi xác nhận đây là DR toàn cluster.
EOF
chmod 600 "${DEST_DIR}/RESTORE.md"
echo "Restore artifacts verified: ${DEST_DIR}"
