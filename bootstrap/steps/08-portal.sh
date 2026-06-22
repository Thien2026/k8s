#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/../lib/common.sh"

export KUBECONFIG="${ROOT_DIR}/kubeconfig/rke2.yaml"
kube_ready

NS="platform"
PLATFORM_HOST="${PLATFORM_HOST:-platform.${DOMAIN}}"
API_IMAGE="portal-api:local"
WEB_IMAGE="portal-web:local"
CTR_SOCK="${CTR_SOCK:-/run/k3s/containerd/containerd.sock}"

log "Deploy Platform Console → https://${PLATFORM_HOST}"

if ! kubectl -n "${NS}" get statefulset platform-postgresql >/dev/null 2>&1; then
  log "Chưa có PostgreSQL — chạy ./bootstrap/run.sh 07 trước."
  exit 1
fi

if [[ -f "${ROOT_DIR}/config/postgres.env" ]]; then
  # shellcheck source=/dev/null
  source "${ROOT_DIR}/config/postgres.env"
fi
: "${DATABASE_URL:?Thiếu DATABASE_URL — chạy bước 07 hoặc tạo config/postgres.env}"

ensure_docker() {
  if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
    return
  fi
  log "Cài Docker để build image..."
  apt-get update -qq
  DEBIAN_FRONTEND=noninteractive apt-get install -y -qq docker.io
  systemctl enable --now docker
}

import_image() {
  local tag="$1"
  local dir="$2"
  local extra_args=""
  [[ "${FORCE_BUILD:-}" == "1" ]] && extra_args="--no-cache"
  log "Build image ${tag} từ ${dir}..."
  docker build ${extra_args} -t "${tag}" "${dir}"
  log "Import ${tag} vào RKE2 containerd..."
  docker save "${tag}" | ctr --address "${CTR_SOCK}" -n k8s.io images import -
}

ensure_docker
import_image "${API_IMAGE}" "${ROOT_DIR}/services/portal-api"
import_image "${WEB_IMAGE}" "${ROOT_DIR}/services/portal-web"

CORS_ORIGIN="https://${PLATFORM_HOST}"

kubectl -n "${NS}" create secret generic portal-api-env \
  --from-literal=DATABASE_URL="${DATABASE_URL}" \
  --from-literal=PORT="8080" \
  --from-literal=CORS_ORIGIN="${CORS_ORIGIN}" \
  --dry-run=client -o yaml | kubectl apply -f -

cat <<EOF | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: portal-api
  namespace: ${NS}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: portal-api
  template:
    metadata:
      labels:
        app: portal-api
    spec:
      containers:
        - name: api
          image: ${API_IMAGE}
          imagePullPolicy: IfNotPresent
          ports:
            - containerPort: 8080
          envFrom:
            - secretRef:
                name: portal-api-env
          readinessProbe:
            httpGet:
              path: /health
              port: 8080
            initialDelaySeconds: 5
            periodSeconds: 10
---
apiVersion: v1
kind: Service
metadata:
  name: portal-api
  namespace: ${NS}
spec:
  selector:
    app: portal-api
  ports:
    - port: 8080
      targetPort: 8080
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: portal-web
  namespace: ${NS}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: portal-web
  template:
    metadata:
      labels:
        app: portal-web
    spec:
      containers:
        - name: web
          image: ${WEB_IMAGE}
          imagePullPolicy: IfNotPresent
          ports:
            - containerPort: 80
          readinessProbe:
            httpGet:
              path: /
              port: 80
            initialDelaySeconds: 3
            periodSeconds: 10
---
apiVersion: v1
kind: Service
metadata:
  name: portal-web
  namespace: ${NS}
spec:
  selector:
    app: portal-web
  ports:
    - port: 80
      targetPort: 80
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: platform-console
  namespace: ${NS}
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
    nginx.ingress.kubernetes.io/enable-gzip: "true"
    nginx.ingress.kubernetes.io/gzip-types: "text/css application/javascript application/json text/plain"
spec:
  ingressClassName: nginx
  tls:
    - hosts:
        - ${PLATFORM_HOST}
      secretName: platform-console-tls
  rules:
    - host: ${PLATFORM_HOST}
      http:
        paths:
          - path: /api
            pathType: Prefix
            backend:
              service:
                name: portal-api
                port:
                  number: 8080
          - path: /health
            pathType: Prefix
            backend:
              service:
                name: portal-api
                port:
                  number: 8080
          - path: /
            pathType: Prefix
            backend:
              service:
                name: portal-web
                port:
                  number: 80
EOF

kubectl -n "${NS}" rollout status deploy/portal-api --timeout=300s
kubectl -n "${NS}" rollout status deploy/portal-web --timeout=300s

log "Platform Console: https://${PLATFORM_HOST}"
log "API health: https://${PLATFORM_HOST}/health"
log "Trỏ DNS ${PLATFORM_HOST} → ${NODE_PUBLIC_IP} (sslip.io tự resolve nếu dùng *.sslip.io)"

mark_step_done "$0"
