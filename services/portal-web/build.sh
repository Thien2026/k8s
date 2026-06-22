#!/usr/bin/env bash
# Gộp CSS + JS vào 1 file HTML — chỉ 1 request tải trang
set -euo pipefail
DIR="$(cd "$(dirname "$0")" && pwd)"
CSS="$(cat "$DIR/src/style.css")"
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
    <div class="page">
      <header>
        <h1>Platform Console</h1>
        <p class="muted">Quản lý tập trung — K8s, deploy, image</p>
      </header>
      <section class="card">
        <h2>Trạng thái hệ thống</h2>
        <p id="health-status" class="muted">Đang kiểm tra…</p>
      </section>
      <section class="card">
        <h2>Cluster (Rancher)</h2>
        <p id="cluster-status" class="muted">Đang kiểm tra…</p>
      </section>
      <section class="card">
        <h2>Projects</h2>
        <p id="projects-status" class="muted">Đang tải…</p>
        <ul id="projects-list" hidden></ul>
      </section>
    </div>
    <script>
(async () => {
  const healthEl = document.getElementById("health-status");
  const clusterEl = document.getElementById("cluster-status");
  const projectsStatusEl = document.getElementById("projects-status");
  const projectsListEl = document.getElementById("projects-list");
  try {
    const res = await fetch("/api/v1/dashboard");
    const data = await res.json();
    const h = data.health || {};
    if (h.status === "ok") {
      healthEl.className = "ok";
      healthEl.innerHTML = "API: <strong>" + h.status + "</strong>" +
        (h.database ? " · DB: <strong>" + h.database + "</strong>" : "");
    } else {
      healthEl.className = "error";
      healthEl.textContent = "API lỗi: " + (h.error || h.status);
    }
    const c = data.cluster || {};
    if (c.connected && c.total !== undefined) {
      clusterEl.className = "ok";
      clusterEl.innerHTML = "Clusters: <strong>" + c.ready + "/" + c.total + " ready</strong>" +
        (c.nodes ? " · Nodes: <strong>" + c.nodes + "</strong>" : "");
    } else if (c.error) {
      clusterEl.className = "error";
      clusterEl.textContent = "Rancher: " + c.error;
    } else {
      clusterEl.className = "muted";
      clusterEl.textContent = c.message || "Rancher chưa kết nối — thêm API token sau bước 09.";
    }
    const projects = data.projects || [];
    if (!projects.length) {
      projectsStatusEl.textContent = "Chưa có project — thêm sau qua API.";
      return;
    }
    projectsStatusEl.hidden = true;
    projectsListEl.hidden = false;
    projectsListEl.innerHTML = projects.map((p) =>
      "<li>" + p.name + " — dev: " + p.namespace_dev + ", prod: " + p.namespace_prod + "</li>"
    ).join("");
  } catch (e) {
    healthEl.className = "error";
    healthEl.textContent = "Lỗi kết nối: " + e;
    projectsStatusEl.textContent = "Không tải được dữ liệu.";
  }
})();
    </script>
  </body>
</html>
EOF
echo "Built dist/index.html ($(wc -c <"$DIR/dist/index.html") bytes)"
