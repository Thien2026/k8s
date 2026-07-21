#!/usr/bin/env bash
# Scanner worker Phase 2.2. Claim từng object queued; Job tải private object vào emptyDir,
# ClamAV quét và worker chỉ promote nếu exit code clean. Không có đường fail-open.
set -euo pipefail

ROOT_DIR="${ROOT_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
KUBECTL="${KUBECTL:-/var/lib/rancher/rke2/bin/kubectl}"
NS="platform"
PG_POD=""
POSTGRES_DB=""
POSTGRES_USER=""
SCAN_ID=""
TEMP_SECRET=""
CLAMAV_IMAGE="${CLAMAV_IMAGE:-clamav/clamav:1.5.3}"

log() { echo "[$(date '+%F %T')] $*"; }
sql() { "${KUBECTL}" -n "${NS}" exec "${PG_POD}" -- psql -v ON_ERROR_STOP=1 -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" -Atq -F $'\t' -c "$1"; }
q() { printf "'%s'" "$(sed "s/'/''/g" <<<"$1")"; }
fail() {
  local msg="${1:-scan worker lỗi}"
  [[ -n "${SCAN_ID}" ]] && sql "UPDATE project_minio_object_scans SET status='failed',error_message=$(q "${msg}"),finished_at=now(),updated_at=now() WHERE id=$(q "${SCAN_ID}") AND status='scanning'" || true
  [[ -n "${TEMP_SECRET}" ]] && "${KUBECTL}" -n "${TEMP_SECRET%%:*}" delete secret "${TEMP_SECRET#*:}" --ignore-not-found >/dev/null 2>&1 || true
  exit 1
}
trap 'fail "worker lỗi tại dòng ${LINENO}"' ERR

exec 9>/run/lock/platform-minio-scan.lock
flock -n 9 || { log "Scanner worker khác đang chạy."; exit 0; }
export KUBECONFIG="${KUBECONFIG:-${ROOT_DIR}/kubeconfig/rke2.yaml}"
PG_POD="$("${KUBECTL}" -n "${NS}" get pod -l app=platform-postgresql -o jsonpath='{.items[0].metadata.name}')"
POSTGRES_DB="$("${KUBECTL}" -n "${NS}" get secret platform-postgresql-auth -o jsonpath='{.data.POSTGRES_DB}' | base64 -d)"
POSTGRES_USER="$("${KUBECTL}" -n "${NS}" get secret platform-postgresql-auth -o jsonpath='{.data.POSTGRES_USER}' | base64 -d)"

SCAN_ID="$(sql "WITH next AS (
 SELECT id FROM project_minio_object_scans WHERE status='queued' ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1
) UPDATE project_minio_object_scans s SET status='scanning',attempts=attempts+1,started_at=now(),updated_at=now()
FROM next WHERE s.id=next.id RETURNING s.id")"
[[ -n "${SCAN_ID}" ]] || { log "Không có scan queued."; exit 0; }

ROW="$(sql "SELECT s.project_id,s.environment,s.quarantine_key,s.destination_key,
 p.namespace_dev,p.namespace_prod,b.name,sp.quarantine_bucket,sp.infected_bucket,
 sp.scanner_connection_secret
 FROM project_minio_object_scans s
 JOIN projects p ON p.id=s.project_id
 JOIN project_minio_buckets b ON b.id=s.bucket_id
 JOIN project_minio_scan_profiles sp ON sp.bucket_id=b.id AND sp.status='ready'
 WHERE s.id=$(q "${SCAN_ID}")")"
IFS=$'\t' read -r PROJECT_ID ENV QUARANTINE_KEY DEST_KEY NS_DEV NS_PROD CLEAN_BUCKET QUARANTINE_BUCKET INFECTED_BUCKET SCANNER_SECRET <<<"${ROW}"
TARGET_NS="${NS_DEV}"; [[ "${ENV}" == "prod" ]] && TARGET_NS="${NS_PROD}"
[[ -n "${TARGET_NS}" && -n "${SCANNER_SECRET}" ]] || fail "scan profile không hợp lệ"

JOB="platform-minio-scan-${SCAN_ID:0:12}"
TEMP_NAME="platform-minio-scan-${SCAN_ID:0:12}"
TEMP_SECRET="${TARGET_NS}:${TEMP_NAME}"
"${KUBECTL}" -n "${TARGET_NS}" create secret generic "${TEMP_NAME}" \
  --from-literal=SCAN_ID="${SCAN_ID}" --from-literal=QUARANTINE_KEY="${QUARANTINE_KEY}" \
  --from-literal=DEST_KEY="${DEST_KEY}" --from-literal=QUARANTINE_BUCKET="${QUARANTINE_BUCKET}" \
  --from-literal=CLEAN_BUCKET="${CLEAN_BUCKET}" --from-literal=INFECTED_BUCKET="${INFECTED_BUCKET}" \
  --dry-run=client -o yaml | "${KUBECTL}" -n "${TARGET_NS}" apply -f -

cat <<EOF | "${KUBECTL}" -n "${TARGET_NS}" apply -f -
apiVersion: batch/v1
kind: Job
metadata:
  name: ${JOB}
  labels: {platform/minio-scan: "${SCAN_ID}"}
spec:
  backoffLimit: 0
  activeDeadlineSeconds: 180
  ttlSecondsAfterFinished: 3600
  template:
    metadata: {labels: {platform/minio-access: scanner}}
    spec:
      restartPolicy: Never
      volumes: [{name: work, emptyDir: {sizeLimit: 512Mi}}]
      initContainers:
      - name: download
        image: minio/mc:latest
        command: ["/bin/sh", "-ec"]
        args: ['mc alias set source "\$S3_ENDPOINT" "\$S3_ACCESS_KEY" "\$S3_SECRET_KEY"; mc cp "source/\$QUARANTINE_BUCKET/\$QUARANTINE_KEY" /work/payload']
        resources: {requests: {cpu: 50m, memory: 32Mi}, limits: {cpu: 100m, memory: 64Mi}}
        volumeMounts: [{name: work, mountPath: /work}]
        env:
        - {name: QUARANTINE_BUCKET, valueFrom: {secretKeyRef: {name: ${TEMP_NAME}, key: QUARANTINE_BUCKET}}}
        - {name: QUARANTINE_KEY, valueFrom: {secretKeyRef: {name: ${TEMP_NAME}, key: QUARANTINE_KEY}}}
        - {name: S3_ENDPOINT, valueFrom: {secretKeyRef: {name: ${SCANNER_SECRET}, key: S3_ENDPOINT}}}
        - {name: S3_ACCESS_KEY, valueFrom: {secretKeyRef: {name: ${SCANNER_SECRET}, key: S3_ACCESS_KEY}}}
        - {name: S3_SECRET_KEY, valueFrom: {secretKeyRef: {name: ${SCANNER_SECRET}, key: S3_SECRET_KEY}}}
      containers:
      - name: clamav
        image: ${CLAMAV_IMAGE}
        command: ["/bin/sh", "-ec"]
        args: ['clamscan --no-summary /work/payload; echo SCAN_CLEAN']
        # Signature DB ~108 MiB, clamscan needs additional working memory. Không hạ
        # thấp giả tạo: OOM sẽ được fail-closed nhưng khiến mọi file bị kẹt quarantine.
        resources: {requests: {cpu: 300m, memory: 512Mi}, limits: {cpu: 750m, memory: 1Gi}}
        volumeMounts: [{name: work, mountPath: /work}]
EOF

for _ in $(seq 1 48); do
  ok="$("${KUBECTL}" -n "${TARGET_NS}" get "job/${JOB}" -o jsonpath='{.status.succeeded}')"
  bad="$("${KUBECTL}" -n "${TARGET_NS}" get "job/${JOB}" -o jsonpath='{.status.failed}')"
  [[ "${ok:-0}" -gt 0 ]] && break
  [[ "${bad:-0}" -gt 0 ]] && break
  sleep 5
done

if [[ "${ok:-0}" -gt 0 ]]; then
  # Server-side copy avoids sending clean data through control plane. Scanner policy
  # is the only non-root credential allowed to publish into clean.
  PROMOTE="platform-minio-promote-${SCAN_ID:0:12}"
  cat <<EOF | "${KUBECTL}" -n "${TARGET_NS}" apply -f -
apiVersion: batch/v1
kind: Job
metadata: {name: ${PROMOTE}, labels: {platform/minio-scan: "${SCAN_ID}"}}
spec:
  backoffLimit: 0
  activeDeadlineSeconds: 120
  ttlSecondsAfterFinished: 3600
  template:
    metadata: {labels: {platform/minio-access: scanner}}
    spec:
      restartPolicy: Never
      containers:
      - name: promote
        image: minio/mc:latest
        command: ["/bin/sh", "-ec"]
        args: ['mc alias set s "\$S3_ENDPOINT" "\$S3_ACCESS_KEY" "\$S3_SECRET_KEY"; mc cp "s/\$QUARANTINE_BUCKET/\$QUARANTINE_KEY" "s/\$CLEAN_BUCKET/\$DEST_KEY"; mc rm "s/\$QUARANTINE_BUCKET/\$QUARANTINE_KEY"']
        env:
        - {name: QUARANTINE_BUCKET, valueFrom: {secretKeyRef: {name: ${TEMP_NAME}, key: QUARANTINE_BUCKET}}}
        - {name: QUARANTINE_KEY, valueFrom: {secretKeyRef: {name: ${TEMP_NAME}, key: QUARANTINE_KEY}}}
        - {name: CLEAN_BUCKET, valueFrom: {secretKeyRef: {name: ${TEMP_NAME}, key: CLEAN_BUCKET}}}
        - {name: DEST_KEY, valueFrom: {secretKeyRef: {name: ${TEMP_NAME}, key: DEST_KEY}}}
        - {name: S3_ENDPOINT, valueFrom: {secretKeyRef: {name: ${SCANNER_SECRET}, key: S3_ENDPOINT}}}
        - {name: S3_ACCESS_KEY, valueFrom: {secretKeyRef: {name: ${SCANNER_SECRET}, key: S3_ACCESS_KEY}}}
        - {name: S3_SECRET_KEY, valueFrom: {secretKeyRef: {name: ${SCANNER_SECRET}, key: S3_SECRET_KEY}}}
EOF
  "${KUBECTL}" -n "${TARGET_NS}" wait --for=condition=complete "job/${PROMOTE}" --timeout=130s >/dev/null || fail "promote clean thất bại"
  sql "UPDATE project_minio_object_scans SET status='clean',finished_at=now(),scanner_image=$(q "${CLAMAV_IMAGE}"),updated_at=now() WHERE id=$(q "${SCAN_ID}")"
  log "Scan clean: ${SCAN_ID}"
else
  SCAN_LOG="$("${KUBECTL}" -n "${TARGET_NS}" logs "job/${JOB}" --all-containers=true 2>&1 || true)"
  if grep -q ' FOUND$' <<<"${SCAN_LOG}"; then
    DETECTION="$(awk -F': ' '/ FOUND$/ {sub(/ FOUND$/, "", $2); print $2; exit}' <<<"${SCAN_LOG}")"
    # Evidence stays private in infected. This is a server-side copy performed only
    # by scanner credential, then quarantine source is removed.
    INFECTED="platform-minio-infected-${SCAN_ID:0:12}"
    cat <<EOF | "${KUBECTL}" -n "${TARGET_NS}" apply -f -
apiVersion: batch/v1
kind: Job
metadata: {name: ${INFECTED}, labels: {platform/minio-scan: "${SCAN_ID}"}}
spec:
  backoffLimit: 0
  activeDeadlineSeconds: 120
  ttlSecondsAfterFinished: 3600
  template:
    metadata: {labels: {platform/minio-access: scanner}}
    spec:
      restartPolicy: Never
      containers:
      - name: quarantine
        image: minio/mc:latest
        command: ["/bin/sh", "-ec"]
        args: ['mc alias set s "\$S3_ENDPOINT" "\$S3_ACCESS_KEY" "\$S3_SECRET_KEY"; mc cp "s/\$QUARANTINE_BUCKET/\$QUARANTINE_KEY" "s/\$INFECTED_BUCKET/\$SCAN_ID/\$DEST_KEY"; mc rm "s/\$QUARANTINE_BUCKET/\$QUARANTINE_KEY"']
        env:
        - {name: SCAN_ID, valueFrom: {secretKeyRef: {name: ${TEMP_NAME}, key: SCAN_ID}}}
        - {name: QUARANTINE_BUCKET, valueFrom: {secretKeyRef: {name: ${TEMP_NAME}, key: QUARANTINE_BUCKET}}}
        - {name: QUARANTINE_KEY, valueFrom: {secretKeyRef: {name: ${TEMP_NAME}, key: QUARANTINE_KEY}}}
        - {name: INFECTED_BUCKET, valueFrom: {secretKeyRef: {name: ${TEMP_NAME}, key: INFECTED_BUCKET}}}
        - {name: DEST_KEY, valueFrom: {secretKeyRef: {name: ${TEMP_NAME}, key: DEST_KEY}}}
        - {name: S3_ENDPOINT, valueFrom: {secretKeyRef: {name: ${SCANNER_SECRET}, key: S3_ENDPOINT}}}
        - {name: S3_ACCESS_KEY, valueFrom: {secretKeyRef: {name: ${SCANNER_SECRET}, key: S3_ACCESS_KEY}}}
        - {name: S3_SECRET_KEY, valueFrom: {secretKeyRef: {name: ${SCANNER_SECRET}, key: S3_SECRET_KEY}}}
EOF
    "${KUBECTL}" -n "${TARGET_NS}" wait --for=condition=complete "job/${INFECTED}" --timeout=130s >/dev/null || fail "chuyển infected evidence thất bại"
    sql "UPDATE project_minio_object_scans SET status='infected',detection_name=$(q "${DETECTION:-unknown}"),finished_at=now(),scanner_image=$(q "${CLAMAV_IMAGE}"),updated_at=now() WHERE id=$(q "${SCAN_ID}")"
    log "Scan infected: ${SCAN_ID} (${DETECTION:-unknown})"
  else
    fail "ClamAV scan Job lỗi vận hành: ${SCAN_LOG:0:400}"
  fi
fi
"${KUBECTL}" -n "${TARGET_NS}" delete secret "${TEMP_NAME}" --ignore-not-found >/dev/null
TEMP_SECRET=""
