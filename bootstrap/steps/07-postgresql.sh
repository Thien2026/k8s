#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/../lib/common.sh"

export KUBECONFIG="${ROOT_DIR}/kubeconfig/rke2.yaml"
kube_ready
helm_ready

: "${POSTGRES_PASSWORD:?Đặt POSTGRES_PASSWORD trong config/env.sh}"
: "${POSTGRESQL_CHART_VERSION:?Đặt POSTGRESQL_CHART_VERSION trong config/env.sh}"

NS="platform"
RELEASE="platform-postgresql"
DB_NAME="${POSTGRES_DB:-platform}"
DB_USER="${POSTGRES_USER:-platform}"

log "Cài PostgreSQL cho Platform Console (namespace: ${NS})"
log "Lưu ý: DB này chỉ cho portal-api (user/project). DB app khách cài riêng per-app."

ensure_storage_class() {
  if kubectl get sc local-path >/dev/null 2>&1; then
    log "StorageClass local-path đã có."
    return
  fi
  log "Chưa có StorageClass — cài local-path-provisioner..."
  kubectl apply -f https://raw.githubusercontent.com/rancher/local-path-provisioner/v0.0.30/deploy/local-path-storage.yaml
  kubectl annotate storageclass local-path storageclass.kubernetes.io/is-default-class=true --overwrite
  log "local-path-provisioner OK"
}

ensure_storage_class

helm repo add bitnami https://charts.bitnami.com/bitnami 2>/dev/null || true
helm repo update bitnami

STORAGE_CLASS="${POSTGRES_STORAGE_CLASS:-local-path}"
if kubectl get sc "${STORAGE_CLASS}" >/dev/null 2>&1; then
  log "StorageClass: ${STORAGE_CLASS}"
else
  STORAGE_CLASS="$(kubectl get sc -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)"
  [[ -n "${STORAGE_CLASS}" ]] || { log "Không có StorageClass"; exit 1; }
  log "StorageClass fallback: ${STORAGE_CLASS}"
fi

HELM_SET=(
  --set "auth.database=${DB_NAME}"
  --set "auth.username=${DB_USER}"
  --set "auth.password=${POSTGRES_PASSWORD}"
  --set "primary.persistence.size=${POSTGRES_STORAGE_SIZE:-8Gi}"
  --set "primary.persistence.storageClass=${STORAGE_CLASS}"
  --set "architecture=standalone"
)

if helm status "${RELEASE}" -n "${NS}" >/dev/null 2>&1; then
  log "${RELEASE} đã cài — upgrade nếu cần."
  helm upgrade "${RELEASE}" bitnami/postgresql \
    -n "${NS}" \
    --version "${POSTGRESQL_CHART_VERSION}" \
    "${HELM_SET[@]}"
else
  helm install "${RELEASE}" bitnami/postgresql \
    -n "${NS}" --create-namespace \
    --version "${POSTGRESQL_CHART_VERSION}" \
    "${HELM_SET[@]}"
fi

kubectl -n "${NS}" rollout status statefulset/"${RELEASE}" --timeout=300s

HOST="${RELEASE}.${NS}.svc.cluster.local"
CONN_FILE="${ROOT_DIR}/config/postgres.env"
mkdir -p "$(dirname "${CONN_FILE}")"
cat >"${CONN_FILE}" <<EOF
# Tự sinh bởi bootstrap/steps/07-postgresql.sh — không commit
POSTGRES_HOST=${HOST}
POSTGRES_PORT=5432
POSTGRES_DB=${DB_NAME}
POSTGRES_USER=${DB_USER}
POSTGRES_PASSWORD=${POSTGRES_PASSWORD}
DATABASE_URL=postgres://${DB_USER}:${POSTGRES_PASSWORD}@${HOST}:5432/${DB_NAME}?sslmode=disable
EOF
chmod 600 "${CONN_FILE}"

log "PostgreSQL OK"
log "Service: ${HOST}:5432"
log "Connection: ${CONN_FILE} (portal-api đọc file này)"
kubectl -n "${NS}" get pods -l app.kubernetes.io/instance="${RELEASE}"

mark_step_done "$0"
