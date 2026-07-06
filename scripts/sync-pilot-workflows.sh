#!/usr/bin/env bash
# Đồng bộ workflow mới (path-filter, probe) cho các project pilot trên VPS.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=/dev/null
[[ -f "${ROOT_DIR}/config/platform-auth.env" ]] && source "${ROOT_DIR}/config/platform-auth.env"
# shellcheck source=/dev/null
[[ -f "${ROOT_DIR}/config/env.sh" ]] && source "${ROOT_DIR}/config/env.sh"

PLATFORM_URL="${PLATFORM_URL:-https://${PLATFORM_HOST:-platform.7mlabs.com}}"
ADMIN_EMAIL="${PLATFORM_ADMIN_EMAIL:-admin@platform.local}"
ADMIN_PASSWORD="${PLATFORM_ADMIN_PASSWORD:?Thiếu PLATFORM_ADMIN_PASSWORD}"

COOKIE="$(mktemp).cookie"
trap 'rm -f "${COOKIE}"' EXIT

curl -sk -c "${COOKIE}" -X POST "${PLATFORM_URL}/api/v1/auth/login" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"${ADMIN_EMAIL}\",\"password\":\"${ADMIN_PASSWORD}\"}" >/dev/null

sync_project() {
  local slug="$1" owner="$2" repo="$3" branch="$4"
  echo "→ sync ${slug} (${owner}/${repo}@${branch})"
  local resp
  resp="$(curl -sk -b "${COOKIE}" -X POST "${PLATFORM_URL}/api/v1/projects/${slug}/github/setup" \
    -H "Content-Type: application/json" \
    -d "{\"owner\":\"${owner}\",\"repo\":\"${repo}\",\"branch\":\"${branch}\",\"environment\":\"dev\",\"layout\":\"multi\",\"apply_repo_contract\":true}")"
  if echo "${resp}" | grep -q '"status"'; then
    echo "  ✓ OK"
  else
    echo "  ✗ ${resp}" | head -c 300
    return 1
  fi
}

echo "=== Sync workflow pilot projects ==="
sync_project back-front-demo huuthienit97 test-k8s back-front-pilot
sync_project polyglot-demo huuthienit97 test-k8s multi-polyglot-full
sync_project test-harbor huuthienit97 test-k8s multi-submodules
echo "=== Done ==="
