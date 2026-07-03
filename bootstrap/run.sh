#!/usr/bin/env bash
# Core bootstrap — K8s + Console (00–08). Addon: ./bootstrap/addons/run.sh
set -euo pipefail

BOOTSTRAP_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=/dev/null
source "${BOOTSTRAP_DIR}/lib/common.sh"
# shellcheck source=/dev/null
source "${BOOTSTRAP_DIR}/lib/runner.sh"

STEPS_DIR="${CORE_STEPS_DIR}"

usage() {
  cat <<'EOF'
Core bootstrap (K8s + Platform Console):

  ./bootstrap/run.sh list          # xem bước core
  ./bootstrap/run.sh next          # chạy bước core tiếp theo
  ./bootstrap/run.sh 08            # chạy 1 bước
  ./bootstrap/run.sh 08 --force    # chạy lại

Addon (Rancher, Harbor…):
  ./bootstrap/addons/run.sh list
  ./bootstrap/addons/run.sh rancher
  ./bootstrap/addons/run.sh harbor
  ./bootstrap/addons/run.sh argocd

SSH rớt → tmux:
  tmux new -s k8s
  cd ~/k8s && ./bootstrap/run.sh next

Log: bootstrap/logs/
EOF
}

cmd="${1:-}"
shift || true

case "${cmd}" in
  list|ls)
    echo "# Core (bootstrap/core/steps/)"
    bootstrap_run_list "${STEPS_DIR}"
    echo ""
    echo "# Addon — ./bootstrap/addons/run.sh list"
    ;;
  next)
    bootstrap_run_next "${STEPS_DIR}" "${1:-}"
    ;;
  ""|help|-h|--help)
    usage
    ;;
  *)
    force=false
    if [[ "${1:-}" == "--force" || "${2:-}" == "--force" ]]; then
      force=true
    fi
    bootstrap_run_step "$(bootstrap_resolve_step "${STEPS_DIR}" "$cmd")" "$force"
    ;;
esac
