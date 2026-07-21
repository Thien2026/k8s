function updateSidebarBrand(ctx) {
  const h1 = document.querySelector(".sidebar-brand h1");
  const sub = document.querySelector(".sidebar-brand p");
  const layout = document.querySelector(".layout");
  if (!h1 || !sub) return;
  if (ctx.mode === "addons") {
    layout?.classList.add("sidebar-project-mode");
    layout?.classList.add("sidebar-addon-mode");
    h1.textContent = ctx.name || ctx.slug;
    sub.innerHTML =
      "<code>" + esc(ctx.slug) + "</code> · Addons" +
      (ctx.engine ? ' · <span class="addon-brand-engine">' + esc(ctx.engine) + "</span>" : "");
  } else if (ctx.mode === "project") {
    layout?.classList.add("sidebar-project-mode");
    layout?.classList.remove("sidebar-addon-mode");
    h1.textContent = ctx.name || ctx.slug;
    sub.innerHTML = "<code>" + esc(ctx.slug) + "</code> · Project";
  } else {
    layout?.classList.remove("sidebar-project-mode");
    layout?.classList.remove("sidebar-addon-mode");
    h1.textContent = "Platform Console";
    sub.textContent = "K8s Explorer";
  }
}

function renderSidebarUser() {
  let userBar = document.getElementById("sidebar-user");
  const sidebar = document.querySelector(".sidebar");
  if (!userBar && sidebar) {
    userBar = document.createElement("div");
    userBar.id = "sidebar-user";
    userBar.className = "sidebar-user";
    const footer = sidebar.querySelector(".sidebar-footer");
    sidebar.insertBefore(userBar, footer);
  }
  if (userBar) {
    userBar.innerHTML =
      '<div class="sidebar-user-info"><strong>' + esc(state.user?.email || "") + "</strong>" +
      '<span class="muted">' + esc(roleLabel(state.user?.role || "")) + "</span></div>" +
      '<button type="button" class="btn-logout" id="btn-logout">Đăng xuất</button>';
    const logoutBtn = document.getElementById("btn-logout");
    if (logoutBtn) logoutBtn.onclick = logout;
  }
}

function bindSidebarEvents(nav) {
  nav.querySelectorAll(".nav-group-toggle").forEach(function (btn) {
    btn.onclick = function () {
      const g = btn.dataset.group;
      const items = btn.nextElementSibling;
      const nowCollapsed = !items.classList.contains("collapsed");
      items.classList.toggle("collapsed", nowCollapsed);
      btn.querySelector(".chev").textContent = nowCollapsed ? "▸" : "▾";
      setGroupCollapsed(g, nowCollapsed);
    };
  });
  nav.querySelectorAll(".nav-section-toggle").forEach(function (btn) {
    btn.onclick = function () {
      const sec = btn.dataset.section;
      const body = btn.nextElementSibling;
      const nowCollapsed = !body.classList.contains("collapsed");
      body.classList.toggle("collapsed", nowCollapsed);
      btn.querySelector(".chev").textContent = nowCollapsed ? "▸" : "▾";
      setSectionCollapsed(sec, nowCollapsed);
    };
  });
}

function buildAddonsSidebarHtml(nav, slug, engine, p, installed) {
  installed = installed || [];
  let html =
    '<a class="nav-back" href="' + projectRoute(slug, "overview") + '" title="Về project">' +
    '<span class="ico">←</span><span class="nav-label">Project</span></a>';

  html += '<div class="nav-section nav-section-addons">';
  html +=
    '<a class="nav-link" data-route="addons/' + esc(slug) + '" href="' + projectAddonsRoute(slug) + '" title="Catalog addons">' +
    '<span class="ico">🧩</span><span class="nav-label">Catalog</span></a>';

  const engines = [];
  installed.forEach(function (it) {
    const eng = String(it.engine || "").trim();
    if (eng && engines.indexOf(eng) < 0) engines.push(eng);
  });
  if (engine && engines.indexOf(engine) < 0) engines.push(engine);

  engines.forEach(function (eng) {
    const routeKey = "addons/" + slug + "/" + eng;
    html +=
      '<a class="nav-link" data-route="' + esc(routeKey) + '" href="' + projectAddonsRoute(slug, eng) + '" title="' + esc(eng) + '">' +
      '<span class="ico">' + addonIcon(eng) + '</span><span class="nav-label">' + esc(eng) + "</span></a>";
  });

  html += "</div>";

  html +=
    '<div class="project-sidebar-meta">' +
    chip("Dev", p.namespace_dev) +
    chip("Prod", p.namespace_prod) +
    "</div>";

  nav.innerHTML = html;
  nav.classList.remove("loading");
}

