const $ = (sel) => document.querySelector(sel);

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

async function pageOverview(main) {
  main.innerHTML = '<p class="loading">Đang tải tổng quan…</p>';
  const data = await api("/api/v1/dashboard");
  const h = data.health || {};
  const c = data.cluster || {};
  let clusterHtml = "—";
  if (c.connected && c.total != null) {
    clusterHtml =
      '<span class="ok">Clusters: <strong>' +
      c.ready +
      "/" +
      c.total +
      " ready</strong></span>";
  } else if (c.error) {
    clusterHtml = '<span class="error">' + esc(c.error) + "</span>";
  }
  main.innerHTML =
    '<h2 class="page-title">Tổng quan</h2>' +
    '<div class="card"><h3>API / DB</h3><p>' +
    (h.status === "ok"
      ? '<span class="ok">API <strong>ok</strong> · DB <strong>' + esc(h.database) + "</strong></span>"
      : '<span class="error">' + esc(h.error || h.status) + "</span>") +
    "</p></div>" +
    '<div class="card"><h3>Cluster (Rancher)</h3><p>' +
    clusterHtml +
    "</p></div>";
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

async function pageK8s(main, resource, label) {
  main.innerHTML = '<p class="loading">Đang tải ' + esc(label) + "…</p>";
  const data = await api("/api/v1/k8s/" + resource);
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
    {
      key: "created",
      label: "Age",
      render: (r) => esc(fmtTime(r.created)),
    },
  ];
  if (!data.items || !data.items.some((i) => i.namespace)) {
    cols.splice(1, 1);
  }
  main.innerHTML =
    '<h2 class="page-title">' +
    esc(label) +
    ' <span class="muted">(' +
    data.total +
    ")</span></h2>" +
    renderTable(cols, data.items || []);
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
  const hash = location.hash.replace(/^#\/?/, "") || "overview";
  return hash;
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

async function buildSidebar() {
  const nav = $("#sidebar-nav");
  const menu = await api("/api/v1/explorer/menu");
  const groups = {};
  menu.forEach((item) => {
    if (!groups[item.group]) groups[item.group] = [];
    groups[item.group].push(item);
  });
  let html = "";
  for (const [group, items] of Object.entries(groups)) {
    html += '<div class="nav-group"><div class="nav-group-title">' + esc(group) + "</div>";
    items.forEach((item) => {
      html +=
        '<a class="nav-link" data-route="' +
        esc(item.key) +
        '" href="#/' +
        esc(item.key) +
        '">' +
        esc(item.label) +
        "</a>";
    });
    html += "</div>";
  }
  nav.innerHTML = html;
}

window.addEventListener("hashchange", navigate);
buildSidebar().then(navigate);
