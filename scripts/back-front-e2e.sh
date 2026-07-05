#!/usr/bin/env bash
# Phase 1 E2E: push branch back-front-pilot → project Console mới → dev success.
# Chạy trên VPS (có KUBECONFIG + admin GitHub đã kết nối).
#
# Usage:
#   ./scripts/back-front-e2e.sh
#   SLUG=my-demo GITHUB_REPO=test-k8s ./scripts/back-front-e2e.sh
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=/dev/null
[[ -f "${ROOT_DIR}/config/env.sh" ]] && source "${ROOT_DIR}/config/env.sh"
# shellcheck source=/dev/null
[[ -f "${ROOT_DIR}/config/platform-auth.env" ]] && source "${ROOT_DIR}/config/platform-auth.env"

export KUBECONFIG="${KUBECONFIG:-${ROOT_DIR}/kubeconfig/rke2.yaml}"
export PATH="/var/lib/rancher/rke2/bin:${PATH}"

SLUG="${SLUG:-back-front-demo}"
PROJECT_NAME="${PROJECT_NAME:-Back Front Demo}"
GITHUB_OWNER="${GITHUB_OWNER:-huuthienit97}"
GITHUB_REPO="${GITHUB_REPO:-test-k8s}"
PILOT_BRANCH="${PILOT_BRANCH:-back-front-pilot}"
REGISTRY="${REGISTRY:-harbor}"
PLATFORM_URL="${PLATFORM_URL:-https://${PLATFORM_HOST:-platform.7mlabs.com}}"
PLATFORM_DOMAIN="${PLATFORM_DOMAIN:-platform.7mlabs.com}"
ADMIN_EMAIL="${PLATFORM_ADMIN_EMAIL:-admin@platform.local}"
ADMIN_PASSWORD="${PLATFORM_ADMIN_PASSWORD:?Thiếu PLATFORM_ADMIN_PASSWORD trong config/platform-auth.env}"
POLL_SECS="${POLL_SECS:-600}"
COOKIE_JAR="$(mktemp).cookie"
CLONE_DIR="$(mktemp -d)"
trap 'rm -f "${COOKIE_JAR}"; rm -rf "${CLONE_DIR}"' EXIT

log() { echo "[$(date '+%H:%M:%S')] $*"; }

api() {
  local method="$1" path="$2"
  shift 2
  curl -sk -b "${COOKIE_JAR}" -c "${COOKIE_JAR}" -X "${method}" \
    "${PLATFORM_URL}${path}" \
    -H "Content-Type: application/json" \
    "$@"
}

login() {
  log "Đăng nhập Console (${ADMIN_EMAIL})…"
  api POST /api/v1/auth/login -d "{\"email\":\"${ADMIN_EMAIL}\",\"password\":\"${ADMIN_PASSWORD}\"}" >/dev/null
}

gh_token_from_db() {
  kubectl -n platform exec statefulset/platform-postgresql -- psql -U platform -d platform -t -A -c \
    "SELECT access_token FROM user_github_tokens WHERE user_id=(SELECT id FROM users WHERE email='${ADMIN_EMAIL}' LIMIT 1);" \
    | tr -d '\r\n'
}

push_pilot_branch() {
  log "Push branch ${PILOT_BRANCH} lên ${GITHUB_OWNER}/${GITHUB_REPO}…"
  local token
  token="$(gh_token_from_db)"
  if [[ -z "${token}" ]]; then
    echo "Admin chưa kết nối GitHub — đăng nhập Console → Deploy/Git → Kết nối GitHub"
    exit 1
  fi
  git clone --quiet "https://x-access-token:${token}@github.com/${GITHUB_OWNER}/${GITHUB_REPO}.git" "${CLONE_DIR}/repo"
  "${ROOT_DIR}/scripts/setup-back-front-pilot.sh" "${CLONE_DIR}/repo" "${PILOT_BRANCH}"
  git -C "${CLONE_DIR}/repo" push -u origin "${PILOT_BRANCH}" --force
  log "✓ Branch ${PILOT_BRANCH} trên GitHub"
}

