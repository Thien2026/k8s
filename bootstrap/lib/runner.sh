#!/usr/bin/env bash
# Shared step runner — core và addons dùng chung.
set -euo pipefail

bootstrap_run_list() {
  local steps_dir="$1"
  local label="${2:-}"
  for f in "${steps_dir}"/*.sh; do
    [[ -f "$f" ]] || continue
    local base
    base="$(basename "$f" .sh)"
    if [[ -f "${BOOTSTRAP_DIR}/state/${base}.done" ]]; then
      echo "[x] ${label}${base}"
    else
      echo "[ ] ${label}${base}"
    fi
  done
}

bootstrap_resolve_step() {
  local steps_dir="$1"
  local arg="$2"
  if [[ -f "${steps_dir}/${arg}.sh" ]]; then
    echo "${steps_dir}/${arg}.sh"
    return
  fi
  local match
  match="$(find "${steps_dir}" -maxdepth 1 -name "${arg}-*.sh" -o -name "${arg}_*.sh" 2>/dev/null | head -1)"
  if [[ -n "${match}" && -f "${match}" ]]; then
    echo "${match}"
    return
  fi
  match="$(find "${steps_dir}" -maxdepth 1 -name "*${arg}*.sh" 2>/dev/null | head -1)"
  if [[ -n "${match}" && -f "${match}" ]]; then
    echo "${match}"
    return
  fi
  echo "Không tìm thấy bước: ${arg}" >&2
  exit 1
}

bootstrap_run_step() {
  local script="$1"
  local force="${2:-false}"
  local base
  base="$(basename "$script" .sh)"
  local logfile="${LOG_DIR}/${base}-$(date '+%Y%m%d-%H%M%S').log"

  if [[ "${force}" != "true" && -f "${BOOTSTRAP_DIR}/state/${base}.done" ]]; then
    echo "Bỏ qua ${base} (đã xong). Dùng --force để chạy lại."
    return 0
  fi

  if [[ "${force}" == "true" ]]; then
    export FORCE_BUILD=1
  fi

  echo "Chạy: ${base}"
  echo "Log:  ${logfile}"
  echo "---"

  set +e
  bash "$script" 2>&1 | tee "$logfile"
  local rc=${PIPESTATUS[0]}
  set -e

  if [[ $rc -eq 0 ]]; then
    if [[ ! -f "${BOOTSTRAP_DIR}/state/${base}.done" ]]; then
      date -Iseconds > "${BOOTSTRAP_DIR}/state/${base}.done"
    fi
    echo "✓ Xong ${base}"
  else
    echo "✗ Lỗi ${base} (exit ${rc}) — xem log: ${logfile}"
    exit $rc
  fi
}

bootstrap_run_next() {
  local steps_dir="$1"
  shift
  local force=false
  [[ "${1:-}" == "--force" ]] && force=true
  for f in "${steps_dir}"/*.sh; do
    [[ -f "$f" ]] || continue
    local base
    base="$(basename "$f" .sh)"
    if [[ ! -f "${BOOTSTRAP_DIR}/state/${base}.done" ]]; then
      bootstrap_run_step "$f" "$force"
      return 0
    fi
  done
  echo "Tất cả bước đã xong."
}
