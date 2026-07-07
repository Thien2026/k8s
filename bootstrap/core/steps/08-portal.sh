#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/../../lib/common.sh"

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
log "Build single-file HTML (1 request)..."
"${ROOT_DIR}/services/portal-web/build.sh"
import_image "${WEB_IMAGE}" "${ROOT_DIR}/services/portal-web"

CORS_ORIGIN="https://${PLATFORM_HOST}"

RANCHER_URL="${RANCHER_URL:-https://${RANCHER_HOST}}"
RANCHER_TOKEN=""
if [[ -f "${ROOT_DIR}/config/rancher.env" ]]; then
  # shellcheck source=/dev/null
  source "${ROOT_DIR}/config/rancher.env"
fi
if [[ -z "${RANCHER_TOKEN}" ]] && kubectl -n "${NS}" get secret portal-api-env >/dev/null 2>&1; then
  RANCHER_TOKEN="$(kubectl -n "${NS}" get secret portal-api-env -o jsonpath='{.data.RANCHER_TOKEN}' 2>/dev/null | base64 -d 2>/dev/null || true)"
fi

JOIN_GATE_SECRET=""
if [[ -f "${ROOT_DIR}/config/join-gate.env" ]]; then
  # shellcheck source=/dev/null
  source "${ROOT_DIR}/config/join-gate.env"
fi
if [[ -z "${JOIN_GATE_SECRET}" ]]; then
  JOIN_GATE_SECRET="$(openssl rand -hex 32)"
  mkdir -p "${ROOT_DIR}/config"
  cat >"${ROOT_DIR}/config/join-gate.env" <<EOF
JOIN_GATE_SECRET="${JOIN_GATE_SECRET}"
EOF
  chmod 600 "${ROOT_DIR}/config/join-gate.env"
  log "Đã tạo config/join-gate.env — PIN join worker (xem trên VPS, không commit)"
fi

# RKE2 join credentials (secret volume — token không vào env plain)
if [[ -f "$(core_step 01b-rke2-join-secret)" ]]; then
  bash "$(core_step 01b-rke2-join-secret)" 2>/dev/null || true
fi

JWT_SECRET=""
if [[ -f "${ROOT_DIR}/config/platform-auth.env" ]]; then
  # shellcheck source=/dev/null
  source "${ROOT_DIR}/config/platform-auth.env"
fi
if [[ -z "${JWT_SECRET}" ]] && kubectl -n "${NS}" get secret platform-auth-keys >/dev/null 2>&1; then
  JWT_SECRET="$(kubectl -n "${NS}" get secret platform-auth-keys -o jsonpath='{.data.JWT_SECRET}' | base64 -d)"
fi

HARBOR_URL=""
HARBOR_ADMIN_PASSWORD=""
HARBOR_ADMIN_USER="admin"
if [[ -f "${ROOT_DIR}/config/harbor.env" ]]; then
  # shellcheck source=/dev/null
  source "${ROOT_DIR}/config/harbor.env"
fi

RANCHER_ADMIN_PASSWORD=""
if [[ -f "${ROOT_DIR}/config/rancher-admin.env" ]]; then
  # shellcheck source=/dev/null
  source "${ROOT_DIR}/config/rancher-admin.env"
fi

SECRET_ARGS=(
  --from-literal=DATABASE_URL="${DATABASE_URL}"
  --from-literal=PORT="8080"
  --from-literal=CORS_ORIGIN="${CORS_ORIGIN}"
  --from-literal=RANCHER_URL="${RANCHER_URL}"
  --from-literal=RANCHER_TOKEN="${RANCHER_TOKEN}"
  --from-literal=JOIN_GATE_SECRET="${JOIN_GATE_SECRET}"
  --from-literal=JWT_SECRET="${JWT_SECRET}"
  --from-literal=COOKIE_SECURE="true"
)
if [[ -n "${HARBOR_URL}" ]]; then
  SECRET_ARGS+=(--from-literal=HARBOR_URL="${HARBOR_URL}")
