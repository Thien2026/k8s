/* Platform overview + monitoring charts. */

function bindInfraCopyButtons(root) {
  if (!root) return;
  root.querySelectorAll("[data-copy]").forEach(function (btn) {
    btn.onclick = function () {
      const text = btn.getAttribute("data-copy") || "";
      if (!text) return;
      navigator.clipboard.writeText(text).then(
        function () { toastSuccess("Đã copy"); },
        function () { toastError("Không copy được"); }
      );
    };
  });
}

function renderInfraLinksCard(links) {
  const items = (links && links.items) || [];
  if (!items.length) return "";
  const rows = items
    .map(function (it) {
      const href = esc(it.login_url || it.url || "#");
      const status = it.enabled
        ? '<span class="badge ok">Kết nối OK</span>'
        : '<span class="badge warn">Chưa cấu hình</span>';
      const cred =
        it.username || it.password
          ? '<div class="infra-creds">' +
            (it.username
              ? '<label class="infra-cred-field">User<input class="infra-cred-input" type="text" readonly value="' +
                esc(it.username) +
                '" /><button type="button" class="btn-ghost btn-sm" data-copy="' +
                esc(it.username) +
                '">Copy</button></label>'
              : "") +
            (it.password
              ? '<label class="infra-cred-field">Pass<input class="infra-cred-input" type="text" readonly value="' +
                esc(it.password) +
                '" /><button type="button" class="btn-ghost btn-sm" data-copy="' +
                esc(it.password) +
                '">Copy</button></label>'
              : '<span class="muted">Pass: chưa cấu hình trên VPS</span>') +
            "</div>"
          : "";
      return (
        '<div class="infra-link-row">' +
        '<div class="infra-link-head"><strong>' +
        esc(it.label) +
        "</strong> " +
        status +
        (it.note ? '<div class="muted" style="font-size:12px;margin-top:2px">' + esc(it.note) + "</div>" : "") +
        "</div>" +
        '<div class="infra-link-actions">' +
        '<a class="btn-primary btn-sm" href="' +
        href +
        '" target="_blank" rel="noopener">Mở ' +
        esc(it.label) +
        "</a>" +
        (it.url && it.url !== it.login_url
          ? '<a class="btn-ghost btn-sm" href="' + esc(it.url) + '" target="_blank" rel="noopener">URL gốc</a>'
          : "") +
        "</div>" +
        cred +
        "</div>"
      );
    })
    .join("");
  return (
    '<div class="card infra-links-card" style="margin-bottom:16px" id="infra-links-card">' +
    "<h3>Công cụ nội bộ</h3>" +
    '<p class="muted">Link Rancher & Harbor kèm tài khoản. <strong>Dùng nút Copy</strong> — đừng gõ tay (dễ sai ký tự).</p>' +
    '<div class="infra-links-grid">' +
    rows +
    "</div></div>"
  );
}

