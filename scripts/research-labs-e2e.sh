#!/usr/bin/env bash
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
source "${ROOT_DIR}/config/harbor.env"
source "${ROOT_DIR}/config/platform-auth.env"
export KUBECONFIG="${KUBECONFIG:-/etc/rancher/rke2/rke2.yaml}"
export PATH="/var/lib/rancher/rke2/bin:${PATH}"

echo "=== Harbor auto_scan ==="
curl -sk -u "admin:${HARBOR_ADMIN_PASSWORD}" -X PATCH \
  "${HARBOR_URL}/api/v2.0/projects/research-labs" \
  -H "Content-Type: application/json" \
  -d '{"metadata":{"auto_scan":"true","public":"false"}}' | head -c 120
echo

echo "=== DB domains + env ==="
kubectl exec -n platform platform-postgresql-0 -- psql -U platform -d platform <<'SQL'
UPDATE project_domains SET hostname='research-labs-dev.platform.7mlabs.com', sync_status='pending', sync_error=''
WHERE project_id=(SELECT id FROM projects WHERE slug='research-labs') AND environment='dev';
UPDATE project_domains SET hostname='research-labs-prod.platform.7mlabs.com', sync_status='pending', sync_error=''
WHERE project_id=(SELECT id FROM projects WHERE slug='research-labs') AND environment='prod';
INSERT INTO project_env_vars (project_id, key, value, is_secret, environment)
SELECT id, 'APP_GREETING', 'hello-research-dev', false, 'dev' FROM projects WHERE slug='research-labs'
AND NOT EXISTS (SELECT 1 FROM project_env_vars e WHERE e.project_id=projects.id AND e.key='APP_GREETING' AND e.environment='dev');
INSERT INTO project_env_vars (project_id, key, value, is_secret, environment)
SELECT id, 'APP_GREETING', 'hello-research-prod', false, 'prod' FROM projects WHERE slug='research-labs'
AND NOT EXISTS (SELECT 1 FROM project_env_vars e WHERE e.project_id=projects.id AND e.key='APP_GREETING' AND e.environment='prod');
SELECT pd.id, pd.hostname, pd.environment, pd.sync_status FROM project_domains pd JOIN projects p ON p.id=pd.project_id WHERE p.slug='research-labs';
SQL

COOKIE=/tmp/pcookie.txt
curl -sk -c "$COOKIE" -X POST https://platform.7mlabs.com/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"${PLATFORM_ADMIN_EMAIL}\",\"password\":\"${PLATFORM_ADMIN_PASSWORD}\"}" >/dev/null

for did in 1 2; do
  echo "=== sync domain $did ==="
  curl -sk -b "$COOKIE" -X POST "https://platform.7mlabs.com/api/v1/projects/research-labs/domains/${did}/sync"
  echo
done

TAG="$(kubectl exec -n platform platform-postgresql-0 -- psql -U platform -d platform -t -A -c \
  "SELECT image_tag FROM project_deployments pd JOIN projects p ON p.id=pd.project_id WHERE p.slug='research-labs' AND environment='dev' AND status='success' ORDER BY pd.id DESC LIMIT 1;")"
TAG="${TAG//$'\n'/}"
echo "=== promote $TAG ==="
curl -sk -b "$COOKIE" -X POST https://platform.7mlabs.com/api/v1/projects/research-labs/deploy/promote \
  -H "Content-Type: application/json" \
  -d "{\"image_tag\":\"${TAG}\"}"
echo

sleep 15
echo "=== verify ==="
curl -sk --max-time 15 "https://research-labs-dev.platform.7mlabs.com/" | head -1
curl -sk --max-time 15 "https://research-labs-prod.platform.7mlabs.com/" | head -1
kubectl get pods -n research-labs-prod 2>/dev/null || true
