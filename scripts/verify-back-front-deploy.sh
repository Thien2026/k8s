#!/usr/bin/env bash
# Smoke test deploy back-front (api + web) — chạy sau push dev/prod.
# Usage: ./scripts/verify-back-front-deploy.sh PROJECT_SLUG [dev|prod] [BASE_DOMAIN]
set -euo pipefail

SLUG="${1:-}"
ENV="${2:-dev}"
DOMAIN="${3:-platform.7mlabs.com}"

if [[ -z "${SLUG}" ]]; then
  echo "Usage: $0 PROJECT_SLUG [dev|prod] [BASE_DOMAIN]"
  echo "  Ví dụ: $0 my-shop dev"
  echo "         $0 my-shop prod platform.7mlabs.com"
  exit 1
fi

ENV="$(echo "${ENV}" | tr '[:upper:]' '[:lower:]')"
if [[ "${ENV}" != "dev" && "${ENV}" != "prod" ]]; then
  echo "environment phải là dev hoặc prod"
  exit 1
fi

BASE="https://${SLUG}-${ENV}.${DOMAIN}"
FAIL=0

check() {
  local label="$1"
  local url="$2"
  local expect="$3"
  local body code
  code="$(curl -sS -o /tmp/verify-bf-body.txt -w '%{http_code}' --max-time 20 "${url}" 2>/dev/null || echo 000)"
  body="$(cat /tmp/verify-bf-body.txt 2>/dev/null || true)"
  if [[ "${code}" != "200" ]]; then
    echo "✗ ${label}: HTTP ${code} — ${url}"
    FAIL=1
    return
  fi
  if [[ -n "${expect}" ]] && ! grep -q "${expect}" <<<"${body}"; then
    echo "✗ ${label}: HTTP 200 nhưng thiếu «${expect}» — ${url}"
    echo "  body: ${body:0:120}"
    FAIL=1
    return
  fi
  echo "✓ ${label}: ${url}"
}

echo "=== verify back-front · ${SLUG} · ${ENV} ==="
echo "Base: ${BASE}"
echo ""

check "Web /" "${BASE}/" ""
check "API /api/health" "${BASE}/api/health" '"status":"ok"'

rm -f /tmp/verify-bf-body.txt

if [[ "${FAIL}" -ne 0 ]]; then
  echo ""
  echo "FAIL — xem docs/KHACH_DEPLOY.md (bước 3 sync workflow, bước 2 Web + API riêng)"
  exit 1
fi

echo ""
echo "OK — dev/prod phản hồi đúng cho micro thường api+web"
