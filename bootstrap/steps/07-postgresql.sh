#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/../lib/common.sh"

export KUBECONFIG="${ROOT_DIR}/kubeconfig/rke2.yaml"
kube_ready

: "${POSTGRES_PASSWORD:?Đặt POSTGRES_PASSWORD trong config/env.sh}"

NS="platform"
DB_NAME="${POSTGRES_DB:-platform}"
DB_USER="${POSTGRES_USER:-platform}"
APP_LABEL="platform-postgresql"

log "Cài PostgreSQL cho Platform Console (namespace: ${NS})"
log "Lưu ý: DB này chỉ cho portal-api (user/project). DB app khách cài riêng per-app."

ensure_storage_class() {
  if kubectl get sc local-path >/dev/null 2>&1; then
    return
  fi
  log "Cài local-path-provisioner..."
  kubectl apply -f https://raw.githubusercontent.com/rancher/local-path-provisioner/v0.0.30/deploy/local-path-storage.yaml
  kubectl annotate storageclass local-path storageclass.kubernetes.io/is-default-class=true --overwrite
}

ensure_storage_class

kubectl create namespace "${NS}" --dry-run=client -o yaml | kubectl apply -f -

kubectl -n "${NS}" create secret generic platform-postgresql-auth \
  --from-literal=POSTGRES_DB="${DB_NAME}" \
  --from-literal=POSTGRES_USER="${DB_USER}" \
  --from-literal=POSTGRES_PASSWORD="${POSTGRES_PASSWORD}" \
  --dry-run=client -o yaml | kubectl apply -f -

# Gỡ Bitnami release cũ nếu có (image pull lỗi)
if helm status platform-postgresql -n "${NS}" >/dev/null 2>&1; then
  log "Gỡ Helm release platform-postgresql cũ..."
  helm uninstall platform-postgresql -n "${NS}" || true
  kubectl -n "${NS}" delete statefulset platform-postgresql --ignore-not-found
  kubectl -n "${NS}" delete svc platform-postgresql platform-postgresql-hl --ignore-not-found
fi

cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Service
metadata:
  name: platform-postgresql
  namespace: ${NS}
spec:
  ports:
    - port: 5432
      targetPort: 5432
  selector:
    app: ${APP_LABEL}
---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: platform-postgresql
  namespace: ${NS}
spec:
  serviceName: platform-postgresql
  replicas: 1
  selector:
    matchLabels:
      app: ${APP_LABEL}
  template:
    metadata:
      labels:
        app: ${APP_LABEL}
    spec:
      containers:
        - name: postgresql
          image: postgres:16-alpine
          ports:
            - containerPort: 5432
          envFrom:
            - secretRef:
                name: platform-postgresql-auth
          volumeMounts:
            - name: data
              mountPath: /var/lib/postgresql/data
              subPath: pgdata
          readinessProbe:
            exec:
              command: ["pg_isready", "-U", "${DB_USER}", "-d", "${DB_NAME}"]
            initialDelaySeconds: 10
            periodSeconds: 5
  volumeClaimTemplates:
    - metadata:
        name: data
      spec:
        accessModes: ["ReadWriteOnce"]
        storageClassName: local-path
        resources:
          requests:
            storage: ${POSTGRES_STORAGE_SIZE:-8Gi}
EOF

kubectl -n "${NS}" rollout status statefulset/platform-postgresql --timeout=300s

HOST="platform-postgresql.${NS}.svc.cluster.local"
CONN_FILE="${ROOT_DIR}/config/postgres.env"
mkdir -p "$(dirname "${CONN_FILE}")"
cat >"${CONN_FILE}" <<EOF
# Tự sinh bởi bootstrap/steps/07-postgresql.sh — không commit
POSTGRES_HOST=${HOST}
POSTGRES_PORT=5432
POSTGRES_DB=${DB_NAME}
POSTGRES_USER=${DB_USER}
POSTGRES_PASSWORD=${POSTGRES_PASSWORD}
DATABASE_URL=postgres://${DB_USER}:${POSTGRES_PASSWORD}@${HOST}:5432/${DB_NAME}?sslmode=disable
EOF
chmod 600 "${CONN_FILE}"

log "PostgreSQL OK"
log "Service: ${HOST}:5432"
log "Connection: ${CONN_FILE}"
kubectl -n "${NS}" get pods -l app=${APP_LABEL}

mark_step_done "$0"
