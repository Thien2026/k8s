#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/../lib/common.sh"

export KUBECONFIG="${ROOT_DIR}/kubeconfig/rke2.yaml"
kube_ready
helm_ready

log "Cài NGINX Ingress Controller"

if helm status ingress-nginx -n ingress-nginx >/dev/null 2>&1; then
  log "ingress-nginx đã cài — upgrade nếu cần."
  helm upgrade ingress-nginx ingress-nginx/ingress-nginx \
    -n ingress-nginx \
    --version "${INGRESS_NGINX_CHART_VERSION}" \
    --set controller.service.type=LoadBalancer \
    --reuse-values 2>/dev/null || \
  helm upgrade ingress-nginx ingress-nginx/ingress-nginx \
    -n ingress-nginx \
    --version "${INGRESS_NGINX_CHART_VERSION}" \
    --set controller.service.type=LoadBalancer
else
  helm install ingress-nginx ingress-nginx/ingress-nginx \
    -n ingress-nginx --create-namespace \
    --version "${INGRESS_NGINX_CHART_VERSION}" \
    --set controller.service.type=LoadBalancer
fi

kubectl -n ingress-nginx rollout status deploy/ingress-nginx-controller --timeout=300s
kubectl -n ingress-nginx get svc

mark_step_done "$0"
