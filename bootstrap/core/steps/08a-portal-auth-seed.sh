#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/../../lib/common.sh"

export KUBECONFIG="${ROOT_DIR}/kubeconfig/rke2.yaml"
kube_ready

if [[ -f "${ROOT_DIR}/config/postgres.env" ]]; then
  # shellcheck source=/dev/null
  source "${ROOT_DIR}/config/postgres.env"
fi
: "${DATABASE_URL:?Thiếu DATABASE_URL — chạy bước 07 trước}"

AUTH_ENV="${ROOT_DIR}/config/platform-auth.env"
if [[ -f "${AUTH_ENV}" ]]; then
  # shellcheck source=/dev/null
  source "${AUTH_ENV}"
fi

if [[ -z "${JWT_SECRET:-}" ]]; then
  JWT_SECRET="$(openssl rand -base64 48 | tr -d '\n')"
  mkdir -p "${ROOT_DIR}/config"
  if [[ -f "${AUTH_ENV}" ]]; then
    if grep -q '^JWT_SECRET=' "${AUTH_ENV}"; then
      sed -i "s|^JWT_SECRET=.*|JWT_SECRET=\"${JWT_SECRET}\"|" "${AUTH_ENV}"
    else
      echo "JWT_SECRET=\"${JWT_SECRET}\"" >>"${AUTH_ENV}"
    fi
  else
    cp "${ROOT_DIR}/config/platform-auth.env.example" "${AUTH_ENV}"
    sed -i "s|^JWT_SECRET=.*|JWT_SECRET=\"${JWT_SECRET}\"|" "${AUTH_ENV}"
  fi
  chmod 600 "${AUTH_ENV}"
  log "Đã sinh JWT_SECRET → ${AUTH_ENV}"
fi

ADMIN_EMAIL="${PLATFORM_ADMIN_EMAIL:-admin@platform.local}"
ADMIN_PASS="${PLATFORM_ADMIN_PASSWORD:-}"
if [[ -z "${ADMIN_PASS}" ]]; then
  ADMIN_PASS="$(openssl rand -base64 18 | tr -d '/+=' | head -c 20)"
  if [[ -f "${AUTH_ENV}" ]]; then
    grep -q '^PLATFORM_ADMIN_PASSWORD=' "${AUTH_ENV}" && \
      sed -i "s|^PLATFORM_ADMIN_PASSWORD=.*|PLATFORM_ADMIN_PASSWORD=\"${ADMIN_PASS}\"|" "${AUTH_ENV}" || \
      echo "PLATFORM_ADMIN_PASSWORD=\"${ADMIN_PASS}\"" >>"${AUTH_ENV}"
  fi
fi

export DATABASE_URL JWT_SECRET
log "Seed admin: ${ADMIN_EMAIL}"

seed_user() {
  local email="$1" pass="$2" name="$3" role="$4"
  local pf_pid=""
  local db_url="${DATABASE_URL}"
  if [[ "${db_url}" == *".svc.cluster.local"* ]]; then
    log "Port-forward Postgres để seed user..."
    kubectl -n platform port-forward svc/platform-postgresql 15432:5432 >/dev/null 2>&1 &
    pf_pid=$!
    sleep 2
    db_url="$(echo "${DATABASE_URL}" | sed -E 's|@[^/]+|@127.0.0.1:15432|')"
  fi
  local rc=0
  docker run --rm --network host \
    -v "${ROOT_DIR}/services/portal-api:/src" -w /src \
    -e DATABASE_URL="${db_url}" \
    golang:1.23-alpine \
    sh -c "go run ./cmd/seed-admin --email '${email}' --password '${pass}' --name '${name}' --role '${role}'" || rc=$?
  [[ -n "${pf_pid}" ]] && kill "${pf_pid}" 2>/dev/null || true
  return "${rc}"
}

seed_user "${ADMIN_EMAIL}" "${ADMIN_PASS}" "${PLATFORM_ADMIN_NAME:-Platform Admin}" admin

if [[ -n "${PLATFORM_LEAD_EMAIL:-}" ]]; then
  LEAD_PASS="${PLATFORM_LEAD_PASSWORD:-$(openssl rand -base64 18 | tr -d '/+=' | head -c 20)}"
  seed_user "${PLATFORM_LEAD_EMAIL}" "${LEAD_PASS}" "${PLATFORM_LEAD_NAME:-Technical Lead}" tech_lead || true
fi

kubectl -n platform create secret generic platform-auth-keys \
  --from-literal=JWT_SECRET="${JWT_SECRET}" \
  --dry-run=client -o yaml | kubectl apply -f -

kubectl -n platform get secret portal-api-env -o json | \
  python3 -c "
import json,sys,os
s=json.load(sys.stdin)
d=dict(zip(s['data'].keys(),[__import__('base64').b64decode(s['data'][k]).decode() for k in s['data']]))
d['JWT_SECRET']=os.environ['JWT_SECRET']
d['COOKIE_SECURE']='true'
out={'apiVersion':'v1','kind':'Secret','metadata':s['metadata'],'type':'Opaque','stringData':d}
json.dump(out,sys.stdout)
" | kubectl apply -f -
kubectl -n platform rollout restart deploy/portal-api
kubectl -n platform rollout status deploy/portal-api --timeout=180s

log "Mật khẩu admin: xem ${AUTH_ENV}"
mark_step_done "$0"