delete_project_if_exists() {
  if api GET "/api/v1/projects/${SLUG}" | grep -q '"slug"'; then
    log "Xóa project cũ ${SLUG}…"
    api DELETE "/api/v1/projects/${SLUG}" -d '{"purge_k8s":true}' >/dev/null || true
    sleep 3
  fi
}

create_project() {
  log "Tạo project ${SLUG}…"
  local resp code
  resp="$(api POST /api/v1/projects -d "{
    \"name\": \"${PROJECT_NAME}\",
    \"slug\": \"${SLUG}\",
    \"registry_provider\": \"${REGISTRY}\"
  }")"
  if echo "${resp}" | grep -q '"slug"'; then
    log "✓ Project created: ${SLUG}"
    return 0
  fi
  if echo "${resp}" | grep -q 'đã tồn tại'; then
    log "Project ${SLUG} đã tồn tại — dùng lại"
    return 0
  fi
  echo "${resp}"
  exit 1
}

github_setup() {
  log "Lưu & đồng bộ GitHub (multi + services.yaml)…"
  local resp
  resp="$(api POST "/api/v1/projects/${SLUG}/github/setup" -d "{
    \"owner\": \"${GITHUB_OWNER}\",
    \"repo\": \"${GITHUB_REPO}\",
    \"branch\": \"${PILOT_BRANCH}\",
    \"environment\": \"dev\",
    \"layout\": \"multi\",
    \"apply_repo_contract\": true
  }")"
  if echo "${resp}" | grep -q '"status"'; then
    log "✓ GitHub setup OK"
    echo "${resp}" | head -c 400
    echo
    return 0
  fi
  echo "GitHub setup failed: ${resp}"
  exit 1
}

wait_deploy_success() {
  log "Đợi deploy dev success (tối đa ${POLL_SECS}s)…"
  local i=0 max=$((POLL_SECS / 10))
  while [[ "${i}" -lt "${max}" ]]; do
    local act status tag
    act="$(api GET "/api/v1/projects/${SLUG}/deploy/activity?environment=dev&limit=3")"
    status="$(echo "${act}" | grep -o '"status":"[^"]*"' | head -1 | cut -d'"' -f4 || true)"
    tag="$(echo "${act}" | grep -o '"image_tag":"[^"]*"' | head -1 | cut -d'"' -f4 || true)"
    if [[ "${status}" == "success" && -n "${tag}" ]]; then
      log "✓ Deploy success tag=${tag:0:12}"
      return 0
    fi
    if [[ "${status}" == "failed" ]]; then
      echo "Deploy failed: ${act}" | head -c 800
      exit 1
    fi
    sleep 10
    i=$((i + 1))
    [[ $((i % 3)) -eq 0 ]] && log "  … đang chờ (status=${status:-pending})"
  done
  echo "Timeout — activity: $(api GET "/api/v1/projects/${SLUG}/deploy/activity?environment=dev&limit=1")"
  exit 1
}

verify_cluster() {
  log "Verify HTTP + pods…"
  "${ROOT_DIR}/scripts/verify-back-front-deploy.sh" "${SLUG}" dev "${PLATFORM_DOMAIN}"
  export PATH="/var/lib/rancher/rke2/bin:${PATH}"
  kubectl get deploy,pods -n "${SLUG}-dev" 2>/dev/null || true
}

main() {
  log "=== Phase 1 E2E back-front · slug=${SLUG} · branch=${PILOT_BRANCH} ==="
  push_pilot_branch
  login
  delete_project_if_exists
  create_project
  github_setup
  wait_deploy_success
  verify_cluster
  log "=== DONE — Phase 1 pilot pass: https://${SLUG}-dev.${PLATFORM_DOMAIN}/ ==="
}

main "$@"
