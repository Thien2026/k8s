#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/../lib/common.sh"

export KUBECONFIG="${ROOT_DIR}/kubeconfig/rke2.yaml"
kube_ready
helm_ready

log "Kiểm tra NGINX Ingress Controller"

# RKE2 đã bundle ingress-nginx trong kube-system — không cài thêm bản thứ 2
if kubectl -n kube-system get deploy rke2-ingress-nginx-controller >/dev/null 2>&1; then
  log "RKE2 đã có ingress-nginx (kube-system) — bỏ qua cài Helm riêng."
  kubectl -n kube-system rollout status deploy/rke2-ingress-nginx-controller --timeout=300s
  kubectl -n kube-system get svc -l app.kubernetes.io/name=rke2-ingress-nginx || \
    kubectl -n kube-system get svc | grep -i ingress || true
elif helm status ingress-nginx -n ingress-nginx >/dev/null 2>&1; then
  log "ingress-nginx (ingress-nginx namespace) đã cài — kiểm tra rollout."
  kubectl -n ingress-nginx rollout status deploy/ingress-nginx-controller --timeout=300s
  kubectl -n ingress-nginx get svc
else
  log "Cài ingress-nginx qua Helm..."
  helm install ingress-nginx ingress-nginx/ingress-nginx \
    -n ingress-nginx --create-namespace \
    --version "${INGRESS_NGINX_CHART_VERSION}" \
    --set controller.service.type=LoadBalancer
  kubectl -n ingress-nginx rollout status deploy/ingress-nginx-controller --timeout=300s
  kubectl -n ingress-nginx get svc
fi

mark_step_done "$0"
