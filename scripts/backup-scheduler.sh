#!/usr/bin/env bash
# Scheduler offsite: chạy mỗi phút từ cron host, enqueue run scheduled theo lịch target,
# rồi gọi worker xử lý tuần tự. Không tự chạy target chưa Test thành công.
set -euo pipefail

ROOT_DIR="${ROOT_DIR:-$(cd "$(dirname "$0")/.." && pwd)}"
KUBECTL="${KUBECTL:-/var/lib/rancher/rke2/bin/kubectl}"
NS="platform"
PG_POD=""
POSTGRES_DB=""
POSTGRES_USER=""

log() { echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*"; }

sql() {
  "${KUBECTL}" -n "${NS}" exec "${PG_POD}" -- \
    psql -v ON_ERROR_STOP=1 -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" -Atqc "$1"
}

sql_literal() {
  printf "'%s'" "$(sed "s/'/''/g" <<<"$1")"
}

minute_matches_field() {
  local value="$1" field="$2" min="$3" max="$4"
  local item start end step
  IFS=',' read -ra items <<<"${field}"
  for item in "${items[@]}"; do
    step=1
    if [[ "${item}" == */* ]]; then
      step="${item#*/}"
      item="${item%%/*}"
    fi
    [[ "${step}" =~ ^[1-9][0-9]*$ ]] || continue
    if [[ "${item}" == "*" ]]; then
      start="${min}"; end="${max}"
    elif [[ "${item}" =~ ^[0-9]+-[0-9]+$ ]]; then
      start="${item%-*}"; end="${item#*-}"
    elif [[ "${item}" =~ ^[0-9]+$ ]]; then
      start="${item}"; end="${item}"
    else
      continue
    fi
    (( start >= min && end <= max && start <= end )) || continue
    (( (value - start) % step == 0 && value >= start && value <= end )) && return 0
  done
  return 1
}

# Cron chuẩn Vixie 5 fields. DOW: 0/7=Chủ nhật. Nếu cả DOM và DOW không wildcard, cron
# thực thi khi MỘT trong hai khớp; các trường còn lại luôn theo AND.
cron_matches_now() {
  local expr="$1" minute hour dom month dow extra
  read -r minute hour dom month dow extra <<<"${expr}"
  [[ -n "${dow:-}" && -z "${extra:-}" ]] || return 1
  local m h d mo w
  m="$(date +%-M)"; h="$(date +%-H)"; d="$(date +%-d)"; mo="$(date +%-m)"; w="$(date +%w)"
  minute_matches_field "${m}" "${minute}" 0 59 &&
    minute_matches_field "${h}" "${hour}" 0 23 &&
    minute_matches_field "${mo}" "${month}" 1 12 || return 1
  local dom_match=1 dow_match=1
  [[ "${dom}" == "*" ]] || { minute_matches_field "${d}" "${dom}" 1 31; dom_match=$?; }
  [[ "${dow}" == "*" ]] || {
    minute_matches_field "${w}" "${dow//7/0}" 0 6
    dow_match=$?
  }
  if [[ "${dom}" != "*" && "${dow}" != "*" ]]; then
    (( dom_match == 0 || dow_match == 0 ))
  else
    (( dom_match == 0 && dow_match == 0 ))
  fi
}

"${KUBECTL}" -n "${NS}" get statefulset platform-postgresql >/dev/null
PG_POD="$("${KUBECTL}" -n "${NS}" get pod -l app=platform-postgresql -o jsonpath='{.items[0].metadata.name}')"
[[ -n "${PG_POD}" ]] || { log "Không tìm thấy PostgreSQL pod."; exit 1; }
POSTGRES_DB="$("${KUBECTL}" -n "${NS}" get secret platform-postgresql-auth -o jsonpath='{.data.POSTGRES_DB}' | base64 -d)"
POSTGRES_USER="$("${KUBECTL}" -n "${NS}" get secret platform-postgresql-auth -o jsonpath='{.data.POSTGRES_USER}' | base64 -d)"

while IFS=$'\t' read -r target_id name schedule; do
  [[ -n "${target_id}" ]] || continue
  if ! cron_matches_now "${schedule}"; then
    continue
  fi
  # Unique theo target/phút bằng NOT EXISTS: scheduler chạy đúp hay cron restart vẫn không tạo run đôi.
  stamp="$(date -u '+%Y%m%dT%H%MZ')"
  prefix="$(sql "SELECT prefix FROM backup_targets WHERE id=${target_id}")/runs/${stamp}"
  sql "INSERT INTO backup_runs (target_id,run_kind,status,run_prefix)
       SELECT ${target_id},'scheduled','queued',$(sql_literal "${prefix}")
       WHERE NOT EXISTS (
         SELECT 1 FROM backup_runs
         WHERE target_id=${target_id} AND run_kind='scheduled'
           AND date_trunc('minute',created_at)=date_trunc('minute',now())
       )" >/dev/null
  log "Target ${name}: schedule khớp ${schedule}, đã enqueue nếu chưa có run phút này."
done < <(sql "SELECT id,name,schedule_cron FROM backup_targets WHERE enabled=true AND last_tested_at IS NOT NULL AND last_test_error IS NULL ORDER BY id")

# Worker claim atomic một run; gọi nhiều lần vẫn chỉ có một run đang xử lý.
"${ROOT_DIR}/scripts/backup-offsite-worker.sh"
