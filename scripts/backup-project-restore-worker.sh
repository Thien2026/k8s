#!/usr/bin/env bash
# Worker restore MinIO theo project. Chỉ restore vào __restore/<id>/ do server sinh.
set -euo pipefail

ROOT_DIR="${ROOT_DIR:-$(cd "$(dirname "$0")/.." && pwd)}"
KUBECTL="${KUBECTL:-/var/lib/rancher/rke2/bin/kubectl}"
NS="platform"
PG_POD=""
POSTGRES_DB=""
POSTGRES_USER=""
RESTORE_ID=""
TEMP_SECRET=""

log() { echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*"; }
sql() { "${KUBECTL}" -n "${NS}" exec "${PG_POD}" -- psql -v ON_ERROR_STOP=1 -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" -Atq -F $'\t' -c "$1"; }
sql_literal() { printf "'%s'" "$(sed "s/'/''/g" <<<"$1")"; }
fail() {
  local message="${1:-restore worker lỗi}"
  [[ -n "${RESTORE_ID}" ]] && sql "UPDATE project_backup_restores SET status='failed',error_message=$(sql_literal "${message}"),finished_at=now() WHERE id=${RESTORE_ID} AND status='running'" || true
  [[ -n "${TEMP_SECRET}" ]] && "${KUBECTL}" -n "${TEMP_SECRET%%:*}" delete secret "${TEMP_SECRET#*:}" --ignore-not-found >/dev/null 2>&1 || true
  exit 1
}
trap 'fail "worker lỗi tại dòng ${LINENO}"' ERR

exec 9>/run/lock/platform-project-restore.lock
flock -n 9 || { log "Restore worker khác đang chạy."; exit 0; }
export KUBECONFIG="${KUBECONFIG:-${ROOT_DIR}/kubeconfig/rke2.yaml}"
PG_POD="$("${KUBECTL}" -n "${NS}" get pod -l app=platform-postgresql -o jsonpath='{.items[0].metadata.name}')"
[[ -n "${PG_POD}" ]] || fail "Không tìm thấy PostgreSQL pod"
POSTGRES_DB="$("${KUBECTL}" -n "${NS}" get secret platform-postgresql-auth -o jsonpath='{.data.POSTGRES_DB}' | base64 -d)"
POSTGRES_USER="$("${KUBECTL}" -n "${NS}" get secret platform-postgresql-auth -o jsonpath='{.data.POSTGRES_USER}' | base64 -d)"

RESTORE_ID="$(sql "WITH next AS (SELECT id FROM project_backup_restores WHERE status='queued' ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1) UPDATE project_backup_restores r SET status='running',started_at=now() FROM next WHERE r.id=next.id RETURNING r.id")"
[[ -n "${RESTORE_ID}" ]] || { log "Không có restore queued."; exit 0; }

ROW="$(sql "SELECT a.object_prefix,t.endpoint,t.region,t.bucket,t.credentials_secret,r.target_namespace,r.target_bucket,r.target_prefix,a.source_bucket
  FROM project_backup_restores r
  JOIN project_backup_artifacts a ON a.id=r.artifact_id
  JOIN backup_runs br ON br.id=a.backup_run_id
  JOIN backup_targets t ON t.id=br.target_id
  WHERE r.id=${RESTORE_ID} AND a.status='success' AND br.status='success'")"
IFS=$'\t' read -r SOURCE_PREFIX ENDPOINT REGION BACKUP_BUCKET CREDENTIAL_SECRET TARGET_NS TARGET_BUCKET TARGET_PREFIX SOURCE_BUCKET <<<"${ROW}"
[[ -n "${TARGET_NS:-}" && "${TARGET_PREFIX:-}" == __restore/*/ ]] || fail "restore request không hợp lệ"
# Resolve đúng credential bucket đích, không dùng instance/default app credential.
# Nếu target là bucket legacy app, migration 039 đã backfill row tương ứng.
CONN_SECRET="$(sql "SELECT b.connection_secret
  FROM project_backup_restores r
  JOIN project_minio_buckets b ON b.project_id=r.project_id
    AND b.environment=r.target_environment AND b.name=r.target_bucket
  WHERE r.id=${RESTORE_ID} AND b.status='running'")"
[[ -n "${CONN_SECRET}" ]] || fail "Bucket MinIO đích chưa running"
MINIO_ENDPOINT="$("${KUBECTL}" -n "${TARGET_NS}" get secret "${CONN_SECRET}" -o jsonpath='{.data.S3_ENDPOINT}' | base64 -d)"
[[ -n "${MINIO_ENDPOINT}" ]] || fail "Endpoint bucket đích chưa sẵn sàng"

AK="$("${KUBECTL}" -n "${NS}" get secret "${CREDENTIAL_SECRET}" -o jsonpath='{.data.access_key_id}' | base64 -d)"
SK="$("${KUBECTL}" -n "${NS}" get secret "${CREDENTIAL_SECRET}" -o jsonpath='{.data.secret_access_key}' | base64 -d)"
TEMP_NAME="platform-project-restore-${RESTORE_ID}"
TEMP_SECRET="${TARGET_NS}:${TEMP_NAME}"
"${KUBECTL}" -n "${TARGET_NS}" create secret generic "${TEMP_NAME}" --from-literal=TARGET_KEY="${AK}" --from-literal=TARGET_SECRET="${SK}" --dry-run=client -o yaml | "${KUBECTL}" -n "${TARGET_NS}" apply -f -
JOB="platform-project-restore-${RESTORE_ID}"
"${KUBECTL}" -n "${TARGET_NS}" delete job "${JOB}" --ignore-not-found >/dev/null
cat <<EOF | "${KUBECTL}" -n "${TARGET_NS}" apply -f -
apiVersion: batch/v1
kind: Job
metadata:
  name: ${JOB}
  labels: {platform/backup-restore: "${RESTORE_ID}"}
spec:
  backoffLimit: 1
  ttlSecondsAfterFinished: 3600
  template:
    metadata: {labels: {app: platform-backup}}
    spec:
      restartPolicy: Never
      containers:
        - name: restore
          image: rclone/rclone:1.69
          command: ["/bin/sh", "-ec"]
          args: ["rclone copy --checksum target:${BACKUP_BUCKET}/${SOURCE_PREFIX} restore:${TARGET_BUCKET}/${TARGET_PREFIX}"]
          env:
            - {name: RCLONE_CONFIG_TARGET_TYPE, value: s3}
            - {name: RCLONE_CONFIG_TARGET_PROVIDER, value: Other}
            - {name: RCLONE_CONFIG_TARGET_ENDPOINT, value: "${ENDPOINT}"}
            - {name: RCLONE_CONFIG_TARGET_REGION, value: "${REGION}"}
            - {name: RCLONE_CONFIG_TARGET_FORCE_PATH_STYLE, value: "true"}
            - {name: RCLONE_CONFIG_TARGET_ACCESS_KEY_ID, valueFrom: {secretKeyRef: {name: ${TEMP_NAME}, key: TARGET_KEY}}}
            - {name: RCLONE_CONFIG_TARGET_SECRET_ACCESS_KEY, valueFrom: {secretKeyRef: {name: ${TEMP_NAME}, key: TARGET_SECRET}}}
            - {name: RCLONE_CONFIG_RESTORE_TYPE, value: s3}
            - {name: RCLONE_CONFIG_RESTORE_PROVIDER, value: Minio}
            - {name: RCLONE_CONFIG_RESTORE_ENDPOINT, value: "${MINIO_ENDPOINT}"}
            - {name: RCLONE_CONFIG_RESTORE_REGION, value: us-east-1}
            - {name: RCLONE_CONFIG_RESTORE_FORCE_PATH_STYLE, value: "true"}
            - {name: RCLONE_CONFIG_RESTORE_ACCESS_KEY_ID, valueFrom: {secretKeyRef: {name: ${CONN_SECRET}, key: S3_ACCESS_KEY}}}
            - {name: RCLONE_CONFIG_RESTORE_SECRET_ACCESS_KEY, valueFrom: {secretKeyRef: {name: ${CONN_SECRET}, key: S3_SECRET_KEY}}}
EOF
for _ in $(seq 1 240); do
  ok="$("${KUBECTL}" -n "${TARGET_NS}" get "job/${JOB}" -o jsonpath='{.status.succeeded}')"
  bad="$("${KUBECTL}" -n "${TARGET_NS}" get "job/${JOB}" -o jsonpath='{.status.failed}')"
  [[ "${ok:-0}" -gt 0 ]] && break
  [[ "${bad:-0}" -gt 0 ]] && { "${KUBECTL}" -n "${TARGET_NS}" logs "job/${JOB}" || true; fail "restore Job thất bại"; }
  sleep 5
done
[[ "${ok:-0}" -gt 0 ]] || fail "restore Job quá thời gian chờ"
"${KUBECTL}" -n "${TARGET_NS}" delete secret "${TEMP_NAME}" --ignore-not-found
TEMP_SECRET=""
sql "UPDATE project_backup_restores SET status='success',finished_at=now(),error_message=NULL WHERE id=${RESTORE_ID} AND status='running'"
log "Project restore ${RESTORE_ID} hoàn tất: ${TARGET_NS}/${TARGET_BUCKET}/${TARGET_PREFIX}"
