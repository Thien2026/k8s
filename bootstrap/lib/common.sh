#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BOOTSTRAP_DIR="${ROOT_DIR}/bootstrap"
STATE_DIR="${BOOTSTRAP_DIR}/state"
LOG_DIR="${BOOTSTRAP_DIR}/logs"

mkdir -p "${STATE_DIR}" "${LOG_DIR}"

if [[ -f "${ROOT_DIR}/config/env.sh" ]]; then
  # shellcheck source=/dev/null
  source "${ROOT_DIR}/config/env.sh"
else
  echo "Thiếu config/env.sh — copy từ config/env.sh.example rồi sửa."
  exit 1
fi

: "${DOMAIN:?DOMAIN chưa set trong config/env.sh}"
: "${LETSENCRYPT_EMAIL:?LETSENCRYPT_EMAIL chưa set}"

# RKE2 cài kubectl tại đây, chưa có trong PATH mặc định
export PATH="/var/lib/rancher/rke2/bin:/usr/local/bin:${PATH}"

log() {
  echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*"
}

step_id() {
  basename "$1" .sh
}

step_done_file() {
  echo "${STATE_DIR}/$(step_id "$1").done"
}

is_step_done() {
  [[ -f "$(step_done_file "$1")" ]]
}

mark_step_done() {
  date -Iseconds > "$(step_done_file "$1")"
  log "✓ Đánh dấu xong: $(step_id "$1")"
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    log "Thiếu lệnh: $1"
    exit 1
  }
}

kube_ready() {
  export KUBECONFIG="${KUBECONFIG:-${ROOT_DIR}/kubeconfig/rke2.yaml}"
  export PATH="/var/lib/rancher/rke2/bin:${PATH}"
  kubectl get nodes >/dev/null 2>&1
}

helm_ready() {
  require_cmd helm
  helm version >/dev/null 2>&1
}
