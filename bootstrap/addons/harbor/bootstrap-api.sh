#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/../../lib/common.sh"

export KUBECONFIG="${ROOT_DIR}/kubeconfig/rke2.yaml"
kube_ready

HARBOR_ENV="${ROOT_DIR}/config/harbor.env"
[[ -f "${HARBOR_ENV}" ]] || { log "Thiếu ${HARBOR_ENV} — chạy ./bootstrap/addons/run.sh harbor trước"; exit 1; }
# shellcheck source=/dev/null
source "${HARBOR_ENV}"

: "${HARBOR_URL:?}"
: "${HARBOR_ADMIN_PASSWORD:?}"
PROJECT="${HARBOR_PROJECT:-demo}"

log "Harbor API bootstrap — project ${PROJECT}"

# Đợi API sẵn sàng
for i in $(seq 1 60); do
  if curl -sk -o /dev/null -w "%{http_code}" -u "admin:${HARBOR_ADMIN_PASSWORD}" \
    "${HARBOR_URL}/api/v2.0/projects?page=1&page_size=1" | grep -qE '^(200|401)$'; then
    break
  fi
  sleep 10
done

auth=(-u "admin:${HARBOR_ADMIN_PASSWORD}")

# Tạo project public demo
if ! curl -sk "${auth[@]}" "${HARBOR_URL}/api/v2.0/projects?project_name=${PROJECT}" | grep -q "\"name\":\"${PROJECT}\""; then
  curl -sk "${auth[@]}" -X POST "${HARBOR_URL}/api/v2.0/projects" \
    -H "Content-Type: application/json" \
    -d "{\"project_name\":\"${PROJECT}\",\"metadata\":{\"public\":\"true\"}}" >/dev/null
  log "Đã tạo project: ${PROJECT}"
else
  log "Project ${PROJECT} đã tồn tại"
fi

# Robot account cho CI
ROBOT_NAME="ci-${PROJECT}"
ROBOT_PAYLOAD=$(cat <<EOF
{
  "name": "${ROBOT_NAME}",
  "duration": -1,
  "level": "project",
  "disable": false,
  "permissions": [
    {"resource": "/project/${PROJECT}/repository", "action": "push"},
    {"resource": "/project/${PROJECT}/repository", "action": "pull"}
  ]
}
EOF
)

ROBOT_RESP="$(curl -sk "${auth[@]}" -X POST "${HARBOR_URL}/api/v2.0/robots" \
  -H "Content-Type: application/json" \
  -d "${ROBOT_PAYLOAD}" 2>/dev/null || true)"

ROBOT_USER=""
ROBOT_SECRET=""
if command -v python3 >/dev/null 2>&1 && [[ -n "${ROBOT_RESP}" ]]; then
  ROBOT_USER="$(echo "${ROBOT_RESP}" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('name',''))" 2>/dev/null || true)"
  ROBOT_SECRET="$(echo "${ROBOT_RESP}" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('secret',''))" 2>/dev/null || true)"
fi

if [[ -n "${ROBOT_USER}" && -n "${ROBOT_SECRET}" ]]; then
  {
    echo "HARBOR_ROBOT_NAME=${ROBOT_USER}"
    echo "HARBOR_ROBOT_SECRET=${ROBOT_SECRET}"
  } >>"${HARBOR_ENV}"
  chmod 600 "${HARBOR_ENV}"
  log "Robot CI: ${ROBOT_USER} (secret trong ${HARBOR_ENV})"
else
  log "Robot account — tạo thủ công trong UI nếu cần"
fi

mark_step_done "$0"
