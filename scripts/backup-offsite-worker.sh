#!/usr/bin/env bash
# Worker offsite cho backup framework Phase 1.
# Chạy trên control-plane (root): claim đúng một backup_runs queued, tạo PostgreSQL + etcd/config
# staging, upload S3-compatible bằng rclone, rồi mới ghi success. Không ghi credential ra disk.
set -euo pipefail

ROOT_DIR="${ROOT_DIR:-$(cd "$(dirname "$0")/.." && pwd)}"
BACKUP_DIR="${BACKUP_DIR:-/var/backups/platform-k8s}"
KUBECTL="${KUBECTL:-/var/lib/rancher/rke2/bin/kubectl}"
NS="platform"
STAMP="$(date -u '+%Y%m%dT%H%M%SZ')"
WORK_DIR=""
RUN_ID=""
PG_POD=""
TEMP_SECRET=""

log() { echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*"; }
die() { log "LỖI: $*"; exit 1; }

# Không cho cron/manual worker xử lý song song: etcd snapshot và băng thông backup phải tuần tự.
exec 9>/run/lock/platform-backup-offsite.lock
flock -n 9 || { log "Worker khác đang chạy; giữ run queued cho lượt sau."; exit 0; }

sql() {
  "${KUBECTL}" -n "${NS}" exec "${PG_POD}" -- \
    psql -v ON_ERROR_STOP=1 -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" -Atq -F $'\t' -c "$1"
}

sql_literal() {
  printf "'%s'" "$(sed "s/'/''/g" <<<"$1")"
}

cleanup_expired_runs() {
  local retention_count="$1" run_id run_prefix
  # Giữ đúng N run thành công mới nhất. Chỉ xóa sau khi run hiện tại upload success.
  # Không động queued/running/failed để còn evidence điều tra hoặc worker khác xử lý.
  while IFS=$'\t' read -r run_id run_prefix; do
    [[ -n "${run_id}" && -n "${run_prefix}" ]] || continue
    log "Retention: xóa offsite run ${run_id} (${run_prefix})"
    # purge xóa toàn bộ prefix của một run; nếu lỗi thì GIỮ metadata để retry ở lần sau.
    if rclone purge "target:${BUCKET}/${run_prefix}"; then
      sql "DELETE FROM backup_runs WHERE id=${run_id} AND status='success'"
    else
      log "Cảnh báo: retention chưa xóa được run ${run_id}; sẽ thử lại lần sau."
    fi
  done < <(
    sql "SELECT id,run_prefix
         FROM backup_runs
         WHERE id IN (
           SELECT id FROM backup_runs
           WHERE target_id=(SELECT target_id FROM backup_runs WHERE id=${RUN_ID})
             AND status='success'
           ORDER BY finished_at DESC NULLS LAST, id DESC
           OFFSET ${retention_count}
         )
         ORDER BY finished_at ASC"
  )
}

finish_failed() {
  local message="${1:-Backup worker dừng bất thường}"
  if [[ -n "${RUN_ID}" ]]; then
    sql "UPDATE backup_runs SET status='failed',error_message=$(sql_literal "${message}"),finished_at=now() WHERE id=${RUN_ID} AND status='running'" || true
  fi
  [[ -n "${WORK_DIR}" ]] && rm -rf "${WORK_DIR}"
  if [[ -n "${TEMP_SECRET}" ]]; then
    "${KUBECTL}" -n "${TEMP_SECRET%%:*}" delete secret "${TEMP_SECRET#*:}" --ignore-not-found >/dev/null 2>&1 || true
  fi
}
trap 'finish_failed "worker lỗi tại dòng ${LINENO}"' ERR

command -v rclone >/dev/null || die "Thiếu rclone. Cài rclone trên control-plane trước khi bật backup offsite."
"${KUBECTL}" -n "${NS}" get statefulset platform-postgresql >/dev/null
PG_POD="$("${KUBECTL}" -n "${NS}" get pod -l app=platform-postgresql -o jsonpath='{.items[0].metadata.name}')"
[[ -n "${PG_POD}" ]] || die "Không tìm thấy PostgreSQL pod"

POSTGRES_DB="$("${KUBECTL}" -n "${NS}" get secret platform-postgresql-auth -o jsonpath='{.data.POSTGRES_DB}' | base64 -d)"
POSTGRES_USER="$("${KUBECTL}" -n "${NS}" get secret platform-postgresql-auth -o jsonpath='{.data.POSTGRES_USER}' | base64 -d)"

# SKIP LOCKED bảo đảm cron không thể xử lý cùng một run nhiều lần.
RUN_ID="$(sql "WITH next AS (SELECT id FROM backup_runs WHERE status='queued' ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1) UPDATE backup_runs r SET status='running',started_at=now() FROM next WHERE r.id=next.id RETURNING r.id")"
[[ -n "${RUN_ID}" ]] || { log "Không có backup run queued."; exit 0; }

TARGET_ROW="$(sql "SELECT t.endpoint||E'\t'||t.region||E'\t'||t.bucket||E'\t'||t.prefix||E'\t'||t.credentials_secret||E'\t'||r.run_prefix FROM backup_runs r JOIN backup_targets t ON t.id=r.target_id WHERE r.id=${RUN_ID} AND t.enabled")"
[[ -n "${TARGET_ROW}" ]] || die "Target của run ${RUN_ID} không tồn tại hoặc đã bị tắt"
IFS=$'\t' read -r ENDPOINT REGION BUCKET PREFIX CREDENTIALS_SECRET RUN_PREFIX <<<"${TARGET_ROW}"
RETENTION_COUNT="$(sql "SELECT retention_count FROM backup_targets t JOIN backup_runs r ON r.target_id=t.id WHERE r.id=${RUN_ID}")"
[[ "${RETENTION_COUNT}" =~ ^[1-9][0-9]*$ ]] || die "retention_count của target không hợp lệ"

ACCESS_KEY="$("${KUBECTL}" -n "${NS}" get secret "${CREDENTIALS_SECRET}" -o jsonpath='{.data.access_key_id}' | base64 -d)"
SECRET_KEY="$("${KUBECTL}" -n "${NS}" get secret "${CREDENTIALS_SECRET}" -o jsonpath='{.data.secret_access_key}' | base64 -d)"
[[ -n "${ACCESS_KEY}" && -n "${SECRET_KEY}" ]] || die "Secret ${CREDENTIALS_SECRET} thiếu access_key_id/secret_access_key"

WORK_DIR="$(mktemp -d "${BACKUP_DIR}/offsite-${RUN_ID}-${STAMP}.XXXX")"
chmod 700 "${WORK_DIR}"
export RCLONE_CONFIG_TARGET_TYPE="s3"
export RCLONE_CONFIG_TARGET_PROVIDER="Other"
export RCLONE_CONFIG_TARGET_ACCESS_KEY_ID="${ACCESS_KEY}"
export RCLONE_CONFIG_TARGET_SECRET_ACCESS_KEY="${SECRET_KEY}"
export RCLONE_CONFIG_TARGET_ENDPOINT="${ENDPOINT}"
export RCLONE_CONFIG_TARGET_REGION="${REGION}"
export RCLONE_CONFIG_TARGET_FORCE_PATH_STYLE="true"

log "Run ${RUN_ID}: dump PostgreSQL"
"${KUBECTL}" -n "${NS}" exec "${PG_POD}" -- \
  pg_dump -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" -Fc >"${WORK_DIR}/platform-postgresql.dump"

log "Run ${RUN_ID}: snapshot etcd/config"
BACKUP_DIR="${WORK_DIR}/core" BACKUP_RETENTION_DAYS=1 "${ROOT_DIR}/scripts/backup-cluster.sh"

# Object MinIO không được copy qua disk control-plane: mỗi bucket có service account
# riêng nên mirror trực tiếp từng bucket sang prefix tách biệt trên target offsite.
log "Run ${RUN_ID}: mirror các MinIO bucket project/env"
while IFS=$'\t' read -r PROJECT_ID ENV MINIO_NS RELEASE SOURCE_BUCKET CONN_SECRET; do
  [[ -n "${MINIO_NS}" ]] || continue
  "${KUBECTL}" -n "${MINIO_NS}" get secret "${CONN_SECRET}" >/dev/null ||
    die "Không tìm thấy MinIO connection secret ${MINIO_NS}/${CONN_SECRET}"
  TEMP_NAME="platform-backup-target-${RUN_ID}"
  TEMP_SECRET="${MINIO_NS}:${TEMP_NAME}"
  "${KUBECTL}" -n "${MINIO_NS}" create secret generic "${TEMP_NAME}" \
    --from-literal=ACCESS_KEY="${ACCESS_KEY}" --from-literal=SECRET_KEY="${SECRET_KEY}" \
    --dry-run=client -o yaml | "${KUBECTL}" -n "${MINIO_NS}" apply -f -
  JOB="platform-backup-minio-${RUN_ID}-$(echo -n "${MINIO_NS}-${SOURCE_BUCKET}" | cksum | awk '{print $1}')"
  cat <<EOF | "${KUBECTL}" -n "${MINIO_NS}" apply -f -
apiVersion: batch/v1
kind: Job
metadata:
  name: ${JOB}
  labels:
    platform/backup-run: "${RUN_ID}"
spec:
  backoffLimit: 1
  ttlSecondsAfterFinished: 3600
  template:
    metadata:
      labels:
        app: platform-backup
    spec:
      restartPolicy: Never
      containers:
        - name: mirror
          image: minio/mc:latest
          command: ["/bin/sh", "-ec"]
          args:
            - |
              mc alias set source http://${RELEASE}.${MINIO_NS}.svc.cluster.local:9000 "\$APP_KEY" "\$APP_SECRET"
              mc alias set target "${ENDPOINT}" "\$TARGET_KEY" "\$TARGET_SECRET" --api S3v4
              mc mirror --overwrite --preserve source/${SOURCE_BUCKET} target/${BUCKET}/${RUN_PREFIX}/minio/${MINIO_NS}/${SOURCE_BUCKET}
          env:
            - name: APP_KEY
              valueFrom: {secretKeyRef: {name: ${CONN_SECRET}, key: S3_ACCESS_KEY}}
            - name: APP_SECRET
              valueFrom: {secretKeyRef: {name: ${CONN_SECRET}, key: S3_SECRET_KEY}}
            - name: TARGET_KEY
              valueFrom: {secretKeyRef: {name: ${TEMP_NAME}, key: ACCESS_KEY}}
            - name: TARGET_SECRET
              valueFrom: {secretKeyRef: {name: ${TEMP_NAME}, key: SECRET_KEY}}
EOF
  for _ in $(seq 1 240); do
    succeeded="$("${KUBECTL}" -n "${MINIO_NS}" get "job/${JOB}" -o jsonpath='{.status.succeeded}')"
    failed="$("${KUBECTL}" -n "${MINIO_NS}" get "job/${JOB}" -o jsonpath='{.status.failed}')"
    [[ "${succeeded:-0}" -gt 0 ]] && break
    if [[ "${failed:-0}" -gt 0 ]]; then
      "${KUBECTL}" -n "${MINIO_NS}" logs "job/${JOB}" --all-containers=true || true
      die "MinIO mirror thất bại: ${MINIO_NS}"
    fi
    sleep 5
  done
  [[ "${succeeded:-0}" -gt 0 ]] || {
    "${KUBECTL}" -n "${MINIO_NS}" logs "job/${JOB}" --all-containers=true || true
    die "MinIO mirror quá thời gian chờ: ${MINIO_NS}"
  }
  "${KUBECTL}" -n "${MINIO_NS}" delete secret "${TEMP_NAME}" --ignore-not-found
  TEMP_SECRET=""
  OBJECT_PREFIX="${RUN_PREFIX}/minio/${MINIO_NS}/${SOURCE_BUCKET}"
  sql "INSERT INTO project_backup_artifacts
       (backup_run_id,project_id,environment,source_namespace,source_release,source_bucket,object_prefix,status)
       VALUES (${RUN_ID},${PROJECT_ID},$(sql_literal "${ENV}"),$(sql_literal "${MINIO_NS}"),$(sql_literal "${RELEASE}"),$(sql_literal "${SOURCE_BUCKET}"),$(sql_literal "${OBJECT_PREFIX}"),'success')
       ON CONFLICT (backup_run_id,project_id,environment,source_bucket) DO UPDATE
       SET source_namespace=EXCLUDED.source_namespace,source_release=EXCLUDED.source_release,
           object_prefix=EXCLUDED.object_prefix,status='success',error_message=NULL"
  printf '%s\t%s\t%s\t%s\t%s\t%s\n' "${PROJECT_ID}" "${ENV}" "${MINIO_NS}" "${RELEASE}" "${SOURCE_BUCKET}" "${OBJECT_PREFIX}" >>"${WORK_DIR}/minio-mirrors.tsv"
done < <(
  sql "SELECT p.id,'dev',p.namespace_dev, a.k8s_release, b.name, b.connection_secret
       FROM project_data_addons a JOIN projects p ON p.id=a.project_id
       JOIN project_minio_buckets b ON b.project_id=a.project_id AND b.environment=a.environment
       WHERE a.engine='minio' AND a.environment='dev' AND a.status='running' AND b.status='running'
       UNION ALL
       SELECT p.id,'prod',p.namespace_prod, a.k8s_release, b.name, b.connection_secret
       FROM project_data_addons a JOIN projects p ON p.id=a.project_id
       JOIN project_minio_buckets b ON b.project_id=a.project_id AND b.environment=a.environment
       WHERE a.engine='minio' AND a.environment='prod' AND a.status='running' AND b.status='running'"
)
unset ACCESS_KEY SECRET_KEY

MANIFEST="${WORK_DIR}/manifest.json"
# Hash bằng path tương đối để verify được sau khi tải xuống bất kỳ host/thư mục restore nào.
(
  cd "${WORK_DIR}"
  find . -type f ! -name checksums.sha256 -print0 | sort -z | xargs -0 sha256sum >checksums.sha256
)
RKE2_VERSION="$(rke2 --version 2>/dev/null | head -n 1 || true)"
POSTGRES_IMAGE="$("${KUBECTL}" -n "${NS}" get statefulset platform-postgresql -o jsonpath='{.spec.template.spec.containers[0].image}')"
BOOTSTRAP_COMMIT="$(git -C "${ROOT_DIR}" rev-parse HEAD 2>/dev/null || true)"
python3 - "${MANIFEST}" "${RUN_ID}" "${RUN_PREFIX}" "${WORK_DIR}/checksums.sha256" "${RKE2_VERSION}" "${POSTGRES_IMAGE}" "${BOOTSTRAP_COMMIT}" <<'PY'
import json, os, sys
manifest, run_id, prefix, sums, rke2, postgres_image, bootstrap_commit = sys.argv[1:]
artifacts = []
for line in open(sums, encoding="utf-8"):
    digest, path = line.rstrip("\n").split(maxsplit=1)
    artifacts.append({"path": path.removeprefix("./"),
                      "bytes": os.path.getsize(os.path.join(os.path.dirname(manifest), path)), "sha256": digest})
with open(manifest, "w", encoding="utf-8") as out:
    json.dump({"schema_version": 1, "run_id": int(run_id), "run_prefix": prefix,
               "created_at": __import__("datetime").datetime.now(__import__("datetime").timezone.utc).isoformat(),
               "infrastructure": {"rke2_version": rke2, "postgres_image": postgres_image,
                                  "bootstrap_git_commit": bootstrap_commit},
               "artifacts": artifacts}, out, indent=2, sort_keys=True)
    out.write("\n")
PY
MANIFEST_SHA="$(sha256sum "${MANIFEST}" | awk '{print $1}')"
TOTAL_BYTES="$(find "${WORK_DIR}" -type f -printf '%s\n' | awk '{s+=$1} END {print s+0}')"
ARTIFACT_COUNT="$(find "${WORK_DIR}" -type f | wc -l | tr -d ' ')"

log "Run ${RUN_ID}: upload ${ARTIFACT_COUNT} artifact lên s3://${BUCKET}/${RUN_PREFIX}"
rclone copy --checksum "${WORK_DIR}/" "target:${BUCKET}/${RUN_PREFIX}/"

sql "UPDATE backup_runs SET status='success',manifest_key=$(sql_literal "${RUN_PREFIX}/manifest.json"),artifact_count=${ARTIFACT_COUNT},total_bytes=${TOTAL_BYTES},checksum_sha256=$(sql_literal "${MANIFEST_SHA}"),error_message=NULL,finished_at=now() WHERE id=${RUN_ID} AND status='running'"
cleanup_expired_runs "${RETENTION_COUNT}"
rm -rf "${WORK_DIR}"
WORK_DIR=""
log "Run ${RUN_ID}: hoàn tất."
