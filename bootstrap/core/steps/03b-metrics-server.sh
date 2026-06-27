#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/../../lib/common.sh"

export KUBECONFIG="${ROOT_DIR}/kubeconfig/rke2.yaml"
kube_ready
helm_ready

NS="kube-system"
RELEASE="metrics-server"

log "Cài metrics-server (CPU/RAM dashboard)"

if kubectl get apiservice v1beta1.metrics.k8s.io >/dev/null 2>&1; then
  log "metrics-server đã có (RKE2/có sẵn) — bỏ qua cài thêm."
  mark_step_done "$0"
  exit 0
fi

helm repo add metrics-server https://kubernetes-sigs.github.io/metrics-server/ 2>/dev/null || true
helm repo update metrics-server

if helm status "${RELEASE}" -n "${NS}" >/dev/null 2>&1; then
  helm upgrade "${RELEASE}" metrics-server/metrics-server \
    -n "${NS}" \
    --set args="{--kubelet-insecure-tls,--kubelet-preferred-address-types=InternalIP}"
else
  helm install "${RELEASE}" metrics-server/metrics-server \
    -n "${NS}" \
    --set args="{--kubelet-insecure-tls,--kubelet-preferred-address-types=InternalIP}"
fi

kubectl -n "${NS}" rollout status deploy/metrics-server --timeout=180s 2>/dev/null || \
  log "metrics-server đang khởi động — dashboard capacity có thể chậm vài phút"

mark_step_done "$0"
