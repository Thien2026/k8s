#!/usr/bin/env bash
set -euo pipefail

BOOTSTRAP_DIR="$(cd "$(dirname "$0")" && pwd)"
STEPS_DIR="${BOOTSTRAP_DIR}/steps"
LOG_DIR="${BOOTSTRAP_DIR}/logs"

usage() {
  cat <<'EOF'
Cách dùng:
  ./bootstrap/run.sh list          # xem các bước
  ./bootstrap/run.sh next          # chạy bước tiếp theo chưa xong
  ./bootstrap/run.sh 04            # chạy 1 bước (số hoặc tên file)
  ./bootstrap/run.sh 04 --force    # chạy lại dù đã xong

SSH hay rớt → dùng tmux:
  tmux new -s k8s
  cd /path/to/k8s && ./bootstrap/run.sh next

Log mỗi lần chạy: bootstrap/logs/
EOF
}

list_steps() {
  for f in "${STEPS_DIR}"/*.sh; do
    [[ -f "$f" ]] || continue
    base="$(basename "$f" .sh)"
    if [[ -f "${BOOTSTRAP_DIR}/state/${base}.done" ]]; then
      echo "[x] ${base}"
    else
      echo "[ ] ${base}"
    fi
  done
}

resolve_step() {
  local arg="$1"
  if [[ -f "${STEPS_DIR}/${arg}.sh" ]]; then
    echo "${STEPS_DIR}/${arg}.sh"
    return
  fi
  local match
  match="$(find "${STEPS_DIR}" -maxdepth 1 -name "${arg}-*.sh" -o -name "${arg}_*.sh" 2>/dev/null | head -1)"
  if [[ -n "${match}" && -f "${match}" ]]; then
    echo "${match}"
    return
  fi
  match="$(find "${STEPS_DIR}" -maxdepth 1 -name "*${arg}*.sh" 2>/dev/null | head -1)"
  if [[ -n "${match}" && -f "${match}" ]]; then
    echo "${match}"
    return
  fi
  echo "Không tìm thấy bước: ${arg}" >&2
  exit 1
}

run_step() {
  local script="$1"
  local force="${2:-false}"
  local base
  base="$(basename "$script" .sh)"
  local logfile="${LOG_DIR}/${base}-$(date '+%Y%m%d-%H%M%S').log"

  if [[ "${force}" != "true" && -f "${BOOTSTRAP_DIR}/state/${base}.done" ]]; then
    echo "Bỏ qua ${base} (đã xong). Dùng --force để chạy lại."
    return 0
  fi

  echo "Chạy: ${base}"
  echo "Log:  ${logfile}"
  echo "---"

  set +e
  bash "$script" 2>&1 | tee "$logfile"
  local rc=${PIPESTATUS[0]}
  set -e

  if [[ $rc -eq 0 ]]; then
    date -Iseconds > "${BOOTSTRAP_DIR}/state/${base}.done"
    echo "✓ Xong ${base}"
  else
    echo "✗ Lỗi ${base} (exit ${rc}) — xem log: ${logfile}"
    exit $rc
  fi
}

cmd="${1:-}"
shift || true

case "${cmd}" in
  list|ls)
    list_steps
    ;;
  next)
    for f in "${STEPS_DIR}"/*.sh; do
      [[ -f "$f" ]] || continue
      base="$(basename "$f" .sh)"
      if [[ ! -f "${BOOTSTRAP_DIR}/state/${base}.done" ]]; then
        force=false
        [[ "${1:-}" == "--force" ]] && force=true
        run_step "$f" "$force"
        exit 0
      fi
    done
    echo "Tất cả bước đã xong."
    ;;
  ""|help|-h|--help)
    usage
    ;;
  *)
    force=false
    if [[ "${2:-}" == "--force" ]]; then
      force=true
    fi
    run_step "$(resolve_step "$cmd")" "$force"
    ;;
esac
