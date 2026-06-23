const $ = (sel) => document.querySelector(sel);
const state = { page: {}, limit: 50 };

function esc(s) {
  if (s == null) return "";
  return String(s)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

function fmtTime(iso) {
  if (!iso) return "—";
  const d = new Date(iso);
  if (isNaN(d)) return iso;
  const mins = Math.floor((Date.now() - d) / 60000);
  if (mins < 60) return mins + "m";
  const hrs = Math.floor(mins / 60);
  if (hrs < 48) return hrs + "h";
  return Math.floor(hrs / 24) + "d";
}

async function api(path) {
  const res = await fetch(path);
  const data = await res.json();
  if (!res.ok) throw new Error(data.error || res.statusText);
  return data;
}

function renderTable(columns, rows) {
  if (!rows.length) {
    return '<p class="muted">Không có dữ liệu.</p>';
  }
  const head = columns.map((c) => "<th>" + esc(c.label) + "</th>").join("");
  const body = rows
    .map((row) => {
      const cells = columns
        .map((c) => "<td>" + (c.render ? c.render(row) : esc(row[c.key] ?? "—")) + "</td>")
        .join("");
      return "<tr>" + cells + "</tr>";
    })
    .join("");
  return (
    '<div class="table-wrap"><table><thead><tr>' +
    head +
    "</tr></thead><tbody>" +
    body +
    "</tbody></table></div>"
  );
}

function renderPagination(route, total, page, limit, onChange) {
  const pages = Math.max(1, Math.ceil(total / limit));
  const start = total === 0 ? 0 : (page - 1) * limit + 1;
  const end = Math.min(page * limit, total);
  const id = "pager-" + route.replace(/\W/g, "");
  setTimeout(() => {
    const prev = document.getElementById(id + "-prev");
    const next = document.getElementById(id + "-next");
    const sel = document.getElementById(id + "-limit");
    if (prev) prev.onclick = () => onChange(page - 1, limit);
    if (next) next.onclick = () => onChange(page + 1, limit);
    if (sel) sel.onchange = () => onChange(1, parseInt(sel.value, 10));
  }, 0);
  return (
    '<div class="pagination" id="' +
    id +
    '">' +
    '<span class="muted">' +
    start +
    "–" +
    end +
    " / " +
    total +
    "</span>" +
    '<div style="display:flex;gap:8px;align-items:center">' +
    '<select id="' +
    id +
    '-limit">' +
    [25, 50, 100]
      .map(
        (n) =>
          '<option value="' +
          n +
          '"' +
          (n === limit ? " selected" : "") +
          ">" +
          n +
          "/trang</option>"
      )
      .join("") +
    "</select>" +
    '<button id="' +
    id +
    '-prev"' +
    (page <= 1 ? " disabled" : "") +
    ">← Trước</button>" +
    '<span class="muted">' +
    page +
    "/" +
    pages +
    "</span>" +
    '<button id="' +
    id +
    '-next"' +
    (page >= pages ? " disabled" : "") +
    ">Sau →</button>" +
    "</div></div>"
  );
}

async function pageOverview(main) {
  main.innerHTML = '<p class="loading">Đang tải Cluster Dashboard…</p>';
  const d = await api("/api/v1/rancher/cluster/dashboard");
  const c = d.counts || {};
  const cap = d.capacity || {};
  const pods = cap.pods || {};
  const cpu = cap.cpu || {};
  const mem = cap.memory || {};

  let eventsHtml = "";
  if (d.recent_events && d.recent_events.length) {
    eventsHtml = renderTable(
      [
        { key: "status", label: "Type" },
        { key: "reason", label: "Reason" },
        { key: "object", label: "Object" },
        { key: "message", label: "Message" },
        { key: "created", label: "Seen", render: (r) => esc(fmtTime(r.created)) },
      ],
      d.recent_events
    );
  } else {
    eventsHtml = '<p class="muted">Không có events gần đây.</p>';
  }

  const barItems = [
    { label: "Pods", value: c.pods || 0 },
    { label: "Deploy", value: c.deployments || 0 },
    { label: "Svc", value: c.services || 0 },
    { label: "NS", value: c.namespaces || 0 },
    { label: "Ing", value: c.ingresses || 0 },
    { label: "Nodes", value: c.nodes || 0 },
  ];

  const comps = (d.components || [])
    .map((x) => compCard(x))
    .join("");

  main.innerHTML =
    '<div class="page-header">' +
    '<h2 class="page-title">Cluster Dashboard</h2>' +
    '<p class="page-subtitle">' +
    esc(d.name || d.cluster_id) +
    ' · <span class="pill">' +
    esc(d.state || "active") +
    "</span></p></div>" +
    '<div class="meta-chips">' +
    chip("Provider", d.provider || "RKE2") +
    chip("Kubernetes", d.k8s_version || "—") +
    chip("Cluster", d.cluster_id) +
    "</div>" +
    '<div class="stat-grid">' +
    statBox(c.resources || 0, "Total Resources", "g1") +
    statBox(c.nodes || 0, "Nodes", "g2") +
    statBox(c.deployments || 0, "Deployments", "g3") +
    statBox(c.pods || 0, "Pods", "g4") +
    statBox(c.namespaces || 0, "Namespaces", "g5") +
    statBox(c.services || 0, "Services", "g6") +
    "</div>" +
    '<div class="dash-grid">' +
    '<div class="card"><h3>Capacity Overview</h3>' +
    '<div class="cap-donuts">' +
    svgDonut(pods.used_pct || 0, 110, "#8b5cf6", "Pods", (pods.used || 0) + "/" + (pods.total || 0)) +
    svgDonut(cpu.used_pct || 0, 110, "#22d3ee", "CPU Used", (cpu.used || 0) + " / " + (cpu.total || 0) + " cores") +
    svgDonut(mem.used_pct || 0, 110, "#ec4899", "Memory", (mem.used || 0) + " / " + (mem.total || 0) + " GiB") +
    "</div>" +
    '<div class="cap-sub">' +
    meterRow("CPU Reserved", cpu.reserved, cpu.total, cpu.reserved_pct, "reserved") +
    meterRow("Memory Reserved", mem.reserved, mem.total, mem.reserved_pct, "reserved") +
    "</div></div>" +
    '<div class="card"><h3>Component Status</h3>' +
    '<div class="comp-grid">' +
    comps +
    "</div></div></div>" +
    '<div class="dash-grid-bottom">' +
    '<div class="card"><h3>Resource Distribution</h3>' +
    svgBarChart(barItems, 180, 420) +
    "</div>" +
    '<div class="card"><h3>Recent Events <a href="#/events" class="link-muted">Xem tất cả →</a></h3>' +
    eventsHtml +
    "</div></div>";
}

function svgDonut(pct, size, color, title, sub) {
  const stroke = 10;
  const r = (size - stroke) / 2;
  const cx = size / 2;
  const c = 2 * Math.PI * r;
  const p = Math.min(Math.max(Number(pct) || 0, 0), 100);
  const off = c * (1 - p / 100);
  return (
    '<div class="cap-donut-item">' +
    '<svg width="' + size + '" height="' + size + '" viewBox="0 0 ' + size + " " + size + '">' +
    '<circle cx="' + cx + '" cy="' + cx + '" r="' + r + '" fill="none" stroke="rgba(255,255,255,0.07)" stroke-width="' + stroke + '"/>' +
    '<circle cx="' + cx + '" cy="' + cx + '" r="' + r + '" fill="none" stroke="' + color + '" stroke-width="' + stroke + '"' +
    ' stroke-dasharray="' + c.toFixed(1) + '" stroke-dashoffset="' + off.toFixed(1) + '"' +
    ' stroke-linecap="round" transform="rotate(-90 ' + cx + " " + cx + ')"/>' +
    '<text x="' + cx + '" y="' + (cx - 2) + '" text-anchor="middle" fill="#fff" font-size="18" font-weight="700">' + Math.round(p) + "%</text>" +
    "</svg>" +
    "<h4>" + esc(title) + "</h4>" +
    (sub ? "<p>" + esc(String(sub)) + "</p>" : "") +
    "</div>"
  );
}

function svgDonutSmall(pct, size, color) {
  const stroke = 6;
  const r = (size - stroke) / 2;
  const cx = size / 2;
  const c = 2 * Math.PI * r;
  const p = Math.min(Math.max(Number(pct) || 0, 0), 100);
  const off = c * (1 - p / 100);
  return (
    '<svg width="' + size + '" height="' + size + '" viewBox="0 0 ' + size + " " + size + '">' +
    '<circle cx="' + cx + '" cy="' + cx + '" r="' + r + '" fill="none" stroke="rgba(255,255,255,0.08)" stroke-width="' + stroke + '"/>' +
    '<circle cx="' + cx + '" cy="' + cx + '" r="' + r + '" fill="none" stroke="' + color + '" stroke-width="' + stroke + '"' +
    ' stroke-dasharray="' + c.toFixed(1) + '" stroke-dashoffset="' + off.toFixed(1) + '"' +
    ' stroke-linecap="round" transform="rotate(-90 ' + cx + " " + cx + ')"/>' +
    "</svg>"
  );
}

function svgBarChart(items, h, w) {
  const max = Math.max.apply(null, items.map(function (i) { return i.value; }).concat([1]));
  const n = items.length;
  const barW = Math.max(24, (w - 50) / n - 10);
  let rects = "";
  items.forEach(function (it, i) {
    const bh = Math.max(4, (it.value / max) * (h - 50));
    const x = 28 + i * (barW + 10);
    const y = h - 28 - bh;
    rects +=
      '<rect x="' + x + '" y="' + y + '" width="' + barW + '" height="' + bh + '" rx="5" fill="url(#barGrad)" opacity="0.9"/>' +
      '<text x="' + (x + barW / 2) + '" y="' + (h - 8) + '" text-anchor="middle" fill="#94a3b8" font-size="10">' + esc(it.label) + "</text>" +
      '<text x="' + (x + barW / 2) + '" y="' + (y - 4) + '" text-anchor="middle" fill="#eef2ff" font-size="10">' + it.value + "</text>";
  });
  return (
    '<div class="bar-chart"><svg viewBox="0 0 ' + w + " " + h + '" preserveAspectRatio="xMidYMid meet">' +
    "<defs><linearGradient id=\"barGrad\" x1=\"0\" y1=\"1\" x2=\"0\" y2=\"0\">" +
    '<stop offset="0%" stop-color="#6d28d9"/><stop offset="100%" stop-color="#22d3ee"/>' +
    "</linearGradient></defs>" + rects + "</svg></div>"
  );
}

function compCard(comp) {
  const ok = comp.status === "ok";
  const color = ok ? "#34d399" : "#f87171";
  return (
    '<div class="comp-card ' + esc(comp.status) + '">' +
    svgDonutSmall(ok ? 100 : 40, 56, color) +
    '<div class="comp-name">' + esc(comp.name) + "</div>" +
    '<div class="comp-msg">' + esc(comp.message || comp.status) + "</div></div>"
  );
}

function chip(label, value) {
  return (
    '<div class="chip"><span class="chip-label">' +
    esc(label) +
    '</span><span class="chip-value">' +
    esc(value) +
    "</span></div>"
  );
}

function meterRow(label, used, total, pct, tone) {
  const p = pct != null ? pct : total ? Math.round((used / total) * 100) : 0;
  return (
    '<div class="meter">' +
    '<div class="meter-head"><span>' + esc(label) + "</span><span>" +
    esc(used) + " / " + esc(total) + " (" + Math.round(p) + "%)</span></div>" +
    '<div class="meter-track"><i class="meter-fill ' + (tone || "used") +
    '" style="width:' + Math.min(p, 100) + '%"></i></div></div>'
  );
}

function statBox(n, label, grad) {
  return (
    '<div class="stat-box ' + (grad || "g1") + '">' +
    '<div class="lbl">' + esc(label) + '</div><div class="num">' + n + "</div></div>"
  );
}

async function pageRancherList(main, title, path, columns) {
  main.innerHTML = '<p class="loading">Đang tải ' + esc(title) + "…</p>";
  const data = await api(path);
  const rows = data.items || data;
  main.innerHTML =
    '<h2 class="page-title">' +
    esc(title) +
    ' <span class="muted">(' +
    (data.total != null ? data.total : rows.length) +
    ")</span></h2>" +
    renderTable(columns, Array.isArray(rows) ? rows : []);
}

function k8sColumns(resource, data) {
  if (resource === "events") {
    return [
      {
        key: "status",
        label: "Type",
        render: (r) =>
          r.status === "Warning"
            ? '<span class="badge badge-warn">' + esc(r.status) + "</span>"
            : '<span class="badge badge-ok">' + esc(r.status || "Normal") + "</span>",
      },
      { key: "reason", label: "Reason" },
      { key: "object", label: "Object" },
      { key: "namespace", label: "Namespace" },
      { key: "message", label: "Message" },
      { key: "created", label: "Last Seen", render: (r) => esc(fmtTime(r.created)) },
    ];
  }
  const cols = [
    { key: "name", label: "Name" },
    { key: "namespace", label: "Namespace" },
    {
      key: "status",
      label: "Status",
      render: (r) =>
        r.status
          ? '<span class="badge badge-ok">' + esc(r.status) + "</span>"
          : "—",
    },
    { key: "created", label: "Age", render: (r) => esc(fmtTime(r.created)) },
  ];
  if (!data.items || !data.items.some((i) => i.namespace)) {
    cols.splice(1, 1);
  }
  return cols;
}

async function pageK8s(main, resource, label, page, limit) {
  const route = resource;
  page = page || state.page[route] || 1;
  limit = limit || state.limit;
  state.page[route] = page;
  state.limit = limit;

  main.innerHTML = '<p class="loading">Đang tải ' + esc(label) + "…</p>";
  const data = await api(
    "/api/v1/k8s/" + resource + "?page=" + page + "&limit=" + limit
  );
  const cols = k8sColumns(resource, data);
  const onPage = (p, l) => pageK8s(main, resource, label, p, l);

  main.innerHTML =
    '<h2 class="page-title">' +
    esc(label) +
    ' <span class="muted">(' +
    data.total +
    ")</span></h2>" +
    renderTable(cols, data.items || []) +
    renderPagination(route, data.total, data.page, data.limit, onPage);
}

const routes = {
  overview: (main) => pageOverview(main),
  clusters: (main) =>
    pageRancherList(main, "Clusters", "/api/v1/rancher/clusters", [
      { key: "name", label: "Name" },
      { key: "id", label: "ID" },
      {
        key: "state",
        label: "State",
        render: (r) => '<span class="badge badge-ok">' + esc(r.state) + "</span>",
      },
    ]),
  projects: (main) =>
    pageRancherList(main, "Projects", "/api/v1/rancher/projects", [
      { key: "name", label: "Name" },
      { key: "id", label: "ID" },
      { key: "state", label: "State" },
      { key: "description", label: "Description" },
    ]),
};

function getRoute() {
  return location.hash.replace(/^#\/?/, "") || "overview";
}

async function navigate() {
  const route = getRoute();
  document.querySelectorAll(".nav-link").forEach((el) => {
    el.classList.toggle("active", el.dataset.route === route);
  });
  const main = $("#main");
  try {
    if (routes[route]) {
      await routes[route](main);
      return;
    }
    const menu = await api("/api/v1/explorer/menu");
    const item = menu.find((m) => m.key === route);
    if (item && item.type === "k8s") {
      await pageK8s(main, item.key, item.label);
      return;
    }
    main.innerHTML = '<p class="error">Không tìm thấy trang: ' + esc(route) + "</p>";
  } catch (e) {
    main.innerHTML = '<p class="error">Lỗi: ' + esc(e.message) + "</p>";
  }
}

function groupCollapsed(group) {
  const key = "nav-collapsed-" + group;
  return localStorage.getItem(key) === "1";
}

function setGroupCollapsed(group, collapsed) {
  localStorage.setItem("nav-collapsed-" + group, collapsed ? "1" : "0");
}

const NAV_ICONS = {
  overview: "◉", clusters: "◎", projects: "▣", namespaces: "▤", nodes: "⬡",
  events: "⚡", deployments: "▶", statefulsets: "▧", daemonsets: "▨",
  jobs: "⏱", cronjobs: "↻", pods: "●", services: "🔗", ingresses: "🌐",
  horizontalpodautoscalers: "📈", persistentvolumeclaims: "💾",
  persistentvolumes: "🗄", storageclasses: "📂", configmaps: "⚙",
  secrets: "🔒",
};

async function buildSidebar() {
  const nav = $("#sidebar-nav");
  const menu = await api("/api/v1/explorer/menu");
  const groups = {};
  menu.forEach((item) => {
    if (!groups[item.group]) groups[item.group] = [];
    groups[item.group].push(item);
  });

  const order = ["Platform", "Cluster", "Workloads", "Networking", "Storage", "Config"];
  let html = "";
  for (const group of order) {
    if (!groups[group]) continue;
    const collapsed = groupCollapsed(group);
    html +=
      '<div class="nav-group">' +
      '<button type="button" class="nav-group-toggle" data-group="' + esc(group) + '">' +
      '<span class="chev">' + (collapsed ? "▸" : "▾") + "</span>" + esc(group) +
      "</button>" +
      '<div class="nav-group-items' + (collapsed ? " collapsed" : "") + '">';
    groups[group].forEach((item) => {
      const ico = NAV_ICONS[item.key] || "·";
      html +=
        '<a class="nav-link" data-route="' + esc(item.key) + '" href="#/' + esc(item.key) + '">' +
        '<span class="ico">' + ico + "</span>" + esc(item.label) + "</a>";
    });
    html += "</div></div>";
  }
  nav.innerHTML = html;

  nav.querySelectorAll(".nav-group-toggle").forEach((btn) => {
    btn.onclick = () => {
      const g = btn.dataset.group;
      const items = btn.nextElementSibling;
      const nowCollapsed = !items.classList.contains("collapsed");
      items.classList.toggle("collapsed", nowCollapsed);
      btn.querySelector(".chev").textContent = nowCollapsed ? "▸" : "▾";
      setGroupCollapsed(g, nowCollapsed);
    };
  });
}

window.addEventListener("hashchange", navigate);
buildSidebar().then(navigate);
