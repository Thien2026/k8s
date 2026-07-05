#!/usr/bin/env bash
# Phase 1 golden path: copy template back-front vào repo Git khách.
# Usage:
#   ./scripts/setup-back-front-pilot.sh /path/to/repo-clone [branch]
#   ./scripts/setup-back-front-pilot.sh --check   # validate template only
set -euo pipefail

PILOT_BRANCH="back-front-pilot"
REPO_DIR=""
CHECK_ONLY=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --check)
      CHECK_ONLY=1
      shift
      ;;
    -h|--help)
      sed -n '1,12p' "$0"
      exit 0
      ;;
    *)
      if [[ -z "${REPO_DIR}" ]]; then
        REPO_DIR="$1"
      elif [[ "${PILOT_BRANCH}" == "back-front-pilot" && "$1" != back-front-* ]]; then
        PILOT_BRANCH="$1"
      fi
      shift
      ;;
  esac
done

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
K8S_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
TEMPLATE="${K8S_ROOT}/templates/back-front"

validate_template() {
  local ok=1
  for f in backend/Dockerfile backend/main.go frontend/Dockerfile frontend/dist/index.html \
    .platform/services.yaml .platform/build.yaml .platform/runtime.yaml PLATFORM_DEPLOY.md; do
    if [[ ! -e "${TEMPLATE}/${f}" ]]; then
      echo "✗ thiếu template/${f}"
      ok=0
    fi
  done
  if [[ "${ok}" -eq 1 ]]; then
    echo "✓ Template back-front đủ file (Phase 1)"
  else
    exit 1
  fi
}

validate_template

if [[ "${CHECK_ONLY}" -eq 1 ]]; then
  exit 0
fi

if [[ -z "${REPO_DIR}" || ! -d "${REPO_DIR}/.git" ]]; then
  echo "Usage: $0 /path/to/repo-clone [branch-name]"
  echo "       $0 --check"
  echo ""
  echo "  branch-name mặc định: back-front-pilot"
  echo "  Doc khách: docs/KHACH_DEPLOY.md"
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
cp "${TEMPLATE}/PLATFORM_DEPLOY.md" "${REPO_DIR}/PLATFORM_DEPLOY.md"

rm -f "${REPO_DIR}/.platform/services.yaml.example"

if [[ ! -f "${REPO_DIR}/.gitignore" ]]; then
  cat >"${REPO_DIR}/.gitignore" <<'EOF'
.DS_Store
node_modules/
EOF
fi

git add backend frontend .platform PLATFORM_DEPLOY.md .gitignore 2>/dev/null || git add backend frontend .platform PLATFORM_DEPLOY.md
if git diff --cached --quiet; then
  echo "Không có thay đổi — branch ${PILOT_BRANCH} đã có template."
else
  git commit -m "$(cat <<EOF
chore(platform): add back-front template (Phase 1 api+web)

Monorepo backend + frontend with .platform/services.yaml for Console.
See PLATFORM_DEPLOY.md for deploy steps.
EOF
)"
fi

echo ""
echo "════════════════════════════════════════════════════════"
echo "✓ Branch ${PILOT_BRANCH} sẵn sàng trong ${REPO_DIR}"
echo "════════════════════════════════════════════════════════"
echo ""
echo "1. Push GitHub:"
echo "     git push -u origin ${PILOT_BRANCH}"
echo ""
echo "2. Console (project MỚI — không dùng project pilot cũ):"
echo "     Deploy / Git → branch ${PILOT_BRANCH} · Deploy env = dev"
echo "     Web + API riêng → 「Áp dụng api + web từ repo」"
echo "     Lưu & đồng bộ GitHub → Workflow OK"
echo "     Push → kiểm tra / và /api/health"
echo ""
echo "3. Verify:"
echo "     ./scripts/verify-back-front-deploy.sh {slug} dev"
echo ""
echo "Doc gửi khách: docs/KHACH_DEPLOY.md"
echo "Doc trong repo: PLATFORM_DEPLOY.md"
