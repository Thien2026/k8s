#!/usr/bin/env bash
# Smoke test Phase 8 — monitoring stack trên cluster.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
export KUBECONFIG="${KUBECONFIG:-${ROOT_DIR}/kubeconfig/rke2.yaml}"
export PATH="/var/lib/rancher/rke2/bin:/usr/local/bin:/usr/bin:/bin:${PATH}"

NS="monitoring"
FAIL=0

ok() { echo "  ✓ $*"; }
fail() { echo "  ✗ $*"; FAIL=1; }
warn() { echo "  · $*"; }

prom_query() {
  local expr="$1"
  local enc
  enc="$(printf '%s' "$expr" | sed 's/ /%20/g; s/{/%7B/g; s/}/%7D/g; s/"/%22/g; s/\[/%5B/g; s/\]/%5D/g')"
  kubectl exec -n "${NS}" prometheus-kube-prometheus-stack-prometheus-0 -c prometheus -- \
    wget -qO- "http://localhost:9090/api/v1/query?query=${enc}" 2>/dev/null || echo ""
}

alert_firing() {
  local name="$1"
  prom_query "ALERTS{alertname=\"${name}\",alertstate=\"firing\"}" | grep -q '"result":\[{' 2>/dev/null
}

echo "== Monitoring pilot smoke (Phase 8) =="

if ! kubectl get ns "${NS}" >/dev/null 2>&1; then
  fail "namespace ${NS} chưa có — chạy bootstrap/addons/install-monitoring.sh"
  exit 1
fi
ok "namespace ${NS}"

for dep in kube-prometheus-stack-operator kube-prometheus-stack-grafana; do
  if kubectl -n "${NS}" get deploy "${dep}" >/dev/null 2>&1; then
    ready="$(kubectl -n "${NS}" get deploy "${dep}" -o jsonpath='{.status.readyReplicas}' 2>/dev/null || echo 0)"
    if [[ "${ready:-0}" -ge 1 ]]; then
      ok "deployment ${dep} ready"
    else
      fail "deployment ${dep} chưa ready"
    fi
  else
    fail "thiếu deployment ${dep}"
  fi
done

if kubectl -n "${NS}" get statefulset prometheus-kube-prometheus-stack-prometheus >/dev/null 2>&1; then
  ready="$(kubectl -n "${NS}" get statefulset prometheus-kube-prometheus-stack-prometheus -o jsonpath='{.status.readyReplicas}' 2>/dev/null || echo 0)"
  if [[ "${ready:-0}" -ge 1 ]]; then
    ok "prometheus statefulset ready"
  else
    fail "prometheus statefulset chưa ready"
  fi
else
  fail "thiếu prometheus statefulset"
fi

if kubectl -n "${NS}" get prometheusrule platform-monitoring-test >/dev/null 2>&1; then
  ok "PrometheusRule platform-monitoring-test"
else
  fail "thiếu PrometheusRule platform-monitoring-test — kubectl apply -f platform/monitoring/prometheusrule-platform-test.yaml"
fi

if alert_firing "PlatformMonitoringStackOK"; then
  ok "alert PlatformMonitoringStackOK đang firing (pipeline OK)"
else
  fail "alert PlatformMonitoringStackOK chưa firing — đợi ~2 phút sau khi apply rule"
fi

metrics_sample="$(prom_query 'up{job="kube-state-metrics"}' || true)"
if echo "${metrics_sample}" | grep -q '"status":"success"'; then
  ok "Prometheus scrape kube-state-metrics"
else
  fail "Prometheus không scrape được kube-state-metrics"
fi

if [[ -f "${ROOT_DIR}/config/grafana.env" ]]; then
  # shellcheck source=/dev/null
  source "${ROOT_DIR}/config/grafana.env"
  if [[ -n "${GRAFANA_HOST:-}" ]]; then
    ok "config/grafana.env — host ${GRAFANA_HOST}"
    if [[ -n "${GRAFANA_URL:-}" ]]; then
      code="$(curl -sk -o /dev/null -w '%{http_code}' "${GRAFANA_URL}/login" 2>/dev/null || echo 000)"
      if [[ "${code}" == "200" || "${code}" == "302" ]]; then
        ok "Grafana HTTPS ${GRAFANA_URL} (${code})"
      else
        fail "Grafana HTTPS ${GRAFANA_URL} trả HTTP ${code}"
      fi
    fi
  fi
else
  warn "chưa có config/grafana.env — chạy monitoring/install.sh"
fi

if [[ -f "${ROOT_DIR}/config/env.sh" ]]; then
  # shellcheck source=/dev/null
  source "${ROOT_DIR}/config/env.sh"
  if [[ -n "${PROMETHEUS_STACK_CHART_VERSION:-}" ]]; then
    ok "PROMETHEUS_STACK_CHART_VERSION=${PROMETHEUS_STACK_CHART_VERSION}"
  else
    fail "thiếu PROMETHEUS_STACK_CHART_VERSION trong config/env.sh"
  fi
fi

if [[ "${FAIL}" -eq 0 ]]; then
  echo ""
  echo "PASS — Phase 8 monitoring OK (Grafana + Prometheus + alert test)."
  exit 0
fi

echo ""
echo "FAIL — xem log: kubectl -n ${NS} get pods"
exit 1
