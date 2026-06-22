#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/../lib/common.sh"

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

# Ghi config cho portal-api (token thêm sau khi tạo API key trong UI)
RANCHER_ENV="${ROOT_DIR}/config/rancher.env"
cat >"${RANCHER_ENV}" <<EOF
# Tự sinh bởi 09-rancher.sh — không commit
RANCHER_URL=${RANCHER_URL}
RANCHER_TOKEN=
# Tạo API key: Rancher UI → Account & API Keys → Bearer Token → dán vào RANCHER_TOKEN
EOF
chmod 600 "${RANCHER_ENV}"

log "Rancher UI: ${RANCHER_URL}"
if [[ -n "${BOOTSTRAP_PASS}" ]]; then
  log "Bootstrap password (lần đầu đăng nhập): ${BOOTSTRAP_PASS}"
  log "Đăng nhập → đặt admin password → tạo API Key → cập nhật config/rancher.env"
else
  log "Lấy bootstrap: kubectl -n ${NS} get secret bootstrap-secret -o jsonpath='{.data.bootstrapPassword}' | base64 -d"
fi

mark_step_done "$0"
