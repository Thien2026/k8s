#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/../../lib/common.sh"

export KUBECONFIG="${ROOT_DIR}/kubeconfig/rke2.yaml"
kube_ready
helm_ready

: "${RANCHER_HOST:?Đặt RANCHER_HOST trong config/env.sh}"
: "${RANCHER_CHART_VERSION:?Đặt RANCHER_CHART_VERSION trong config/env.sh}"

NS="cattle-system"
RELEASE="rancher"
VALUES="${ROOT_DIR}/platform/rancher/values.yaml"
RANCHER_URL="https://${RANCHER_HOST}"

log "Cài Rancher → ${RANCHER_URL}"

helm repo add rancher-latest https://releases.rancher.com/server-charts/latest 2>/dev/null || true
helm repo update rancher-latest

kubectl create namespace "${NS}" --dry-run=client -o yaml | kubectl apply -f -

if helm status "${RELEASE}" -n "${NS}" >/dev/null 2>&1; then
  log "Rancher đã cài — upgrade."
  helm upgrade "${RELEASE}" rancher-latest/rancher \
    -n "${NS}" \
    --version "${RANCHER_CHART_VERSION}" \
    -f "${VALUES}" \
    --set "hostname=${RANCHER_HOST}"
else
  helm install "${RELEASE}" rancher-latest/rancher \
    -n "${NS}" \
    --version "${RANCHER_CHART_VERSION}" \
    -f "${VALUES}" \
    --set "hostname=${RANCHER_HOST}"
fi

log "Đợi Rancher ready (có thể 3–5 phút)..."
kubectl -n "${NS}" rollout status deploy/rancher --timeout=600s

BOOTSTRAP_PASS=""
if kubectl -n "${NS}" get secret bootstrap-secret >/dev/null 2>&1; then
  BOOTSTRAP_PASS="$(kubectl -n "${NS}" get secret bootstrap-secret -o jsonpath='{.data.bootstrapPassword}' | base64 -d)"
fi

# Ghi config cho portal-api — giữ token cũ nếu đã có
RANCHER_ENV="${ROOT_DIR}/config/rancher.env"
if [[ ! -f "${RANCHER_ENV}" ]]; then
  cat >"${RANCHER_ENV}" <<EOF
# Tự sinh bởi 09-rancher.sh — không commit
RANCHER_URL=${RANCHER_URL}
RANCHER_TOKEN=
EOF
  chmod 600 "${RANCHER_ENV}"
fi

log "Rancher UI: ${RANCHER_URL}"
if [[ -n "${BOOTSTRAP_PASS}" ]]; then
  log "Bootstrap password (lần đầu): ${BOOTSTRAP_PASS}"
fi

# Bootstrap API token + redeploy Console (không cần UI)
BOOTSTRAP_API="$(addon_script rancher bootstrap-api)"
if [[ -x "${BOOTSTRAP_API}" || -f "${BOOTSTRAP_API}" ]]; then
  log "Chạy bootstrap Rancher qua API..."
  bash "${BOOTSTRAP_API}" || log "bootstrap-api bỏ qua (có thể đã setup)"
fi

JOIN_STEP="$(core_step 01b-rke2-join-secret)"
if [[ -f "${JOIN_STEP}" ]]; then
  bash "${JOIN_STEP}" || true
fi

if grep -q 'RANCHER_TOKEN=token:' "${RANCHER_ENV}" 2>/dev/null; then
  log "Redeploy Platform Console với Rancher token..."
  FORCE_BUILD=1 bash "$(core_step 08-portal)" || \
    FORCE_BUILD=1 bash "${ROOT_DIR}/bootstrap/run.sh" 08 --force || true
else
  log "Chưa có RANCHER_TOKEN — sau khi có token: FORCE_BUILD=1 ./bootstrap/run.sh 08 --force"
fi

BACKUP_STEP="$(addon_script rancher backup)"
if [[ -f "${BACKUP_STEP}" ]]; then
  log "Thiết lập backup cluster..."
  bash "${BACKUP_STEP}" || log "backup cảnh báo — xem /var/log/platform-backup.log"
fi

mark_addon_done rancher
