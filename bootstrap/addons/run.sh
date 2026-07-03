#!/usr/bin/env bash
# Addon bootstrap — Rancher, Harbor (sau core 00–08).
set -euo pipefail

BOOTSTRAP_DIR="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=/dev/null
source "${BOOTSTRAP_DIR}/lib/common.sh"
# shellcheck source=/dev/null
source "${BOOTSTRAP_DIR}/lib/addon-preflight.sh"

usage() {
  cat <<'EOF'
Addon bootstrap (pin version, preflight):

  ./bootstrap/addons/run.sh list       # trạng thái addon
  ./bootstrap/addons/run.sh check rancher   # chỉ kiểm tra RAM/disk/chart — không cài
  ./bootstrap/addons/run.sh check harbor
  ./bootstrap/addons/run.sh check argocd
  ./bootstrap/addons/run.sh rancher    # cài Rancher (+ backup)
  ./bootstrap/addons/run.sh harbor     # cài Harbor
  ./bootstrap/addons/run.sh argocd     # cài Argo CD
  ./bootstrap/addons/run.sh rancher --force

Bỏ qua kiểm tra tài nguyên: SKIP_RESOURCE_CHECK=1
Cài dù thiếu RAM/disk:      FORCE_RESOURCE=1

Hoặc entry ngắn:
  bash bootstrap/addons/install-rancher.sh
  bash bootstrap/addons/install-harbor.sh

Yêu cầu: core bootstrap xong (06 + 08).
EOF
}

list_addons() {
  for name in rancher harbor argocd; do
    if is_addon_done "$name"; then
      echo "[x] ${name}"
    else
      echo "[ ] ${name}"
    fi
  done
}

run_check() {
  local name="$1"
  # shellcheck source=/dev/null
  source "${BOOTSTRAP_DIR}/lib/addon-preflight.sh"
  addon_check_resources_only "$name"
}

run_addon() {
  local name="$1"
  local force="${2:-false}"
  case "$name" in
    rancher)
      if [[ "$force" == "true" ]]; then
        export FORCE_VERSION=1
      fi
      bash "${ADDONS_DIR}/install-rancher.sh"
      ;;
    harbor)
      if [[ "$force" == "true" ]]; then
        export FORCE_VERSION=1
      fi
      bash "${ADDONS_DIR}/install-harbor.sh"
      ;;
    argocd)
      if [[ "$force" == "true" ]]; then
        export FORCE_VERSION=1
      fi
      bash "${ADDONS_DIR}/install-argocd.sh"
      ;;
    *)
      echo "Addon không tồn tại: ${name} (rancher | harbor | argocd)" >&2
      exit 1
      ;;
  esac
}

cmd="${1:-}"
shift || true
force=false
[[ "${1:-}" == "--force" ]] && force=true

case "${cmd}" in
  list|ls)
    list_addons
    ;;
  check)
    name="${1:-}"
    [[ -z "$name" ]] && { echo "Dùng: ./bootstrap/addons/run.sh check rancher|harbor|argocd" >&2; exit 1; }
    run_check "$name"
    ;;
  rancher|harbor|argocd)
    run_addon "$cmd" "$force"
    ;;
  ""|help|-h|--help)
    usage
    ;;
  *)
    echo "Lệnh không hợp lệ: ${cmd}" >&2
    usage
    exit 1
    ;;
esac
