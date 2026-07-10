#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/../../lib/common.sh"

export KUBECONFIG="${ROOT_DIR}/kubeconfig/rke2.yaml"
kube_ready
helm_ready

: "${GRAFANA_HOST:?Đặt GRAFANA_HOST trong config/env.sh}"
: "${PROMETHEUS_STACK_CHART_VERSION:?Đặt PROMETHEUS_STACK_CHART_VERSION trong config/env.sh}"

NS="monitoring"
RELEASE="kube-prometheus-stack"
VALUES="${ROOT_DIR}/platform/monitoring/values.yaml"
GRAFANA_URL="https://${GRAFANA_HOST}"

GRAFANA_ADMIN_USER="${GRAFANA_ADMIN_USER:-admin}"
GRAFANA_ADMIN_PASSWORD="${GRAFANA_ADMIN_PASSWORD:-}"
if [[ -z "${GRAFANA_ADMIN_PASSWORD}" ]]; then
  if [[ -f "${ROOT_DIR}/config/grafana.env" ]]; then
    # shellcheck source=/dev/null
    source "${ROOT_DIR}/config/grafana.env"
    GRAFANA_ADMIN_PASSWORD="${GRAFANA_ADMIN_PASSWORD:-}"
  fi
fi
if [[ -z "${GRAFANA_ADMIN_PASSWORD}" ]]; then
  GRAFANA_ADMIN_PASSWORD="$(openssl rand -base64 18 | tr -d '/+=' | head -c 20)"
  log "Tự sinh GRAFANA_ADMIN_PASSWORD (lưu vào config/grafana.env)"
fi

log "Cài kube-prometheus-stack ${PROMETHEUS_STACK_CHART_VERSION} → ${GRAFANA_URL}"

helm repo add prometheus-community https://prometheus-community.github.io/helm-charts 2>/dev/null || true
helm repo update prometheus-community

kubectl create namespace "${NS}" --dry-run=client -o yaml | kubectl apply -f -

helm_args=(
  -n "${NS}"
  --version "${PROMETHEUS_STACK_CHART_VERSION}"
  -f "${VALUES}"
  --set "grafana.ingress.hosts[0]=${GRAFANA_HOST}"
  --set "grafana.ingress.tls[0].hosts[0]=${GRAFANA_HOST}"
  --set "grafana.ingress.tls[0].secretName=grafana-tls"
  --set "grafana.adminUser=${GRAFANA_ADMIN_USER}"
  --set "grafana.adminPassword=${GRAFANA_ADMIN_PASSWORD}"
)

if helm status "${RELEASE}" -n "${NS}" >/dev/null 2>&1; then
  log "Monitoring stack đã cài — upgrade."
  helm upgrade "${RELEASE}" prometheus-community/kube-prometheus-stack "${helm_args[@]}"
else
  helm install "${RELEASE}" prometheus-community/kube-prometheus-stack "${helm_args[@]}"
fi

log "Đợi Prometheus Operator + Grafana..."
for dep in kube-prometheus-stack-operator kube-prometheus-stack-grafana; do
  if kubectl -n "${NS}" get deploy "${dep}" >/dev/null 2>&1; then
    kubectl -n "${NS}" rollout status "deploy/${dep}" --timeout=600s || true
  fi
done
if kubectl -n "${NS}" get statefulset prometheus-kube-prometheus-stack-prometheus >/dev/null 2>&1; then
  kubectl -n "${NS}" rollout status statefulset/prometheus-kube-prometheus-stack-prometheus --timeout=600s || true
fi

RULE_FILE="${ROOT_DIR}/platform/monitoring/prometheusrule-platform-test.yaml"
if [[ -f "${RULE_FILE}" ]]; then
  kubectl apply -f "${RULE_FILE}"
  log "Đã apply PrometheusRule test (PlatformMonitoringStackOK) — xem Alertmanager sau ~2 phút."
fi

REDIS_DASH="${ROOT_DIR}/platform/monitoring/grafana-dashboard-redis-addon.json"
if [[ -f "${REDIS_DASH}" ]]; then
  kubectl -n "${NS}" create configmap grafana-dashboard-redis-addon \
    --from-file=platform-redis-addon.json="${REDIS_DASH}" \
    --dry-run=client -o yaml | kubectl apply -f -
  kubectl -n "${NS}" label configmap grafana-dashboard-redis-addon grafana_dashboard=1 --overwrite
  log "Đã import Grafana dashboard Platform Redis addon (uid: platform-redis-addon)."
fi

GRAFANA_ENV="${ROOT_DIR}/config/grafana.env"
cat >"${GRAFANA_ENV}" <<EOF
# Tự sinh bởi monitoring/install.sh — không commit
GRAFANA_URL=${GRAFANA_URL}
GRAFANA_HOST=${GRAFANA_HOST}
GRAFANA_NAMESPACE=${NS}
GRAFANA_ADMIN_USER=${GRAFANA_ADMIN_USER}
GRAFANA_ADMIN_PASSWORD=${GRAFANA_ADMIN_PASSWORD}
EOF
chmod 600 "${GRAFANA_ENV}"

log "Grafana UI: ${GRAFANA_URL}"
log "Grafana user: ${GRAFANA_ADMIN_USER}"
log "Grafana password: ${GRAFANA_ADMIN_PASSWORD}"
log "Dashboard gợi ý: Kubernetes / Compute Resources / Namespace (chọn namespace project)."
log "Sau khi cài: FORCE_BUILD=1 ./bootstrap/run.sh 08 --force (portal đọc GRAFANA_*)"

mark_addon_done monitoring
