#!/usr/bin/env bash
# Gộp CSS + JS vào 1 file HTML — chỉ 1 request tải trang
set -euo pipefail
DIR="$(cd "$(dirname "$0")" && pwd)"
CSS="$(cat "$DIR/src/style.css")"
JS="$(cat "$DIR/src/app.js")"
cat >"$DIR/dist/index.html" <<EOF
<!doctype html>
<html lang="vi">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>Platform Console</title>
    <style>
${CSS}
    </style>
  </head>
  <body>
    <div class="layout">
      <aside class="sidebar">
        <div class="sidebar-brand">
          <h1>Platform Console</h1>
          <p>K8s Explorer</p>
        </div>
        <nav id="sidebar-nav" class="sidebar-nav loading">Đang tải menu…</nav>
        <div class="sidebar-footer">© 2026 <strong>7mlabs</strong></div>
      </aside>
      <main id="main" class="main">
        <p class="loading">Đang tải…</p>
      </main>
    </div>
    <script>
${JS}
    </script>
  </body>
</html>
EOF
echo "Built dist/index.html ($(wc -c <"$DIR/dist/index.html") bytes)"