function buildProjectSidebarHtml(nav, slug, tab, p) {
  let html =
    '<a class="nav-back" href="' + platformHomeHash() + '" title="Về Platform">' +
    '<span class="ico">←</span><span class="nav-label">Platform</span></a>';

  html += '<div class="nav-section nav-section-project">';
  PROJECT_NAV_GROUPS.forEach(function (grp) {
    const collapsed = projectGroupCollapsed(grp.group, grp.defaultCollapsed);
    html +=
      '<div class="nav-group nav-group-project">' +
      '<button type="button" class="nav-group-toggle" data-group="' +
      esc(grp.group) +
      '">' +
      '<span class="chev">' +
      (collapsed ? "▸" : "▾") +
      "</span>" +
      esc(grp.label) +
      "</button>" +
      '<div class="nav-group-items' +
      (collapsed ? " collapsed" : "") +
      '">';
    grp.items.forEach(function (item) {
      if (item.adminOnly && !canManagePlatformProjects()) {
        return;
      }
      const routeKey = "project/" + slug + "/" + item.tab;
      html +=
        '<a class="nav-link" data-route="' +
        esc(routeKey) +
        '" href="' +
        projectRoute(slug, item.tab) +
        '" title="' +
        esc(item.label) +
        '">' +
        '<span class="ico">' +
        item.icon +
        '</span><span class="nav-label">' +
        esc(item.label) +
        "</span></a>";
    });
    html += "</div></div>";
  });
  html += "</div>";

  html +=
    '<div class="project-sidebar-meta">' +
    chip("Dev", p.namespace_dev) +
    chip("Prod", p.namespace_prod) +
    "</div>";

  nav.innerHTML = html;
  nav.classList.remove("loading");
}

