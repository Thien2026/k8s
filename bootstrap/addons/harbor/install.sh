#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/../../lib/common.sh"

export KUBECONFIG="${ROOT_DIR}/kubeconfig/rke2.yaml"
kube_ready
helm_ready

: "${HARBOR_HOST:?Đặt HARBOR_HOST trong config/env.sh}"
: "${HARBOR_CHART_VERSION:?Đặt HARBOR_CHART_VERSION trong config/env.sh}"

HARBOR_CHART_VERSION="${HARBOR_CHART_VERSION}"
NS="harbor"
RELEASE="harbor"
VALUES="${ROOT_DIR}/platform/harbor/values.yaml"
HARBOR_URL="https://${HARBOR_HOST}"

if [[ -z "${HARBOR_ADMIN_PASSWORD:-}" ]]; then
  if [[ -f "${ROOT_DIR}/config/harbor.env" ]]; then
    # shellcheck source=/dev/null
    source "${ROOT_DIR}/config/harbor.env"
  fi
fi
if [[ -z "${HARBOR_ADMIN_PASSWORD:-}" ]]; then
  HARBOR_ADMIN_PASSWORD="$(openssl rand -base64 18 | tr -d '/+=' | head -c 20)"
fi

log "Cài Harbor ${HARBOR_CHART_VERSION} (OSS 2.15.x) → ${HARBOR_URL}"
log "Profile: minimal — Trivy tắt, registry PVC 30Gi, local-path"

helm repo add harbor https://helm.goharbor.io 2>/dev/null || true
helm repo update harbor

kubectl create namespace "${NS}" --dry-run=client -o yaml | kubectl apply -f -

helm_args=(
  -n "${NS}"
  --version "${HARBOR_CHART_VERSION}"
  -f "${VALUES}"
  --set "expose.ingress.hosts.core=${HARBOR_HOST}"
  --set "externalURL=${HARBOR_URL}"
  --set "harborAdminPassword=${HARBOR_ADMIN_PASSWORD}"
)

if helm status "${RELEASE}" -n "${NS}" >/dev/null 2>&1; then
  log "Harbor đã cài — upgrade."
  helm upgrade "${RELEASE}" harbor/harbor "${helm_args[@]}"
else
  helm install "${RELEASE}" harbor/harbor "${helm_args[@]}"
fi

log "Đợi Harbor ready (có thể 5–10 phút lần đầu)..."
for dep in harbor-core harbor-portal harbor-jobservice harbor-registry; do
  if kubectl -n "${NS}" get deploy "${dep}" >/dev/null 2>&1; then
    kubectl -n "${NS}" rollout status "deploy/${dep}" --timeout=900s || true
  fi
done

HARBOR_ENV="${ROOT_DIR}/config/harbor.env"
cat >"${HARBOR_ENV}" <<EOF
# Tự sinh bởi 10-harbor.sh — không commit
HARBOR_URL=${HARBOR_URL}
HARBOR_HOST=${HARBOR_HOST}
HARBOR_ADMIN_USER=admin
HARBOR_ADMIN_PASSWORD=${HARBOR_ADMIN_PASSWORD}
HARBOR_PROJECT=demo
EOF
chmod 600 "${HARBOR_ENV}"

BOOTSTRAP="$(addon_script harbor bootstrap-api)"
if [[ -f "${BOOTSTRAP}" ]]; then
  log "Tạo project demo + robot account..."
  bash "${BOOTSTRAP}" || log "bootstrap-api bỏ qua (có thể đã setup)"
fi

log "Harbor UI: ${HARBOR_URL}"
log "Đăng nhập: admin / (xem ${HARBOR_ENV})"
log "Docker login: docker login ${HARBOR_HOST}"
log "Push thử: docker tag hello-world:latest ${HARBOR_HOST}/demo/hello:v1 && docker push ${HARBOR_HOST}/demo/hello:v1"

mark_addon_done harbor
