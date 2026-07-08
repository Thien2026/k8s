function getRoute() {
  return location.hash.replace(/^#\/?/, "") || defaultHomeRoute();
}

function parseRoute() {
  const raw = getRoute();
  const pathOnly = raw.split("?")[0];
  if (pathOnly.startsWith("view/")) {
    const parts = pathOnly.split("/");
    return { type: "view", resource: parts[1], ns: parts[2] || "_", name: parts.slice(3).join("/") };
  }
  const parts = pathOnly.split("/");
  if (parts[0] === "project" && parts[1]) {
    if (parts[2] === "addons") {
      return { type: "project", slug: parts[1], tab: "addons", addon: parts[3] || "", key: "project/" + parts[1] };
    }
    return { type: "project", slug: parts[1], tab: parts[2] || "overview", addon: "", key: "project/" + parts[1] };
  }
  return { type: "page", key: pathOnly };
}

async function navigate() {
  const navToken = nextNavToken();
  const parsed = parseRoute();
  await buildSidebarForRoute(parsed);
  if (!isNavTokenActive(navToken)) return;
  const main = $("#main");
  try {
    if (parsed.type === "view") {
      await pageResourceDetail(main, parsed.resource, parsed.ns, parsed.name);
      return;
    }
    if (parsed.type === "project") {
      await pageProjectHub(main, parsed.slug, parsed.tab, parsed.addon);
      return;
    }
    if (routes[parsed.key]) {
      await routes[parsed.key](main);
      return;
    }
    const menu = await api("/api/v1/explorer/menu");
    const item = menu.find((m) => m.key === parsed.key);
    if (item && item.type === "k8s") {
      await pageK8s(main, item.key, item.label);
      return;
    }
    if (!isNavTokenActive(navToken)) return;
    main.innerHTML = '<p class="error">Không tìm thấy trang: ' + esc(parsed.key) + "</p>";
  } catch (e) {
    if (!isNavTokenActive(navToken)) return;
    main.innerHTML =
      '<p class="error">Lỗi: ' +
      esc(errorMessage(e)) +
      '</p><p class="muted" style="margin-top:8px"><button type="button" class="btn-ghost btn-sm" onclick="location.reload()">Tải lại</button></p>';
  }
}

function markActiveNav(parsed) {
  document.querySelectorAll(".nav-link").forEach(function (el) {
    let active = false;
    if (parsed.type === "project") {
      const route = el.dataset.route || "";
      const tab = projectNavActiveTab(parsed.tab, parsed.addon);
      active = route === "project/" + parsed.slug + "/" + tab || route === "project/" + parsed.slug;
      if (!active && parsed.tab === "addons") {
        active =
          route === "addons/" + parsed.slug + "/" + (parsed.addon || "") ||
          route === "addons/" + parsed.slug;
      }
    } else if (parsed.type === "view") {
      active = el.dataset.route === parsed.resource;
    } else {
      active = el.dataset.route === parsed.key;
    }
    el.classList.toggle("active", active);
  });
}
