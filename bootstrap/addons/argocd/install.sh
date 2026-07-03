#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/../../lib/common.sh"

export KUBECONFIG="${ROOT_DIR}/kubeconfig/rke2.yaml"
kube_ready
helm_ready

: "${ARGOCD_HOST:?Đặt ARGOCD_HOST trong config/env.sh}"
: "${ARGOCD_CHART_VERSION:?Đặt ARGOCD_CHART_VERSION trong config/env.sh}"

NS="argocd"
RELEASE="argocd"
VALUES="${ROOT_DIR}/platform/argocd/values.yaml"
ARGOCD_URL="https://${ARGOCD_HOST}"

log "Cài Argo CD ${ARGOCD_CHART_VERSION} → ${ARGOCD_URL}"

helm repo add argo https://argoproj.github.io/argo-helm 2>/dev/null || true
helm repo update argo

kubectl create namespace "${NS}" --dry-run=client -o yaml | kubectl apply -f -

helm_args=(
  -n "${NS}"
  --version "${ARGOCD_CHART_VERSION}"
  -f "${VALUES}"
  --set "global.domain=${ARGOCD_HOST}"
  --set "server.ingress.enabled=true"
  --set "server.ingress.ingressClassName=nginx"
  --set "server.ingress.hostname=${ARGOCD_HOST}"
  --set "server.ingress.tls=true"
  --set "configs.params.server\\.insecure=true"
)

if helm status "${RELEASE}" -n "${NS}" >/dev/null 2>&1; then
  log "Argo CD đã cài — upgrade."
  helm upgrade "${RELEASE}" argo/argo-cd "${helm_args[@]}"
else
  helm install "${RELEASE}" argo/argo-cd "${helm_args[@]}"
fi

log "Đợi Argo CD server/repo-server/application-controller ready..."
for dep in argocd-server argocd-repo-server argocd-applicationset-controller argocd-notifications-controller; do
  if kubectl -n "${NS}" get deploy "${dep}" >/dev/null 2>&1; then
    kubectl -n "${NS}" rollout status "deploy/${dep}" --timeout=600s || true
  fi
done
if kubectl -n "${NS}" get statefulset argocd-application-controller >/dev/null 2>&1; then
  kubectl -n "${NS}" rollout status statefulset/argocd-application-controller --timeout=600s || true
fi

ARGOCD_ENV="${ROOT_DIR}/config/argocd.env"
cat >"${ARGOCD_ENV}" <<EOF
# Tự sinh bởi 11-argocd.sh — không commit
ARGOCD_URL=${ARGOCD_URL}
ARGOCD_HOST=${ARGOCD_HOST}
ARGOCD_NAMESPACE=${NS}
EOF
chmod 600 "${ARGOCD_ENV}"

if kubectl -n "${NS}" get secret argocd-initial-admin-secret >/dev/null 2>&1; then
  INITIAL_PASS="$(kubectl -n "${NS}" get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' | base64 -d || true)"
  if [[ -n "${INITIAL_PASS}" ]]; then
    log "Argo CD admin initial password: ${INITIAL_PASS}"
  fi
fi

log "Argo CD UI: ${ARGOCD_URL}"
log "Sau khi đăng nhập, đổi mật khẩu admin ngay."

mark_addon_done argocd
