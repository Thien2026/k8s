#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/../lib/common.sh"

log "Copy kubeconfig về repo (chạy trên VPS sau bước 01)"

KCFG_DIR="${ROOT_DIR}/kubeconfig"
mkdir -p "${KCFG_DIR}"
chmod 700 "${KCFG_DIR}"

if [[ "$(id -u)" -eq 0 ]]; then
  SRC="/etc/rancher/rke2/rke2.yaml"
else
  SRC="${HOME}/.kube/config"
  if [[ ! -f "${SRC}" && -f /etc/rancher/rke2/rke2.yaml ]]; then
    sudo cp /etc/rancher/rke2/rke2.yaml "${KCFG_DIR}/rke2.yaml"
    sudo chown "$(id -u):$(id -g)" "${KCFG_DIR}/rke2.yaml"
    SRC="${KCFG_DIR}/rke2.yaml"
  fi
fi

if [[ -f /etc/rancher/rke2/rke2.yaml ]]; then
  cp /etc/rancher/rke2/rke2.yaml "${KCFG_DIR}/rke2.yaml"
  chmod 600 "${KCFG_DIR}/rke2.yaml"
  # Thay localhost bằng IP VPS nếu kubectl chạy từ máy khác
  if [[ -n "${NODE_PUBLIC_IP:-}" ]]; then
    sed -i.bak "s/127.0.0.1/${NODE_PUBLIC_IP}/g" "${KCFG_DIR}/rke2.yaml"
    rm -f "${KCFG_DIR}/rke2.yaml.bak"
  fi
fi

export KUBECONFIG="${KCFG_DIR}/rke2.yaml"
require_cmd kubectl

kubectl get nodes
kubectl get pods -A | head -20

log "Kubeconfig: ${KCFG_DIR}/rke2.yaml"
log "Trên máy local: export KUBECONFIG=/path/to/k8s/kubeconfig/rke2.yaml"

mark_step_done "$0"