async function pageOverview(main) {
  const navToken = state.navToken;
  main.innerHTML = '<p class="loading">Đang tải Cluster Dashboard…</p>';
  const clusters = await api("/api/v1/rancher/clusters").catch(() => ({ items: [] }));
  const infraLinks = await api("/api/v1/infra/links").catch(function () { return { items: [] }; });
  if (!isNavTokenActive(navToken)) return;
  const clusterItems = clusters.items || [];
  if (!state.clusterId && clusterItems.length) {
    state.clusterId = clusterItems[0].id;
  }

  let d;
  try {
    const ctrl = typeof AbortController !== "undefined" ? new AbortController() : null;
    const timer = ctrl
      ? setTimeout(function () {
          ctrl.abort();
        }, 45000)
      : null;
    d = await api("/api/v1/rancher/cluster/dashboard" + qs(), {
      signal: ctrl ? ctrl.signal : undefined,
    });
    if (timer) clearTimeout(timer);
  } catch (err) {
    if (!isNavTokenActive(navToken)) return;
    const dash = await api("/api/v1/dashboard").catch(function () {
      return null;
    });
    if (!isNavTokenActive(navToken)) return;
    main.innerHTML = renderInfraLinksCard(infraLinks) + renderOverviewFallback(err.message || String(err), dash, clusterItems);
    bindInfraCopyButtons(main);
    const retryBtn = document.getElementById("overview-retry");
    if (retryBtn) retryBtn.onclick = function () { pageOverview(main); };
    return;
  }
  if (!isNavTokenActive(navToken)) return;

  const quickLinks =
    renderInfraLinksCard(infraLinks) +
    '<div class="card" style="margin-bottom:16px"><h3>Đi nhanh</h3>' +
    '<p class="muted">Deploy app nằm trong từng project — không hiển thị trên Cluster Dashboard.</p>' +
    '<div class="meta-chips">' +
    '<a class="chip-link" href="#/platform-projects">Quản lý Projects</a>' +
    '<a class="chip-link" href="#/addons">Addons</a>' +
    (clusterItems.length ? '<a class="chip-link" href="#/pods">Pods</a>' : "") +
    "</div></div>";

  const c = d.counts || {};
  const cap = d.capacity || {};
  const pods = cap.pods || {};
  const cpu = cap.cpu || {};
  const mem = cap.memory || {};
  const disk = cap.disk || {};

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

  const sc = d.scaling || {};
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
    quickLinks +
    '<div class="page-header">' +
    '<h2 class="page-title">Cluster Dashboard</h2>' +
    '<p class="page-subtitle">' +
    esc(d.name || d.cluster_id) +
    ' · <span class="pill">' +
    esc(d.state || "active") +
    "</span></p></div>" +
    (clusterItems.length > 1
      ? '<div class="list-toolbar" style="margin-top:12px">' +
        '<div class="field-group" style="min-width:240px;flex:1">' +
        '<span class="field-label">Cluster</span>' +
        '<div class="select-wrap"><select id="cluster-select">' +
        clusterItems
          .map(
            (c) =>
              '<option value="' +
              esc(c.id) +
              '"' +
              (c.id === state.clusterId ? " selected" : "") +
              ">" +
              esc(c.name || c.id) +
              "</option>"
          )
          .join("") +
        '</select><svg class="select-chev" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M6 9l6 6 6-6"/></svg></div></div></div>'
      : "") +
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
    '<div class="scale-chips">' +
    '<div class="scale-chip">HPA (Auto Scale)<strong>' + (sc.hpa_count || 0) + '</strong></div>' +
    '<div class="scale-chip">Pods đã restart<strong>' + (sc.pods_with_restart || 0) + '</strong></div>' +
    '<div class="scale-chip">Tổng lần restart<strong>' + (sc.total_restarts || 0) + '</strong></div>' +
    '<a href="#/horizontalpodautoscalers" class="scale-chip" style="text-decoration:none;color:inherit">→ Xem HPA</a>' +
    '<a href="#/pods" class="scale-chip" style="text-decoration:none;color:inherit">→ Xem Pods</a>' +
    '<a href="#/add-worker" class="scale-chip" style="text-decoration:none;color:inherit">→ Thêm worker</a>' +
    "</div>" +
    '<div class="dash-grid">' +
    '<div class="card"><h3>Capacity Overview</h3>' +
    '<div class="cap-donuts cap-donuts-4">' +
    svgDonut(pods.used_pct || 0, 100, "#8b5cf6", "Pods", (pods.used || 0) + "/" + (pods.total || 0)) +
    svgDonut(cpu.used_pct || 0, 100, "#22d3ee", "CPU Used", (cpu.used || 0) + " / " + (cpu.total || 0) + " cores") +
    svgDonut(mem.used_pct || 0, 100, "#ec4899", "Memory", (mem.used || 0) + " / " + (mem.total || 0) + " GiB") +
    svgDonut(disk.used_pct || 0, 100, "#f59e0b", "Disk", (disk.used || 0) + " / " + (disk.total || 0) + " GiB") +
    "</div>" +
    '<div class="cap-sub">' +
    meterRow("CPU Reserved", cpu.reserved, cpu.total, cpu.reserved_pct, "reserved", "cores") +
    meterRow("Memory Reserved", mem.reserved, mem.total, mem.reserved_pct, "reserved", "GiB") +
    ephemeralMeterRow(disk) +
    "</div></div>" +
    '<div class="card"><h3>Component Status</h3>' +
    '<div class="comp-grid">' +
    comps +
    "</div></div></div>" +
    '<div class="card dash-full"><h3>Resource Distribution</h3>' +
    '<p class="muted dash-hint">Số đếm tại thời điểm tải trang — refresh để cập nhật.</p>' +
    svgBarChart(barItems, 220, 900) +
    "</div>" +
    '<div class="card dash-full"><h3>Recent Events <a href="#/events" class="link-muted">Xem tất cả →</a></h3>' +
    eventsHtml +
    "</div>";

  const sel = document.getElementById("cluster-select");
  if (sel) {
    sel.onchange = () => {
      state.clusterId = sel.value;
      localStorage.setItem("cluster-id", state.clusterId);
      pageOverview(main);
    };
  }
  bindInfraCopyButtons(main);
}

