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

  const comps = (d.components || [])
    .map((x) => '<span class="comp ok">' + esc(x.name) + "</span>")
    .join("");

  main.innerHTML =
    '<div class="page-header">' +
    '<h2 class="page-title">Cluster Dashboard</h2>' +
    '<p class="page-subtitle">' +
    esc(d.name || d.cluster_id) +
    ' · <span class="pill">' +
    esc(d.state || "—") +
    "</span></p>" +
    "</div>" +
    '<div class="meta-chips">' +
    chip("Provider", d.provider || "RKE2") +
    chip("Kubernetes", d.k8s_version || "—") +
    chip("Cluster ID", d.cluster_id) +
    "</div>" +
    '<div class="stat-grid">' +
    statBox(c.resources || 0, "Resources", "📦") +
    statBox(c.nodes || 0, "Nodes", "🖥") +
    statBox(c.deployments || 0, "Deployments", "🚀") +
    statBox(c.pods || 0, "Pods", "⬡") +
    statBox(c.namespaces || 0, "Namespaces", "📁") +
    statBox(c.services || 0, "Services", "🔗") +
    "</div>" +
    '<div class="card card-glass"><h3>Capacity</h3>' +
    renderCapacity(cap) +
    "</div>" +
    '<div class="card card-glass"><h3>Components</h3><div class="components">' +
    comps +
    "</div></div>" +
    '<div class="card card-glass"><h3>Recent Events <a href="#/events" class="link-muted">Xem tất cả →</a></h3>' +
    eventsHtml +
    "</div>";
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
    '<div class="meter-head"><span>' +
    esc(label) +
    "</span><span>" +
    esc(used) +
    " / " +
    esc(total) +
    " <strong>(" +
    p +
    "%)</strong></span></div>" +
    '<div class="meter-track"><i class="meter-fill ' +
  (tone || "used") +
    '" style="width:' +
    Math.min(p, 100) +
    '%"></i></div></div>'
  );
}

function renderCapacity(cap) {
  const pods = cap.pods || {};
  const cpu = cap.cpu || {};
  const mem = cap.memory || {};
  return (
    '<div class="cap-grid">' +
    '<div class="cap-card"><h4>Pods</h4>' +
    meterRow("Used", pods.used || 0, pods.total || 0, pods.used_pct) +
    "</div>" +
    '<div class="cap-card"><h4>CPU</h4>' +
    meterRow("Reserved", cpu.reserved || 0, cpu.total || 0, cpu.reserved_pct, "reserved") +
    meterRow("Used", cpu.used || 0, cpu.total || 0, cpu.used_pct, "used") +
    '<div class="cap-unit">' +
    esc(cpu.total || 0) +
    " cores</div></div>" +
    '<div class="cap-card"><h4>Memory</h4>' +
    meterRow("Reserved", mem.reserved || 0, mem.total || 0, mem.reserved_pct, "reserved") +
    meterRow("Used", mem.used || 0, mem.total || 0, mem.used_pct, "used") +
    '<div class="cap-unit">' +
    esc(mem.total || 0) +
    " GiB</div></div></div>"
  );
}

function statBox(n, label, icon) {
  return (
    '<div class="stat-box"><div class="stat-icon">' +
    (icon || "") +
    '</div><div class="num">' +
    n +
    '</div><div class="lbl">' +
    esc(label) +
    "</div></div>"
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
      '<div class="nav-group" data-group="' +
      esc(group) +
      '">' +
      '<button type="button" class="nav-group-toggle" data-group="' +
      esc(group) +
      '">' +
      '<span class="chev">' +
      (collapsed ? "▸" : "▾") +
      "</span>" +
      esc(group) +
      "</button>" +
      '<div class="nav-group-items' +
      (collapsed ? " collapsed" : "") +
      '">';
    groups[group].forEach((item) => {
      html +=
        '<a class="nav-link" data-route="' +
        esc(item.key) +
        '" href="#/' +
        esc(item.key) +
        '">' +
        esc(item.label) +
        "</a>";
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
