/* Shared UI helpers. */

function resourceLink(resource, row) {
  const ns = row.namespace || "_";
  return (
    '<a class="res-link" href="#/view/' +
    esc(resource) +
    "/" +
    esc(ns) +
    "/" +
    esc(row.name) +
    '">' +
    esc(row.name) +
    "</a>"
  );
}

function renderTable(columns, rows, resource) {
  if (!rows.length) {
    return '<p class="muted">Không có dữ liệu.</p>';
  }
  const head = columns.map((c) => "<th>" + esc(c.label) + "</th>").join("");
  const body = rows
    .map((row) => {
      const cells = columns
        .map((c) => {
          if (c.key === "name" && resource && row.name) {
            return "<td>" + resourceLink(resource, row) + "</td>";
          }
          return "<td>" + (c.render ? c.render(row) : esc(row[c.key] ?? "—")) + "</td>";
        })
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

function projectNsLink(resource, ns, label) {
  if (!ns || ns === "—") return "";
  return (
    '<a href="#/' + esc(resource) + '" class="btn-ghost project-ns-link" data-resource="' + esc(resource) + '" data-ns="' + esc(ns) + '">' +
    esc(label) +
    "</a> "
  );
}

function defaultHomeRoute() {
  const u = state.user;
  if (!u) return "overview";
  if (u.role === "dev" || u.role === "readonly") return "my-projects";
  return "overview";
}

function canWriteK8s() {
  const r = state.user && state.user.role;
  return r === "admin" || r === "tech_lead" || r === "dev";
}

function platformHomeHash() {
  return canManagePlatformProjects() ? "#/platform-projects" : "#/my-projects";
}

function projectRoute(slug, tab) {
  return "#/project/" + slug + (tab && tab !== "overview" ? "/" + tab : "");
}

function projectAddonsRoute(slug, engine) {
  let path = "#/project/" + slug + "/addons";
  if (engine) path += "/" + engine;
  return path;
}

const ADDON_SIDEBAR_NAV = {
  redis: [
    { section: "overview", label: "Tổng quan", icon: "◉" },
    { section: "connection", label: "Connection", icon: "🔗" },
    { section: "quota", label: "Quota", icon: "📊" },
  ],
};

const PROJECT_NAV_GROUPS = [
  {
    group: "project-quan-sat",
    label: "Quan sát",
    defaultCollapsed: false,
    items: [
      { tab: "overview", label: "Tổng quan", icon: "◉" },
      { tab: "monitoring", label: "Monitoring", icon: "📈" },
    ],
  },
  {
    group: "project-trien-khai",
    label: "Triển khai",
    defaultCollapsed: false,
    items: [
      { tab: "deploy", label: "Deploy / Git", icon: "⎇" },
      { tab: "deploy-history", label: "Lịch sử deploy", icon: "🕘" },
      { tab: "promote", label: "Promote Prod", icon: "↑", adminOnly: true },
      { tab: "runtime", label: "Runtime", icon: "▶" },
    ],
  },
  {
    group: "project-van-hanh",
    label: "Vận hành",
    defaultCollapsed: true,
    items: [{ tab: "ops", label: "Sổ lệnh K8s", icon: "⌘" }],
  },
  {
    group: "project-cau-hinh",
    label: "Cấu hình",
    defaultCollapsed: true,
    items: [
      { tab: "addons", label: "Addons", icon: "🧩" },
      { tab: "domains", label: "Domains", icon: "🌐" },
      { tab: "env", label: "Cấu hình app", icon: "🔑" },
      { tab: "settings", label: "Cài đặt", icon: "⚙" },
    ],
  },
];

/** Routes vẫn hoạt động — không hiện sidebar (xem qua Runtime / Sổ lệnh K8s). */
const PROJECT_WORKLOADS = [
  { tab: "pods", label: "Pods", icon: "●" },
  { tab: "deployments", label: "Deployments", icon: "▶" },
  { tab: "services", label: "Services", icon: "🔗" },
  { tab: "ingresses", label: "Ingresses", icon: "🌐" },
];

const GRAFANA_NS_DASH_UID = "85a562078cdf77779eaa1add43ccec1e";

function grafanaNamespaceDashboardUrl(baseUrl, namespace) {
  if (!baseUrl || !namespace) return "";
  const base = String(baseUrl).replace(/\/+$/, "");
  const q = new URLSearchParams({
    "var-namespace": namespace,
    orgId: "1",
    from: "now-6h",
    to: "now",
    timezone: "browser",
  });
  return base + "/d/" + GRAFANA_NS_DASH_UID + "/kubernetes-compute-resources-namespace-pods?" + q.toString();
}

function projectHeader(p, subtitle, opts) {
  opts = opts || {};
  let helpBtn = "";
  if (opts.help === "deploy") helpBtn = renderDeployHelpButton("steps", "btn-help-header");
  if (opts.help === "k8sops") helpBtn = renderK8sOpsHelpButton("start", "btn-help-header");
  return (
    '<div class="page-header project-header page-header-row">' +
    '<div class="page-header-text">' +
    "<h2 class=\"page-title\">" + esc(p.name) + "</h2>" +
    '<p class="page-subtitle">' + esc(subtitle || p.slug) + "</p></div>" +
    helpBtn +
    "</div>"
  );
}

function copyText(text, okMsg) {
  navigator.clipboard.writeText(text || "").then(
    function () { toastSuccess(okMsg || "Đã copy"); },
    function () { toastError("Không copy được — chọn thủ công"); }
  );
}

function snippetBlock(id, title, content, copyLabel) {
  return (
    '<div class="snippet-block" style="margin-top:12px">' +
    '<div class="snippet-head">' +
    "<h4>" + esc(title) + "</h4>" +
    '<button type="button" class="btn-sm btn-copy-snippet" data-snippet-id="' + esc(id) + '">' +
    esc(copyLabel || "Copy") +
    "</button></div>" +
    '<pre class="yaml-box" id="' + esc(id) + '">' + esc(content) + "</pre></div>"
  );
}

function bindSnippetCopyButtons(root) {
  (root || document).querySelectorAll(".btn-copy-snippet").forEach(function (btn) {
    btn.onclick = function () {
      const el = document.getElementById(btn.dataset.snippetId);
      copyText(el ? el.textContent : "", "Đã copy");
    };
  });
}

function domainKindBadge(kind) {
  return kind === "auto"
    ? '<span class="badge ok">Tự động</span>'
    : '<span class="badge">Custom</span>';
}

function domainSyncBadge(status) {
  const s = status || "pending";
  if (s === "synced") return '<span class="badge ok">Ingress OK</span>';
  if (s === "error") return '<span class="badge warn">Lỗi sync</span>';
  return '<span class="badge">Chờ sync</span>';
}

function domainCertBadge(status, tls) {
  if (!tls) return '<span class="muted">—</span>';
  const s = status || "pending";
  if (s === "ready") return '<span class="badge ok">TLS Ready</span>';
  if (s === "failed") return '<span class="badge warn">TLS lỗi</span>';
  if (s === "n/a") return '<span class="muted">—</span>';
  return '<span class="badge">TLS pending</span>';
}

function renderDNSHint(dns) {
  if (!dns) return "";
  if (dns.mode === "auto") {
    return '<p class="muted dns-hint">' + esc(dns.message || "") + "</p>";
  }
  let html = '<div class="dns-hint-box"><p class="muted"><strong>Cấu hình DNS</strong></p>';
  if (dns.record_type && dns.record_value) {
    html +=
      "<p><code>" +
      esc(dns.record_type) +
      "</code> · <code>" +
      esc(dns.record_name || "@") +
      "</code> → <code>" +
      esc(dns.record_value) +
      "</code></p>";
  }
  if (dns.alt_type && dns.alt_value) {
    html +=
      '<p class="muted">Hoặc <code>' +
      esc(dns.alt_type) +
      "</code> → <code>" +
      esc(dns.alt_value) +
      "</code></p>";
  }
  if (dns.note) html += '<p class="muted">' + esc(dns.note) + "</p>";
  return html + "</div>";
}

function selectWrapHtml(id, optionsHtml, opts) {
  opts = opts || {};
  const cls = "select-wrap" + (opts.compact ? " select-wrap-compact" : "");
  const attrs = [];
  if (id) attrs.push('id="' + esc(id) + '"');
  if (opts.name) attrs.push('name="' + esc(opts.name) + '"');
  if (opts.required) attrs.push("required");
  return (
    '<div class="' + cls + '">' +
    "<select " + attrs.join(" ") + ">" +
    optionsHtml +
    '</select><span class="select-chev" aria-hidden="true">▾</span></div>'
  );
}

function setButtonLoading(btn, loading, label) {
  if (!btn) return;
  if (loading) {
    if (!btn.dataset.idleLabel) btn.dataset.idleLabel = btn.textContent;
    btn.disabled = true;
    btn.classList.add("is-loading");
    btn.innerHTML = '<span class="btn-spinner" aria-hidden="true"></span> ' + esc(label || "Đang xử lý…");
  } else {
    btn.disabled = false;
    btn.classList.remove("is-loading");
    btn.textContent = btn.dataset.idleLabel || label || "OK";
    delete btn.dataset.idleLabel;
  }
}

function bindEnvSegment(onChange) {
  setTimeout(function () {
    document.querySelectorAll(".env-segment .env-seg-btn").forEach(function (btn) {
      btn.onclick = function () {
        const env = btn.dataset.env;
        if (!env || state.projectEnv === env) return;
        state.projectEnv = env;
        localStorage.setItem("project-env", env);
        nextNavToken();
        document.querySelectorAll(".env-segment .env-seg-btn").forEach(function (b) {
          b.classList.toggle("active", b.dataset.env === env);
        });
        if (onChange) onChange();
        else location.reload();
      };
    });
  }, 0);
}

function deployActivityEnv(activity, fallback) {
  return (fallback || state.projectEnv || activity?.environment || "dev").toLowerCase();
}

function deployActivityEnvMatches(activity, expectedEnv) {
  if (!activity || activity.loading) return true;
  const want = deployActivityEnv(null, expectedEnv);
  const got = deployActivityEnv(activity, null);
  return want === got;
}

function projectEnvToolbar(slug, p, onChange) {
  const env = state.projectEnv || "dev";
  const ns = env === "prod" ? p.namespace_prod : p.namespace_dev;
  bindEnvSegment(onChange);
  return (
    '<div class="toolbar project-env-bar">' +
    '<div class="env-segment" role="group" aria-label="Môi trường">' +
    '<span class="env-segment-label">Môi trường</span>' +
    '<button type="button" class="env-seg-btn' + (env === "dev" ? " active" : "") + '" data-env="dev">Dev</button>' +
    '<button type="button" class="env-seg-btn' + (env === "prod" ? " active" : "") + '" data-env="prod">Prod</button>' +
    "</div>" +
    '<span class="muted project-env-ns">Namespace <code>' + esc(ns) + "</code></span></div>"
  );
}

function canManagePlatformProjects() {
  const r = state.user && state.user.role;
  return r === "admin" || r === "tech_lead";
}

function canDeleteProject() {
  return canManagePlatformProjects();
}

function listPage(title, total, body, toolbar) {
  return (
    '<div class="list-page">' +
    '<div class="list-header"><h2 class="page-title">' + esc(title) +
    ' <span class="muted count">(' + total + ")</span></h2>" +
    (toolbar || "") +
    "</div>" +
    '<div class="card list-card">' + body + "</div></div>"
  );
}

async function pageRancherList(main, title, path, columns) {
  main.innerHTML = '<p class="loading">Đang tải ' + esc(title) + "…</p>";
  const data = await api(path);
  const rows = data.items || data;
  const total = data.total != null ? data.total : rows.length;
  main.innerHTML = listPage(title, total, renderTable(columns, Array.isArray(rows) ? rows : []));
}

function badgeStatus(val) {
  if (!val) return "—";
  const v = String(val).toLowerCase();
  const ok = v.includes("active") || v.includes("running") || v.includes("ready=true") || v === "bound" || v === "succeeded";
  const warn = v.includes("pending") || v.includes("warning") || v.includes("progress");
  const cls = warn ? "badge-warn" : ok ? "badge-ok" : "badge-neutral";
  return '<span class="badge ' + cls + '">' + esc(val) + "</span>";
}
