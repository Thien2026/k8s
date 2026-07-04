#!/usr/bin/env bash
# Phase 1 pilot: copy template back-front vào repo Git, tạo branch back-front-pilot.
# Usage: ./scripts/setup-back-front-pilot.sh /path/to/repo-clone [branch]
set -euo pipefail

REPO_DIR="${1:-}"
PILOT_BRANCH="${2:-back-front-pilot}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
K8S_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
TEMPLATE="${K8S_ROOT}/templates/back-front"

if [[ -z "${REPO_DIR}" || ! -d "${REPO_DIR}/.git" ]]; then
  echo "Usage: $0 /path/to/repo-clone [branch-name]"
  echo "  branch-name mặc định: back-front-pilot"
  exit 1
fi

if [[ ! -d "${TEMPLATE}/backend" || ! -f "${TEMPLATE}/.platform/services.yaml" ]]; then
  echo "Không tìm thấy template tại ${TEMPLATE}"
  exit 1
fi

cd "${REPO_DIR}"
git fetch origin 2>/dev/null || true

if git show-ref --verify --quiet "refs/heads/${PILOT_BRANCH}"; then
  git checkout "${PILOT_BRANCH}"
else
  git checkout -B "${PILOT_BRANCH}"
fi

rsync -a --delete \
  "${TEMPLATE}/backend/" "${REPO_DIR}/backend/"
rsync -a --delete \
  "${TEMPLATE}/frontend/" "${REPO_DIR}/frontend/"
mkdir -p "${REPO_DIR}/.platform"
rsync -a \
  "${TEMPLATE}/.platform/" "${REPO_DIR}/.platform/"

# Bỏ file example khỏi repo đích
rm -f "${REPO_DIR}/.platform/services.yaml.example"

if [[ ! -f "${REPO_DIR}/.gitignore" ]]; then
  cat >"${REPO_DIR}/.gitignore" <<'EOF'
.DS_Store
node_modules/
EOF
fi

git add backend frontend .platform .gitignore 2>/dev/null || git add backend frontend .platform
if git diff --cached --quiet; then
  echo "Không có thay đổi — branch ${PILOT_BRANCH} đã có template."
else
  git commit -m "$(cat <<EOF
chore(platform): add back-front pilot template (Phase 1)

api + web monorepo with .platform/services.yaml for Console multi deploy.
EOF
)"
fi

echo ""
echo "✓ Branch ${PILOT_BRANCH} sẵn sàng."
echo "  Tiếp theo:"
echo "    git push -u origin ${PILOT_BRANCH}"
echo "  Console:"
echo "    1. Project mới → Deploy/Git → branch ${PILOT_BRANCH}"
echo "    2. Web + API riêng → Lưu & đồng bộ GitHub"
echo "    3. Cấu hình app: VITE_API_BASE=/api (build dev)"
echo "  Doc: docs/MICRO_DEPLOY.md"
