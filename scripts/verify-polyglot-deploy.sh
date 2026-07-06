#!/usr/bin/env bash
# Smoke test polyglot fleet — public ingress + pod count.
# Usage: ./scripts/verify-polyglot-deploy.sh PROJECT_SLUG [dev|prod] [BASE_DOMAIN]
set -euo pipefail

SLUG="${1:-}"
ENV="${2:-dev}"
DOMAIN="${3:-platform.7mlabs.com}"

if [[ -z "${SLUG}" ]]; then
  echo "Usage: $0 PROJECT_SLUG [dev|prod] [BASE_DOMAIN]"
  exit 1
fi

ENV="$(echo "${ENV}" | tr '[:upper:]' '[:lower:]')"
BASE="https://${SLUG}-${ENV}.${DOMAIN}"
FAIL=0

check_url() {
  local label="$1" url="$2" expect="${3:-}"
  local code body
  code="$(curl -sS -o /tmp/vpoly.txt -w '%{http_code}' --max-time 20 "${url}" 2>/dev/null || echo 000)"
  body="$(cat /tmp/vpoly.txt 2>/dev/null || true)"
  if [[ "${code}" != "200" ]]; then
    echo "✗ ${label}: HTTP ${code} — ${url}"
    FAIL=1
    return
  fi
  if [[ -n "${expect}" ]] && ! grep -q "${expect}" <<<"${body}"; then
    echo "✗ ${label}: thiếu «${expect}»"
    FAIL=1
    return
  fi
  echo "✓ ${label}: ${url}"
}

echo "=== verify polyglot · ${SLUG} · ${ENV} ==="
check_url "Web /" "${BASE}/" ""
check_url "API health" "${BASE}/api/health" '"status":"ok"'

  if command -v kubectl >/dev/null 2>&1 && [[ -n "${KUBECONFIG:-}" ]]; then
  NS="${SLUG}-${ENV}"
  echo ""
  echo "Pods (${NS}):"
  kubectl get pods -n "${NS}" 2>/dev/null || echo "(không đọc được kubectl)"
  if ! kubectl get pods -n "${NS}" --no-headers 2>/dev/null | awk '$2 !~ /^[0-9]+\/[0-9]+$/ { bad=1 } $2 ~ /^0\// { bad=1 } END { exit (bad ? 1 : 0) }'; then
    echo "✗ Có pod chưa Ready"
    FAIL=1
  else
    echo "✓ Mọi pod Ready"
  fi
fi

rm -f /tmp/vpoly.txt
[[ "${FAIL}" -eq 0 ]] || exit 1
echo "OK — polyglot dev phản hồi đúng"