async function buildPlatformSidebar(nav) {
  const menu = await api("/api/v1/explorer/menu");
  const sections = { platform: [], workspace: [], infra: [] };
  menu.forEach(function (item) {
    const sec = item.section || (item.group === "Platform" ? "platform" : "infra");
    if (!sections[sec]) sections[sec] = [];
    sections[sec].push(item);
  });

  const infraGroupOrder = ["Cluster", "Vận hành", "Workloads", "Networking", "Storage", "Config"];
  const workspaceGroupOrder = ["Vận hành", "Workloads", "Networking"];
  let html = "";

  function renderGroup(group, items) {
    const collapsed = groupCollapsed(group);
    let g =
      '<div class="nav-group">' +
      '<button type="button" class="nav-group-toggle" data-group="' + esc(group) + '">' +
      '<span class="chev">' + (collapsed ? "▸" : "▾") + "</span>" + esc(group) +
      "</button>" +
      '<div class="nav-group-items' + (collapsed ? " collapsed" : "") + '">';
    items.forEach(function (item) {
      const ico = NAV_ICONS[item.key] || "·";
      g +=
        '<a class="nav-link" data-route="' + esc(item.key) + '" href="#/' + esc(item.key) + '" title="' + esc(item.label) + '">' +
        '<span class="ico">' + ico + '</span><span class="nav-label">' + esc(item.label) + "</span></a>";
    });
    return g + "</div></div>";
  }

  if (sections.platform.length) {
    const secCollapsed = sectionCollapsed("platform");
    html +=
      '<div class="nav-section">' +
      '<button type="button" class="nav-section-toggle" data-section="platform">' +
      '<span class="chev">' + (secCollapsed ? "▸" : "▾") + "</span>" +
      esc(SECTION_LABELS.platform) +
      "</button>" +
      '<div class="nav-section-body' + (secCollapsed ? " collapsed" : "") + '">';
    sections.platform.forEach(function (item) {
      const ico = NAV_ICONS[item.key] || "·";
      html +=
        '<a class="nav-link" data-route="' + esc(item.key) + '" href="#/' + esc(item.key) + '" title="' + esc(item.label) + '">' +
        '<span class="ico">' + ico + '</span><span class="nav-label">' + esc(item.label) + "</span></a>";
    });
    html += "</div></div>";
  }

  if (sections.workspace.length) {
    const secCollapsed = sectionCollapsed("workspace");
    html +=
      '<div class="nav-section nav-section-workspace">' +
      '<button type="button" class="nav-section-toggle" data-section="workspace">' +
      '<span class="chev">' + (secCollapsed ? "▸" : "▾") + "</span>" +
      esc(SECTION_LABELS.workspace) +
      "</button>" +
      '<div class="nav-section-body' + (secCollapsed ? " collapsed" : "") + '">';
    const groups = {};
    sections.workspace.forEach(function (item) {
      if (!groups[item.group]) groups[item.group] = [];
      groups[item.group].push(item);
    });
    for (const group of workspaceGroupOrder) {
      if (groups[group]) html += renderGroup(group, groups[group]);
    }
    Object.keys(groups).forEach(function (group) {
      if (!workspaceGroupOrder.includes(group)) html += renderGroup(group, groups[group]);
    });
    html += "</div></div>";
  }

  if (sections.infra.length) {
    const secCollapsed = sectionCollapsed("infra");
    html +=
      '<div class="nav-section nav-section-infra">' +
      '<button type="button" class="nav-section-toggle" data-section="infra">' +
      '<span class="chev">' + (secCollapsed ? "▸" : "▾") + "</span>" +
      esc(SECTION_LABELS.infra) +
      "</button>" +
      '<div class="nav-section-body' + (secCollapsed ? " collapsed" : "") + '">';
    const groups = {};
    sections.infra.forEach(function (item) {
      if (!groups[item.group]) groups[item.group] = [];
      groups[item.group].push(item);
    });
    for (const group of infraGroupOrder) {
      if (groups[group]) html += renderGroup(group, groups[group]);
    }
    html += "</div></div>";
  }

  nav.innerHTML = html;
  nav.classList.remove("loading");
}

async function buildSidebarForRoute(parsed) {
  const nav = $("#sidebar-nav");
  if (!nav) return;

  if (parsed.type === "project") {
    let p = state.projectCtx;
    if (!p || p.slug !== parsed.slug) {
      try {
        const data = await api("/api/v1/projects/" + encodeURIComponent(parsed.slug));
        p = data.project;
        state.projectCtx = p;
      } catch (_) {
        p = { slug: parsed.slug, name: parsed.slug, namespace_dev: "—", namespace_prod: "—" };
      }
    }
    if (parsed.tab === "addons") {
      let installed = [];
      try {
        const ad = await api("/api/v1/projects/" + encodeURIComponent(parsed.slug) + "/addons");
        installed = ad.items || [];
      } catch (_) {}
      updateSidebarBrand({ mode: "addons", name: p.name, slug: p.slug, engine: parsed.addon || "" });
      buildAddonsSidebarHtml(nav, parsed.slug, parsed.addon || "", p, installed);
    } else {
      updateSidebarBrand({ mode: "project", name: p.name, slug: p.slug });
      buildProjectSidebarHtml(nav, parsed.slug, parsed.tab || "overview", p);
    }
  } else {
    state.projectCtx = null;
    updateSidebarBrand({ mode: "platform" });
    await buildPlatformSidebar(nav);
  }

  renderSidebarUser();
  bindSidebarEvents(nav);
  markActiveNav(parsed);
}

