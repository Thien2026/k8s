#!/usr/bin/env bash
# Đồng bộ RKE2 join token vào K8s secret (portal-api đọc qua volume — không commit).
set -euo pipefail
source "$(dirname "$0")/../../lib/common.sh"

export KUBECONFIG="${ROOT_DIR}/kubeconfig/rke2.yaml"
kube_ready

NS="platform"
TOKEN_FILE="/var/lib/rancher/rke2/server/node-token"

if [[ ! -f "${TOKEN_FILE}" ]]; then
  log "Chưa có ${TOKEN_FILE} — chạy 01-rke2-server trước."
  exit 1
fi

RKE2_JOIN_TOKEN="$(tr -d '\n' < "${TOKEN_FILE}")"
RKE2_SERVER_IP="${NODE_PUBLIC_IP:?NODE_PUBLIC_IP chưa set trong config/env.sh}"
RKE2_SERVER_URL="https://${RKE2_SERVER_IP}:9345"

kubectl create namespace "${NS}" --dry-run=client -o yaml | kubectl apply -f -

kubectl -n "${NS}" create secret generic rke2-join \
  --from-literal=server_token="${RKE2_JOIN_TOKEN}" \
  --from-literal=server_url="${RKE2_SERVER_URL}" \
  --from-literal=server_ip="${RKE2_SERVER_IP}" \
  --dry-run=client -o yaml | kubectl apply -f -

# Gate secret — cần khi lấy script join từ Console (không lộ token qua GET)
JOIN_GATE_FILE="${ROOT_DIR}/config/join-gate.env"
if [[ ! -f "${JOIN_GATE_FILE}" ]]; then
  JOIN_GATE_SECRET="$(openssl rand -hex 32)"
  cat >"${JOIN_GATE_FILE}" <<EOF
# Không commit — PIN join worker trên Console
JOIN_GATE_SECRET="${JOIN_GATE_SECRET}"
EOF
  chmod 600 "${JOIN_GATE_FILE}"
  log "Đã tạo config/join-gate.env — giữ PIN này, dùng trên Console → Thêm worker"
else
  # shellcheck source=/dev/null
  source "${JOIN_GATE_FILE}"
fi

log "Secret platform/rke2-join OK (token không ghi log)"
mark_step_done "$0"
