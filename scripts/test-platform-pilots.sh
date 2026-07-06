#!/usr/bin/env bash
# Smoke test nhanh — chạy trước khi demo / giao khách.
# Usage: ./scripts/test-platform-pilots.sh [BASE_DOMAIN]
set -euo pipefail

DOMAIN="${1:-platform.7mlabs.com}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
export KUBECONFIG="${KUBECONFIG:-${ROOT}/kubeconfig/rke2.yaml}"
export PATH="/var/lib/rancher/rke2/bin:${PATH}"

FAIL=0
pass() { echo "✓ $*"; }
fail() { echo "✗ $*"; FAIL=1; }

echo "════════════════════════════════════════"
echo " Platform pilot smoke · ${DOMAIN}"
echo "════════════════════════════════════════"
echo ""

# Phase 1 — api+web
echo "── Phase 1: back-front-demo (api+web) ──"
if "${ROOT}/scripts/verify-back-front-deploy.sh" back-front-demo dev "${DOMAIN}"; then
  pass "back-front-demo dev"
else
  fail "back-front-demo dev"
fi
echo ""

# Phase 2 — polyglot fleet
echo "── Phase 2: polyglot-demo (5 service) ──"
if "${ROOT}/scripts/verify-polyglot-deploy.sh" polyglot-demo dev "${DOMAIN}"; then
  pass "polyglot-demo dev"
else
  fail "polyglot-demo dev"
fi
echo ""

# Pilot nội bộ — polyglot đầy đủ (test-harbor)
echo "── Pilot: test-harbor dev ──"
if "${ROOT}/scripts/verify-polyglot-deploy.sh" test-harbor dev "${DOMAIN}"; then
  pass "test-harbor dev"
else
  fail "test-harbor dev"
fi
echo ""

# Console (DOMAIN = host Console, vd. platform.7mlabs.com)
echo "── Console ──"
CODE="$(curl -sS -o /dev/null -w '%{http_code}' --max-time 10 "https://${DOMAIN}/health" 2>/dev/null || echo 000)"
if [[ "${CODE}" == "200" ]]; then
  pass "${DOMAIN}/health → ${CODE}"
else
  fail "${DOMAIN}/health → ${CODE}"
fi

echo ""
if [[ "${FAIL}" -eq 0 ]]; then
  echo "════════════════════════════════════════"
  echo " OK — sẵn sàng test"
  echo " Console: https://${DOMAIN}"
  echo " Phase 1: https://back-front-demo-dev.${DOMAIN}/"
  echo " Phase 2: https://polyglot-demo-dev.${DOMAIN}/"
  echo " Pilot:   https://test-harbor-dev.${DOMAIN}/"
  echo "════════════════════════════════════════"
  exit 0
fi

echo "════════════════════════════════════════"
echo " FAIL — xem log trên"
echo "════════════════════════════════════════"
exit 1
