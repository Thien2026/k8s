#!/usr/bin/env bash
# L4C pilot: tạo branch multi-submodules trên huuthienit97/test-k8s
# Chạy trên máy có quyền push GitHub (đã clone test-k8s).
set -euo pipefail

REPO_DIR="${1:-}"
LIB_REPO="${LIB_REPO:-huuthienit97/test-k8s-lib-node}"
BASE_BRANCH="${BASE_BRANCH:-multi-polyglot-full}"
PILOT_BRANCH="${PILOT_BRANCH:-multi-submodules}"

if [[ -z "${REPO_DIR}" || ! -d "${REPO_DIR}/.git" ]]; then
  echo "Usage: $0 /path/to/test-k8s-clone"
  exit 1
fi

cd "${REPO_DIR}"
git fetch origin "${BASE_BRANCH}"
git checkout -B "${PILOT_BRANCH}" "origin/${BASE_BRANCH}"

# Lib repo (tạo trước trên GitHub nếu chưa có)
if ! git ls-remote "https://github.com/${LIB_REPO}.git" HEAD >/dev/null 2>&1; then
  LIB_TMP="$(mktemp -d)"
  trap 'rm -rf "${LIB_TMP}"' EXIT
  mkdir -p "${LIB_TMP}/repo"
  cat >"${LIB_TMP}/repo/package.json" <<'EOF'
{"name":"test-k8s-lib-node","version":"1.0.0","main":"index.js"}
EOF
  cat >"${LIB_TMP}/repo/index.js" <<'EOF'
"use strict";
module.exports = { greeting: "hello from submodule lib", version: "lib-node-1" };
EOF
  cat >"${LIB_TMP}/repo/README.md" <<'EOF'
# test-k8s-lib-node
Shared Node lib for L4C submodule pilot.
EOF
  git -C "${LIB_TMP}/repo" init -b main
  git -C "${LIB_TMP}/repo" add .
  git -C "${LIB_TMP}/repo" commit -m "init: L4C submodule lib"
  git -C "${LIB_TMP}/repo" remote add origin "https://github.com/${LIB_REPO}.git"
  git -C "${LIB_TMP}/repo" push -u origin main
fi

git submodule add -f "https://github.com/${LIB_REPO}.git" libs/node-lib
git submodule update --init --recursive

cat >backend-node/index.js <<'EOF'
"use strict";

const http = require("http");
const lib = require("../libs/node-lib");

const VERSION = "polyglot-node-submodule-1";
const PORT = Number(process.env.PORT || 8080);

const server = http.createServer((req, res) => {
  const url = req.url || "/";
  if (url === "/health") {
    res.writeHead(200, { "Content-Type": "application/json" });
    res.end(JSON.stringify({ status: "ok", service: "node", stack: "node", version: VERSION, lib: lib.version }));
    return;
  }
  if (url === "/hello") {
    res.writeHead(200, { "Content-Type": "application/json" });
    res.end(JSON.stringify({ message: lib.greeting, stack: "node", version: VERSION, lib_version: lib.version, submodule: true }));
    return;
  }
  res.writeHead(404, { "Content-Type": "text/plain" });
  res.end("not found");
});

server.listen(PORT, () => {
  console.log(`node backend listening on :${PORT} (${VERSION}) submodule=${lib.version}`);
});
EOF

python3 - <<'PY'
from pathlib import Path
p = Path(".platform/services.yaml")
text = p.read_text()
if "git:" not in text:
    text = text.replace("layout: multi\n", "layout: multi\ngit:\n  submodules: recursive\n", 1)
    p.write_text(text)
PY

git add .gitmodules libs/node-lib backend-node/index.js .platform/services.yaml
git commit -m "feat(L4C): git submodule pilot — node dùng libs/node-lib"
git push -u origin "${PILOT_BRANCH}"

echo "Done. Trên Console: research-labs → branch ${PILOT_BRANCH} → Áp dụng contract → Sync workflow."