function renderOverviewFallback(errMsg, dash, clusterItems) {
  const cluster = (dash && dash.cluster) || {};
  const projects = (dash && dash.projects) || [];
  const projRows = projects
    .slice(0, 8)
    .map(function (p) {
      return (
        '<tr><td><a href="#/project/' +
        esc(p.slug) +
        '/deploy">' +
        esc(p.name) +
        '</a></td><td><code>' +
        esc(p.namespace_dev || "") +
        "</code></td></tr>"
      );
    })
    .join("");

  return (
    '<div class="page-header">' +
    '<h2 class="page-title">Platform</h2>' +
    '<p class="page-subtitle">Cluster Dashboard tạm không tải được</p></div>' +
    '<div class="card" style="margin-bottom:16px;border-color:rgba(248,113,113,0.35)">' +
    '<h3>Không tải được Cluster Dashboard</h3>' +
    '<p class="muted">' +
    esc(errMsg) +
    "</p>" +
    '<p class="muted">Thường do Rancher API chậm hoặc token hết hạn. Thử refresh, hoặc mở Rancher trực tiếp. Menu <strong>Hạ tầng</strong> bên trái — bấm mũi tên để mở rộng nếu đang thu gọn.</p>' +
    '<button type="button" class="btn-primary" id="overview-retry">Thử lại</button></div>' +
    '<div class="card" style="margin-bottom:16px"><h3>Trạng thái nhanh</h3>' +
    '<div class="meta-chips">' +
    chip("Cluster", cluster.connected ? "Kết nối" : "Chưa kết nối") +
    chip("Nodes", cluster.nodes != null ? String(cluster.nodes) : "—") +
    chip("Projects", String(projects.length)) +
    "</div></div>" +
    '<div class="card" style="margin-bottom:16px"><h3>Deploy app ở đâu?</h3>' +
    '<p class="muted">Vào <strong>Quản lý Projects</strong> → chọn project → tab <strong>Deploy / Git</strong>.</p>' +
    '<a class="btn-primary" href="#/platform-projects" style="display:inline-block;margin-top:8px">Mở Quản lý Projects</a></div>' +
    (projRows
      ? '<div class="card"><h3>Projects</h3><div class="table-wrap"><table><thead><tr><th>Project</th><th>Namespace dev</th></tr></thead><tbody>' +
        projRows +
        "</tbody></table></div></div>"
      : "") +
    (clusterItems.length
      ? '<p class="muted" style="margin-top:12px">Phát hiện ' + clusterItems.length + " cluster — thử lại sau vài giây.</p>"
      : "")
  );
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

function meterRow(label, used, total, pct, tone, unit) {
  const u = Number(used) || 0;
  const t = Number(total) || 0;
  let p = pct != null && !isNaN(Number(pct)) ? Number(pct) : t > 0 ? (u / t) * 100 : 0;
  if (isNaN(p)) p = 0;
  const suf = unit ? " " + unit : "";
  return (
    '<div class="meter">' +
    '<div class="meter-head"><span>' + esc(label) + "</span><span>" +
    esc(roundDisp(u) + suf) + " / " + esc(roundDisp(t) + suf) + " (" + Math.round(p) + "%)</span></div>" +
    '<div class="meter-track">' +
    (p > 0
      ? '<i class="meter-fill ' + (tone || "used") + '" style="width:' + Math.min(p, 100) + '%"></i>'
      : "") +
    "</div></div>"
  );
}

function ephemeralMeterRow(disk) {
  const reserved = Number(disk.reserved) || 0;
  const total = Number(disk.reserved_total) || 0;
  if (reserved <= 0) {
    return "";
  }
  if (total <= 0) {
    return "";
  }
  return meterRow("Ephemeral Reserved", reserved, total, disk.reserved_pct, "reserved", "GiB");
}

function roundDisp(n) {
  return Number.isInteger(n) ? n : Math.round(n * 100) / 100;
}

function statBox(n, label, grad) {
  return (
    '<div class="stat-box ' + (grad || "g1") + '">' +
    '<div class="lbl">' + esc(label) + '</div><div class="num">' + n + "</div></div>"
  );
}

function monitoringPoints(values, divisor) {
  if (!Array.isArray(values)) return [];
  return values.map(function (row) {
    if (!Array.isArray(row) || row.length < 2) return null;
    const x = Number(row[0]) || 0;
    let y = Number(row[1]) || 0;
    if (divisor) y = y / divisor;
    return [x, y];
  }).filter(Boolean);
}

const chartRegistry = new Map();
let chartSeq = 0;

function lineChartLayout(points) {
  const w = 980;
  const h = 250;
  const m = { top: 14, right: 16, bottom: 34, left: 52 };
  const cw = w - m.left - m.right;
  const ch = h - m.top - m.bottom;
  let min = points[0][1];
  let max = points[0][1];
  points.forEach(function (p) {
    if (p[1] < min) min = p[1];
    if (p[1] > max) max = p[1];
  });
  const padY = Math.max((max - min) * 0.08, 0.000001);
  min -= padY;
  max += padY;
  const span = Math.max(max - min, 0.000001);
  const step = cw / Math.max(points.length - 1, 1);
  function yPos(v) {
    return m.top + ch - ((v - min) / span) * ch;
  }
  function xPos(i) {
    return m.left + i * step;
  }
  return { w: w, h: h, m: m, cw: cw, ch: ch, min: min, max: max, span: span, step: step, yPos: yPos, xPos: xPos };
}

function lineChart(points, opts) {
  opts = opts || {};
  const color = opts.color || "#6EE7FF";
  const unit = opts.unit || "";
  const digits = opts.digits == null ? 2 : opts.digits;
  if (!points.length) {
    return '<div class="muted">Chưa có dữ liệu timeline</div>';
  }
  const layout = lineChartLayout(points);
  const w = layout.w;
  const h = layout.h;
  const m = layout.m;
  const cw = layout.cw;
  const ch = layout.ch;
  const yPos = layout.yPos;
  const xPos = layout.xPos;
  const path = points.map(function (p, i) {
    const x = xPos(i);
    const y = yPos(p[1]);
    return (i === 0 ? "M" : "L") + x.toFixed(2) + " " + y.toFixed(2);
  }).join(" ");
  const area =
    path +
    " L " + xPos(points.length - 1).toFixed(2) + " " + (m.top + ch).toFixed(2) +
    " L " + xPos(0).toFixed(2) + " " + (m.top + ch).toFixed(2) +
    " Z";
  const yTicks = 4;
  const yGrid = [];
  for (let i = 0; i <= yTicks; i++) {
    const ratio = i / yTicks;
    const val = layout.max - ratio * (layout.max - layout.min);
    const y = m.top + ratio * ch;
    yGrid.push({ y: y, val: val });
  }
  const tickCount = 5;
  const xTicks = [];
  for (let i = 0; i < tickCount; i++) {
    const idx = Math.round((points.length - 1) * (i / (tickCount - 1)));
    const ts = Number(points[idx][0]) || 0;
    xTicks.push({ x: xPos(idx), label: fmtClockFromUnix(ts) });
  }
  const last = points[points.length - 1];
  const lastX = xPos(points.length - 1);
  const lastY = yPos(last[1]);
  const chartId = "chart-" + (++chartSeq);
  chartRegistry.set(chartId, { points: points, color: color, unit: unit, digits: digits, layout: layout });

  return (
    '<div class="chart-interactive" data-chart-id="' + chartId + '">' +
    '<svg viewBox="0 0 ' + w + " " + h + '" class="chart-svg" aria-hidden="true">' +
    yGrid.map(function (g) {
      return (
        '<line x1="' + m.left + '" y1="' + g.y.toFixed(2) + '" x2="' + (m.left + cw) + '" y2="' + g.y.toFixed(2) + '" stroke="rgba(255,255,255,0.08)" stroke-width="1"></line>' +
        '<text x="' + (m.left - 8) + '" y="' + (g.y + 4).toFixed(2) + '" text-anchor="end" fill="rgba(200,210,230,0.75)" font-size="11">' +
        esc(Number(g.val).toFixed(digits) + (unit ? " " + unit : "")) +
        "</text>"
      );
    }).join("") +
    xTicks.map(function (t) {
      return '<text x="' + t.x.toFixed(2) + '" y="' + (h - 10) + '" text-anchor="middle" fill="rgba(200,210,230,0.65)" font-size="11">' + esc(t.label) + "</text>";
    }).join("") +
    '<path d="' + area + '" fill="' + color + '" opacity="0.12"></path>' +
    '<path d="' + path + '" fill="none" stroke="' + color + '" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"></path>' +
    '<circle class="chart-last-dot" cx="' + lastX.toFixed(2) + '" cy="' + lastY.toFixed(2) + '" r="3.5" fill="' + color + '"></circle>' +
    "</svg>" +
    '<div class="chart-overlay">' +
    '<div class="chart-crosshair-v"></div>' +
    '<div class="chart-crosshair-h"></div>' +
    '<div class="chart-focus-dot" style="background:' + color + ';box-shadow:0 0 0 2px #fff"></div>' +
    '<div class="chart-tooltip" role="tooltip"><div class="tt-time"></div><div class="tt-val"></div></div>' +
    "</div>" +
    "</div>"
  );
}

function fmtChartDateTime(tsSec) {
  const d = new Date((Number(tsSec) || 0) * 1000);
  if (isNaN(d.getTime())) return "--:--";
  const pad = function (n) {
    return n < 10 ? "0" + n : String(n);
  };
  return (
    pad(d.getDate()) + "/" + pad(d.getMonth() + 1) + " " +
    pad(d.getHours()) + ":" + pad(d.getMinutes()) + ":" + pad(d.getSeconds())
  );
}

function bindInteractiveCharts(root) {
  (root || document).querySelectorAll(".chart-interactive").forEach(function (wrap) {
    if (wrap.dataset.chartBound === "1") return;
    const chartId = wrap.getAttribute("data-chart-id");
    const meta = chartRegistry.get(chartId);
    if (!meta) return;
    wrap.dataset.chartBound = "1";

    const overlay = wrap.querySelector(".chart-overlay");
    const crosshairV = wrap.querySelector(".chart-crosshair-v");
    const crosshairH = wrap.querySelector(".chart-crosshair-h");
    const dot = wrap.querySelector(".chart-focus-dot");
    const lastDot = wrap.querySelector(".chart-last-dot");
    const tooltip = wrap.querySelector(".chart-tooltip");
    const ttTime = wrap.querySelector(".tt-time");
    const ttVal = wrap.querySelector(".tt-val");
    if (!overlay || !crosshairV || !crosshairH || !dot || !tooltip || !ttTime || !ttVal) return;

    const points = meta.points;
    const layout = meta.layout;
    const m = layout.m;
    const cw = layout.cw;
    const ch = layout.ch;
    const unitSuffix = meta.unit ? " " + meta.unit : "";
    let geom = null;
    let pxCache = null;
    let lastIdx = -1;

    function rebuildGeom() {
      const ow = overlay.clientWidth;
      const oh = overlay.clientHeight;
      if (!ow || !oh) return;
      geom = {
        left: (m.left / layout.w) * ow,
        top: (m.top / layout.h) * oh,
        width: (cw / layout.w) * ow,
        height: (ch / layout.h) * oh,
      };
      crosshairV.style.top = geom.top.toFixed(1) + "px";
      crosshairV.style.height = geom.height.toFixed(1) + "px";
      crosshairH.style.left = "0px";
      crosshairH.style.width = ow.toFixed(1) + "px";
      pxCache = points.map(function (p, i) {
        const xNorm = i / Math.max(points.length - 1, 1);
        const yNorm = (p[1] - layout.min) / layout.span;
        return {
          px: geom.left + xNorm * geom.width,
          py: geom.top + geom.height * (1 - yNorm),
          time: fmtChartDateTime(p[0]),
          val: Number(p[1]).toFixed(meta.digits) + unitSuffix,
        };
      });
    }

    function hideHover() {
      lastIdx = -1;
      wrap.classList.remove("is-hovering");
      if (lastDot) lastDot.style.opacity = "1";
    }

    function pointIndexFromEvent(evt) {
      if (!geom) rebuildGeom();
      if (!geom || !geom.width) return 0;
      const rect = overlay.getBoundingClientRect();
      const mouseX = evt.clientX - rect.left - geom.left;
      let idx = Math.round((mouseX / geom.width) * (points.length - 1));
      if (idx < 0) idx = 0;
      if (idx >= points.length) idx = points.length - 1;
      return idx;
    }

    function showHover(idx) {
      if (!pxCache) rebuildGeom();
      if (!pxCache || !pxCache[idx]) return;
      if (idx === lastIdx && wrap.classList.contains("is-hovering")) return;

      const c = pxCache[idx];
      lastIdx = idx;
      wrap.classList.add("is-hovering");
      crosshairV.style.transform = "translate3d(" + c.px.toFixed(1) + "px,0,0)";
      crosshairH.style.transform = "translate3d(0," + c.py.toFixed(1) + "px,0)";
      dot.style.transform = "translate3d(" + c.px.toFixed(1) + "px," + c.py.toFixed(1) + "px,0)";
      tooltip.style.left = c.px.toFixed(1) + "px";
      tooltip.style.top = (c.py - 8).toFixed(1) + "px";
      ttTime.textContent = c.time;
      ttVal.textContent = c.val;
      if (lastDot) lastDot.style.opacity = "0.35";
    }

    function onMove(evt) {
      showHover(pointIndexFromEvent(evt));
    }

    rebuildGeom();
    overlay.addEventListener("pointerenter", onMove);
    overlay.addEventListener("pointermove", onMove);
    overlay.addEventListener("pointerleave", hideHover);
    window.addEventListener("resize", function () {
      geom = null;
      pxCache = null;
      rebuildGeom();
    }, { passive: true });
  });
}

function timelineStats(points) {
  if (!points.length) {
    return { current: 0, min: 0, max: 0, avg: 0, minTs: 0, maxTs: 0 };
  }
  let min = points[0][1];
  let max = points[0][1];
  let minTs = points[0][0];
  let maxTs = points[0][0];
  let sum = 0;
  points.forEach(function (p) {
    const v = Number(p[1]) || 0;
    if (v < min) {
      min = v;
      minTs = Number(p[0]) || 0;
    }
    if (v > max) {
      max = v;
      maxTs = Number(p[0]) || 0;
    }
    sum += v;
  });
  return {
    current: Number(points[points.length - 1][1]) || 0,
    min: min,
    max: max,
    avg: sum / points.length,
    minTs: minTs,
    maxTs: maxTs,
  };
}

function timelineStatsChips(stats, unit, digits) {
  const d = digits == null ? 2 : digits;
  return (
    '<div class="meta-chips monitor-stats">' +
    chip("Current", Number(stats.current).toFixed(d) + " " + unit) +
    chip("Avg", Number(stats.avg).toFixed(d) + " " + unit) +
    chip("Đáy", Number(stats.min).toFixed(d) + " " + unit) +
    chip("Đỉnh", Number(stats.max).toFixed(d) + " " + unit) +
    "</div>"
  );
}

function fmtClockFromUnix(tsSec) {
  const d = new Date((Number(tsSec) || 0) * 1000);
  if (isNaN(d.getTime())) return "--:--";
  return d.toLocaleTimeString("vi-VN", { hour: "2-digit", minute: "2-digit", hour12: false });
}

function timelinePeakText(stats, unit, digits) {
  const d = digits == null ? 2 : digits;
  return (
    '<p class="muted">Đỉnh: <strong>' + esc(Number(stats.max).toFixed(d) + " " + unit) + "</strong> lúc " +
    esc(fmtClockFromUnix(stats.maxTs)) +
    ' · Đáy: <strong>' + esc(Number(stats.min).toFixed(d) + " " + unit) + "</strong> lúc " +
    esc(fmtClockFromUnix(stats.minTs)) +
    "</p>"
  );
}

function monitoringWindowToolbar(active, slug, env) {
  const opts = [
    { key: "15m", label: "15m" },
    { key: "1h", label: "1h" },
    { key: "6h", label: "6h" },
    { key: "24h", label: "24h" },
  ];
  return (
    '<div class="monitor-toolbar">' +
    '<span class="monitor-toolbar-label">Khung thời gian</span>' +
    '<div class="monitor-segmented">' +
    opts.map(function (o) {
      const cls = o.key === active ? "seg-btn active" : "seg-btn";
      return '<a class="' + cls + '" href="' + esc("#/project/" + slug + "/monitoring?env=" + env + "&window=" + o.key) + '">' + esc(o.label) + "</a>";
    }).join("") +
    "</div>" +
    "</div>"
  );
}

function monitoringInsight(cpu, memMiB, restarts) {
  if (restarts > 0) return { tone: "warn", text: "Có restart trong khung thời gian này, nên kiểm tra pod logs." };
  if (cpu > 0.8 || memMiB > 1024) return { tone: "warn", text: "Tải tài nguyên khá cao, cân nhắc scale hoặc tối ưu." };
  if (cpu > 0.25 || memMiB > 512) return { tone: "ok", text: "Hệ thống đang ổn định, tải ở mức trung bình." };
  return { tone: "ok", text: "Hệ thống ổn định, tải thấp." };
}
