#!/usr/bin/env bash
# Hoàn tất first-login Rancher qua API (không cần UI) — chạy trên VPS.
set -euo pipefail
source "$(dirname "$0")/../../lib/common.sh"

export KUBECONFIG="${ROOT_DIR}/kubeconfig/rke2.yaml"
kube_ready

: "${RANCHER_HOST:?}"
RANCHER_URL="https://${RANCHER_HOST}"
RANCHER_ENV="${ROOT_DIR}/config/rancher.env"
ADMIN_ENV="${ROOT_DIR}/config/rancher-admin.env"

if [[ -f "${RANCHER_ENV}" ]] && grep -q 'RANCHER_TOKEN=token:' "${RANCHER_ENV}" 2>/dev/null; then
  log "RANCHER_TOKEN đã có trong config/rancher.env — bỏ qua."
  exit 0
fi

BOOTSTRAP_PW="$(kubectl -n cattle-system get secret bootstrap-secret \
  -o go-template='{{.data.bootstrapPassword|base64decode}}')"

FIRST_LOGIN="$(curl -sk "${RANCHER_URL}/v3/settings/first-login" | sed -n 's/.*"value":"\([^"]*\)".*/\1/p')"
if [[ "${FIRST_LOGIN}" != "true" ]]; then
  log "first-login=false — Rancher đã setup. Tạo API token thủ công trong UI hoặc cập nhật rancher.env."
  exit 0
fi

# Mật khẩu admin mới (lưu file local, không commit)
if [[ -f "${ADMIN_ENV}" ]]; then
  # shellcheck source=/dev/null
  source "${ADMIN_ENV}"
fi
ADMIN_PASSWORD="${RANCHER_ADMIN_PASSWORD:-$(openssl rand -base64 24 | tr -d '/+=' | head -c 24)}"

log "Đăng nhập bootstrap qua API..."
LOGIN_JSON="$(curl -sk -H 'content-type: application/json' \
  -d "{\"username\":\"admin\",\"password\":\"${BOOTSTRAP_PW}\",\"responseType\":\"json\"}" \
  "${RANCHER_URL}/v3-public/localProviders/local?action=login")"

TOKEN="$(echo "${LOGIN_JSON}" | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')"
if [[ -z "${TOKEN}" ]]; then
  err "Login bootstrap thất bại: ${LOGIN_JSON}"
fi

log "Đổi mật khẩu admin..."
curl -sk -u "${TOKEN}" -H 'content-type: application/json' \
  -d "{\"currentPassword\":\"${BOOTSTRAP_PW}\",\"newPassword\":\"${ADMIN_PASSWORD}\"}" \
  "${RANCHER_URL}/v3/users?action=changepassword" >/dev/null

log "Đăng nhập lại với mật khẩu mới..."
LOGIN_JSON="$(curl -sk -H 'content-type: application/json' \
  -d "{\"username\":\"admin\",\"password\":\"${ADMIN_PASSWORD}\",\"responseType\":\"json\"}" \
  "${RANCHER_URL}/v3-public/localProviders/local?action=login")"
TOKEN="$(echo "${LOGIN_JSON}" | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')"

log "Tạo API token cho portal-api..."
TOKEN_JSON="$(curl -sk -u "${TOKEN}" -H 'content-type: application/json' \
  -d '{"type":"token","description":"platform-portal","ttl":0}' \
  "${RANCHER_URL}/v3/tokens")"
API_TOKEN="$(echo "${TOKEN_JSON}" | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')"
if [[ -z "${API_TOKEN}" ]]; then
  err "Tạo API token thất bại: ${TOKEN_JSON}"
fi

cat > "${RANCHER_ENV}" <<EOF
RANCHER_URL="${RANCHER_URL}"
RANCHER_TOKEN="${API_TOKEN}"
EOF
chmod 600 "${RANCHER_ENV}"

cat > "${ADMIN_ENV}" <<EOF
RANCHER_ADMIN_PASSWORD="${ADMIN_PASSWORD}"
EOF
chmod 600 "${ADMIN_ENV}"

log "Đã ghi config/rancher.env và config/rancher-admin.env"
log "Admin: https://${RANCHER_HOST}/dashboard/auth/login (user: admin)"
log "Mật khẩu admin: xem config/rancher-admin.env trên VPS"