async function buildSidebar() {
  await buildSidebarForRoute(parseRoute());
}

/* --- nav collapse / sidebar mode --- */

function groupCollapsed(group) {
  const key = "nav-collapsed-" + group;
  return localStorage.getItem(key) === "1";
}

function projectGroupCollapsed(group, defaultCollapsed) {
  const key = "nav-collapsed-" + group;
  const stored = localStorage.getItem(key);
  if (stored === null) return !!defaultCollapsed;
  return stored === "1";
}

function projectNavActiveTab(tab, addon) {
  tab = tab || "overview";
  if (tab === "addons") {
    return addon ? "addons/" + addon : "addons";
  }
  if (PROJECT_WORKLOADS.some(function (w) { return w.tab === tab; })) {
    return "runtime";
  }
  return tab;
}

function setGroupCollapsed(group, collapsed) {
  localStorage.setItem("nav-collapsed-" + group, collapsed ? "1" : "0");
}

const NAV_ICONS = {
  overview: "◉", "my-projects": "▣", "platform-projects": "＋", addons: "🧩", policy: "🛡", backups: "▣", gitops: "⎇", audit: "📋", users: "👤", "add-worker": "⊕", clusters: "◎", projects: "▣", namespaces: "▤", nodes: "⬡",
  events: "⚡", deployments: "▶", statefulsets: "▧", daemonsets: "▨",
  jobs: "⏱", cronjobs: "↻", pods: "●", services: "🔗", ingresses: "🌐",
  horizontalpodautoscalers: "📈", persistentvolumeclaims: "💾",
  persistentvolumes: "🗄", storageclasses: "📂", configmaps: "⚙",
  secrets: "🔒",
  "k8s-ops": "⌘",
};

function sectionCollapsed(section) {
  return localStorage.getItem("nav-section-" + section) === "1";
}

function setSectionCollapsed(section, collapsed) {
  localStorage.setItem("nav-section-" + section, collapsed ? "1" : "0");
}

const SECTION_LABELS = { platform: "Platform", workspace: "Dự án", infra: "Hạ tầng" };


function sidebarMode() {
  return localStorage.getItem("sidebar-mode") || "expanded";
}

function setSidebarMode(mode) {
  localStorage.setItem("sidebar-mode", mode);
  applySidebarMode();
}

function applySidebarMode() {
  const layout = document.querySelector(".layout");
  const btn = document.getElementById("sidebar-toggle");
  const fab = document.getElementById("sidebar-fab");
  if (!layout) return;
  layout.classList.remove("sidebar-collapsed", "sidebar-hidden");
  const mode = sidebarMode();
  if (mode === "collapsed") {
    layout.classList.add("sidebar-collapsed");
    if (btn) btn.textContent = "›";
    if (btn) btn.title = "Mở rộng sidebar";
  } else if (mode === "hidden") {
    layout.classList.add("sidebar-hidden");
    if (fab) fab.style.display = "block";
    if (btn) btn.textContent = "‹";
  } else {
    if (btn) btn.textContent = "‹";
    if (btn) btn.title = "Thu gọn sidebar";
    if (fab) fab.style.display = "none";
  }
}

function initSidebarToggle() {
  const btn = document.getElementById("sidebar-toggle");
  const fab = document.getElementById("sidebar-fab");
  if (btn) {
    btn.onclick = () => {
      const mode = sidebarMode();
      if (mode === "expanded") setSidebarMode("collapsed");
      else if (mode === "collapsed") setSidebarMode("hidden");
      else setSidebarMode("expanded");
    };
  }
  if (fab) {
    fab.onclick = () => setSidebarMode("expanded");
  }
  applySidebarMode();
}
