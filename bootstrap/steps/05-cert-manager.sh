#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/../lib/common.sh"

export KUBECONFIG="${ROOT_DIR}/kubeconfig/rke2.yaml"
kube_ready
helm_ready

log "Cài cert-manager + ClusterIssuer Let's Encrypt"

if ! helm status cert-manager -n cert-manager >/dev/null 2>&1; then
  helm install cert-manager jetstack/cert-manager \
    -n cert-manager --create-namespace \
    --version "${CERT_MANAGER_CHART_VERSION}" \
    --set crds.enabled=true
fi

kubectl -n cert-manager rollout status deploy/cert-manager --timeout=300s

cat <<EOF | kubectl apply -f -
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-prod
spec:
  acme:
    server: https://acme-v02.api.letsencrypt.org/directory
    email: ${LETSENCRYPT_EMAIL}
    privateKeySecretRef:
      name: letsencrypt-prod
    solvers:
      - http01:
          ingress:
            class: nginx
EOF

log "ClusterIssuer letsencrypt-prod OK"
mark_step_done "$0"
