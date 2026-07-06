#!/usr/bin/env bash
# Phase 2 E2E: project polyglot fleet trên branch multi-polyglot-full.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=/dev/null
[[ -f "${ROOT_DIR}/config/env.sh" ]] && source "${ROOT_DIR}/config/env.sh"
# shellcheck source=/dev/null
[[ -f "${ROOT_DIR}/config/platform-auth.env" ]] && source "${ROOT_DIR}/config/platform-auth.env"

export KUBECONFIG="${KUBECONFIG:-${ROOT_DIR}/kubeconfig/rke2.yaml}"
export PATH="/var/lib/rancher/rke2/bin:${PATH}"

SLUG="${SLUG:-polyglot-demo}"
PROJECT_NAME="${PROJECT_NAME:-Polyglot Demo}"
GITHUB_OWNER="${GITHUB_OWNER:-huuthienit97}"
GITHUB_REPO="${GITHUB_REPO:-test-k8s}"
PILOT_BRANCH="${PILOT_BRANCH:-multi-polyglot-full}"
REGISTRY="${REGISTRY:-harbor}"
PLATFORM_URL="${PLATFORM_URL:-https://${PLATFORM_HOST:-platform.7mlabs.com}}"
PLATFORM_DOMAIN="${PLATFORM_DOMAIN:-platform.7mlabs.com}"
ADMIN_EMAIL="${PLATFORM_ADMIN_EMAIL:-admin@platform.local}"
ADMIN_PASSWORD="${PLATFORM_ADMIN_PASSWORD:?Thiếu PLATFORM_ADMIN_PASSWORD}"
POLL_SECS="${POLL_SECS:-900}"
COOKIE_JAR="$(mktemp).cookie"
trap 'rm -f "${COOKIE_JAR}"' EXIT

log() { echo "[$(date '+%H:%M:%S')] $*"; }

api() {
  curl -sk -b "${COOKIE_JAR}" -c "${COOKIE_JAR}" -X "$1" "${PLATFORM_URL}$2" \
    -H "Content-Type: application/json" "${@:3}"
}

login() {
  log "Đăng nhập Console…"
  api POST /api/v1/auth/login -d "{\"email\":\"${ADMIN_EMAIL}\",\"password\":\"${ADMIN_PASSWORD}\"}" >/dev/null
}

ensure_branch() {
  log "Kiểm tra branch ${PILOT_BRANCH} trên ${GITHUB_OWNER}/${GITHUB_REPO}…"
  local token
  token="$(kubectl -n platform exec statefulset/platform-postgresql -- psql -U platform -d platform -t -A -c \
    "SELECT access_token FROM user_github_tokens WHERE user_id=(SELECT id FROM users WHERE email='${ADMIN_EMAIL}' LIMIT 1);" | tr -d '\r\n')"
  if ! git ls-remote "https://x-access-token:${token}@github.com/${GITHUB_OWNER}/${GITHUB_REPO}.git" "refs/heads/${PILOT_BRANCH}" | grep -q "${PILOT_BRANCH}"; then
    echo "Branch ${PILOT_BRANCH} chưa có — tạo từ repo test-k8s polyglot pilot trước."
    exit 1
  fi
  log "✓ Branch ${PILOT_BRANCH} tồn tại"
}

delete_project_if_exists() {
  if api GET "/api/v1/projects/${SLUG}" | grep -q '"slug"'; then
    log "Xóa project cũ ${SLUG}…"
    api DELETE "/api/v1/projects/${SLUG}" -d '{"purge_k8s":true}' >/dev/null || true
    sleep 5
  fi
}

create_project() {
  log "Tạo project ${SLUG}…"
  local resp
  resp="$(api POST /api/v1/projects -d "{\"name\":\"${PROJECT_NAME}\",\"slug\":\"${SLUG}\",\"registry_provider\":\"${REGISTRY}\"}")"
  echo "${resp}" | grep -q '"slug"' || { echo "${resp}"; exit 1; }
}

github_setup() {
  log "Sync GitHub multi + services.yaml…"
  local resp
  resp="$(api POST "/api/v1/projects/${SLUG}/github/setup" -d "{
    \"owner\": \"${GITHUB_OWNER}\",
    \"repo\": \"${GITHUB_REPO}\",
    \"branch\": \"${PILOT_BRANCH}\",
    \"environment\": \"dev\",
    \"layout\": \"multi\",
    \"apply_repo_contract\": true
  }")"
  echo "${resp}" | grep -q '"status"' || { echo "setup failed: ${resp}"; exit 1; }
  log "✓ GitHub setup OK"
}

wait_deploy_success() {
  log "Đợi deploy dev success (max ${POLL_SECS}s)…"
  local i=0 max=$((POLL_SECS / 15))
  while [[ "${i}" -lt "${max}" ]]; do
    local act st
    act="$(api GET "/api/v1/projects/${SLUG}/deploy/activity?environment=dev&limit=1")"
    st="$(echo "${act}" | grep -o '"status":"[^"]*"' | head -1 | cut -d'"' -f4 || true)"
    if [[ "${st}" == "success" ]]; then
      log "✓ Deploy success"
      return 0
    fi
    if [[ "${st}" == "failed" ]]; then
      echo "${act}" | head -c 1200
      exit 1
    fi
    sleep 15
    i=$((i + 1))
    [[ $((i % 4)) -eq 0 ]] && log "  … ${st:-pending}"
  done
  exit 1
}

main() {
  log "=== Phase 2 polyglot E2E · ${SLUG} · ${PILOT_BRANCH} ==="
  ensure_branch
  login
  delete_project_if_exists
  create_project
  github_setup
  wait_deploy_success
  "${ROOT_DIR}/scripts/verify-polyglot-deploy.sh" "${SLUG}" dev "${PLATFORM_DOMAIN}"
  log "=== DONE Phase 2 pilot: https://${SLUG}-dev.${PLATFORM_DOMAIN}/ ==="
}

main "$@"
