#!/usr/bin/env bash
# Smoke test Phase 8 — monitoring stack trên cluster.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
export KUBECONFIG="${KUBECONFIG:-${ROOT_DIR}/kubeconfig/rke2.yaml}"
export PATH="/var/lib/rancher/rke2/bin:/usr/local/bin:${PATH}"

NS="monitoring"
FAIL=0

ok() { echo "  ✓ $*"; }
fail() { echo "  ✗ $*"; FAIL=1; }

echo "== Monitoring pilot smoke =="

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
  ok "PrometheusRule test có mặt"
else
  echo "  · PrometheusRule test chưa apply (tuỳ chọn)"
fi

if [[ -f "${ROOT_DIR}/config/grafana.env" ]]; then
  # shellcheck source=/dev/null
  source "${ROOT_DIR}/config/grafana.env"
  if [[ -n "${GRAFANA_HOST:-}" ]]; then
    ok "config/grafana.env — host ${GRAFANA_HOST}"
  fi
fi

if [[ "${FAIL}" -eq 0 ]]; then
  echo ""
  echo "PASS — monitoring stack cơ bản OK."
  echo "Mở Grafana, đăng nhập, vào Alerting → xem PlatformMonitoringStackOK (nếu rule test đã apply)."
  exit 0
fi

echo ""
echo "FAIL — xem log pod: kubectl -n ${NS} get pods"
exit 1