fi
if [[ -n "${HARBOR_ADMIN_PASSWORD}" ]]; then
  SECRET_ARGS+=(--from-literal=HARBOR_ADMIN_PASSWORD="${HARBOR_ADMIN_PASSWORD}")
fi
if [[ -n "${HARBOR_ADMIN_USER}" ]]; then
  SECRET_ARGS+=(--from-literal=HARBOR_ADMIN_USER="${HARBOR_ADMIN_USER}")
fi
if [[ -n "${RANCHER_ADMIN_PASSWORD}" ]]; then
  SECRET_ARGS+=(--from-literal=RANCHER_ADMIN_PASSWORD="${RANCHER_ADMIN_PASSWORD}")
fi
GHCR_ORG="${GHCR_ORG:-}"
if [[ -n "${GHCR_ORG}" ]]; then
  SECRET_ARGS+=(--from-literal=GHCR_ORG="${GHCR_ORG}")
fi
GHCR_PULL_USER="${GHCR_PULL_USER:-${GHCR_ORG:-}}"
GHCR_PULL_TOKEN="${GHCR_PULL_TOKEN:-}"
if [[ -n "${GHCR_PULL_USER}" ]]; then
  SECRET_ARGS+=(--from-literal=GHCR_PULL_USER="${GHCR_PULL_USER}")
fi
if [[ -n "${GHCR_PULL_TOKEN}" ]]; then
  SECRET_ARGS+=(--from-literal=GHCR_PULL_TOKEN="${GHCR_PULL_TOKEN}")
fi
# shellcheck source=/dev/null
source "${ROOT_DIR}/config/env.sh" 2>/dev/null || true
if [[ -n "${PLATFORM_DOMAIN:-}" ]]; then
  SECRET_ARGS+=(--from-literal=PLATFORM_DOMAIN="${PLATFORM_DOMAIN}")
elif [[ -n "${DOMAIN:-}" ]]; then
  SECRET_ARGS+=(--from-literal=PLATFORM_DOMAIN="platform.${DOMAIN}")
fi
if [[ -n "${NODE_PUBLIC_IP:-}" ]]; then
  SECRET_ARGS+=(--from-literal=NODE_PUBLIC_IP="${NODE_PUBLIC_IP}")
fi
if [[ -n "${ARGOCD_HOST:-}" ]]; then
  SECRET_ARGS+=(--from-literal=ARGOCD_HOST="${ARGOCD_HOST}")
  SECRET_ARGS+=(--from-literal=ARGOCD_URL="https://${ARGOCD_HOST}")
fi
if [[ -f "${ROOT_DIR}/config/grafana.env" ]]; then
  # shellcheck source=/dev/null
  source "${ROOT_DIR}/config/grafana.env"
fi
if [[ -n "${GRAFANA_HOST:-}" ]]; then
  SECRET_ARGS+=(--from-literal=GRAFANA_HOST="${GRAFANA_HOST}")
  SECRET_ARGS+=(--from-literal=GRAFANA_URL="https://${GRAFANA_HOST}")
fi
if [[ -n "${GRAFANA_ADMIN_USER:-}" ]]; then
  SECRET_ARGS+=(--from-literal=GRAFANA_ADMIN_USER="${GRAFANA_ADMIN_USER}")
fi
if [[ -n "${GRAFANA_ADMIN_PASSWORD:-}" ]]; then
  SECRET_ARGS+=(--from-literal=GRAFANA_ADMIN_PASSWORD="${GRAFANA_ADMIN_PASSWORD}")
fi
SECRET_ARGS+=(--from-literal=ARGOCD_NAMESPACE="${ARGOCD_NAMESPACE:-argocd}")
if [[ -n "${GITOPS_REPO_URL:-}" ]]; then
  SECRET_ARGS+=(--from-literal=GITOPS_REPO_URL="${GITOPS_REPO_URL}")
fi
if [[ -n "${GITOPS_REPO_BRANCH:-}" ]]; then
  SECRET_ARGS+=(--from-literal=GITOPS_REPO_BRANCH="${GITOPS_REPO_BRANCH}")
fi
if [[ -n "${GITOPS_BASE_PATH:-}" ]]; then
  SECRET_ARGS+=(--from-literal=GITOPS_BASE_PATH="${GITOPS_BASE_PATH}")
fi
if [[ -n "${GITOPS_PUSH_TOKEN:-}" ]]; then
  SECRET_ARGS+=(--from-literal=GITOPS_PUSH_TOKEN="${GITOPS_PUSH_TOKEN}")
fi
if [[ -n "${GITHUB_CLIENT_ID:-}" ]]; then
  SECRET_ARGS+=(--from-literal=GITHUB_CLIENT_ID="${GITHUB_CLIENT_ID}")
fi
if [[ -n "${GITHUB_CLIENT_SECRET:-}" ]]; then
  SECRET_ARGS+=(--from-literal=GITHUB_CLIENT_SECRET="${GITHUB_CLIENT_SECRET}")
fi
PLATFORM_PUBLIC_URL="https://${PLATFORM_HOST}"
SECRET_ARGS+=(--from-literal=PLATFORM_PUBLIC_URL="${PLATFORM_PUBLIC_URL}")
SECRET_ARGS+=(--from-literal=GITHUB_REDIRECT_URI="${PLATFORM_PUBLIC_URL}/api/v1/github/oauth/callback")

# Đăng nhập nhanh (tạm — gỡ khi production): cần PLATFORM_ADMIN_* trong platform-auth.env
QUICK_LOGIN_ENABLED="${QUICK_LOGIN_ENABLED:-true}"
PLATFORM_ADMIN_EMAIL="${PLATFORM_ADMIN_EMAIL:-admin@platform.local}"
if [[ -n "${PLATFORM_ADMIN_PASSWORD:-}" ]]; then
  SECRET_ARGS+=(--from-literal=QUICK_LOGIN_ENABLED="${QUICK_LOGIN_ENABLED}")
  SECRET_ARGS+=(--from-literal=PLATFORM_ADMIN_EMAIL="${PLATFORM_ADMIN_EMAIL}")
  SECRET_ARGS+=(--from-literal=PLATFORM_ADMIN_PASSWORD="${PLATFORM_ADMIN_PASSWORD}")
fi

kubectl -n "${NS}" create secret generic portal-api-env \
  "${SECRET_ARGS[@]}" \
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
          volumeMounts:
            - name: rke2-join
              mountPath: /etc/rke2-join
              readOnly: true
          readinessProbe:
            httpGet:
              path: /health
              port: 8080
            initialDelaySeconds: 5
            periodSeconds: 10
      volumes:
        - name: rke2-join
          secret:
            secretName: rke2-join
            optional: true
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
    nginx.ingress.kubernetes.io/gzip-types: "text/css application/javascript application/json text/plain text/html"
    nginx.ingress.kubernetes.io/ssl-session-cache: "true"
    nginx.ingress.kubernetes.io/ssl-session-cache-size: "10m"
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

# Image tag :local không đổi → buộc restart pod sau rebuild
if [[ "${FORCE_BUILD:-}" == "1" ]]; then
  log "Restart portal pods sau rebuild image..."
  kubectl -n "${NS}" rollout restart deploy/portal-api deploy/portal-web
  kubectl -n "${NS}" rollout status deploy/portal-api --timeout=300s
  kubectl -n "${NS}" rollout status deploy/portal-web --timeout=300s
fi

log "Platform Console: https://${PLATFORM_HOST}"
log "API health: https://${PLATFORM_HOST}/health"
log "Trỏ DNS ${PLATFORM_HOST} → ${NODE_PUBLIC_IP} (sslip.io tự resolve nếu dùng *.sslip.io)"

AUTH_SEED="$(core_step 08a-portal-auth-seed)"
if [[ -f "${AUTH_SEED}" ]]; then
  bash "${AUTH_SEED}" || log "08a auth seed cảnh báo — xem log"
fi

mark_step_done "$0"
