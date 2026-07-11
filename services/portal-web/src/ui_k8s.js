/* K8s list / ops / detail pages (extracted from app.js). */

function k8sColumns(resource, data) {
  const age = { key: "created", label: "Age", render: (r) => esc(fmtTime(r.created)) };
  const ns = { key: "namespace", label: "Namespace" };

  if (resource === "events") {
    return [
      { key: "status", label: "Type", render: (r) => badgeStatus(r.status) },
      { key: "reason", label: "Reason" },
      { key: "object", label: "Object" },
      ns,
      { key: "message", label: "Message" },
      age,
    ];
  }
  if (resource === "nodes") {
    return [
      { key: "name", label: "Name" },
      { key: "status", label: "Status", render: (r) => badgeStatus(r.status) },
      { key: "node_ip", label: "Internal IP" },
      { key: "cpu_cores", label: "CPU", render: (r) => esc(r.cpu_cores ? r.cpu_cores + " cores" : "—") },
      { key: "mem_gib", label: "Memory", render: (r) => esc(r.mem_gib ? r.mem_gib.toFixed(1) + " GiB" : "—") },
      { key: "pods_max", label: "Pod Capacity" },
      age,
    ];
  }
  if (resource === "pods") {
    return [
      { key: "name", label: "Name" },
      ns,
      { key: "status", label: "Phase", render: (r) => badgeStatus(r.status) },
      { key: "node", label: "Node" },
      { key: "pod_ip", label: "Pod IP" },
      { key: "restarts", label: "Restarts", render: (r) => r.restarts > 0 ? '<span class="badge badge-warn">' + r.restarts + "</span>" : esc(r.restarts || 0) },
      { key: "restart_policy", label: "Restart Policy" },
      { key: "images", label: "Image" },
      age,
    ];
  }
  if (resource === "deployments" || resource === "statefulsets" || resource === "daemonsets") {
    return [
      { key: "name", label: "Name" },
      ns,
      { key: "replicas", label: "Replicas" },
      { key: "status", label: "Status", render: (r) => badgeStatus(r.status) },
      { key: "selector", label: "Selector" },
      age,
    ];
  }
  if (resource === "jobs") {
    return [
      { key: "name", label: "Name" },
      ns,
      { key: "status", label: "Status", render: (r) => badgeStatus(r.status) },
      { key: "completions", label: "Completions" },
      age,
    ];
  }
  if (resource === "cronjobs") {
    return [
      { key: "name", label: "Name" },
      ns,
      { key: "schedule", label: "Schedule" },
      { key: "suspend", label: "Suspended" },
      { key: "status", label: "Status", render: (r) => badgeStatus(r.status) },
      age,
    ];
  }
  if (resource === "horizontalpodautoscalers") {
    return [
      { key: "name", label: "Name" },
      ns,
      { key: "scale", label: "Min–Max → Current" },
      age,
    ];
  }
  if (resource === "services") {
    return [
      { key: "name", label: "Name" },
      ns,
      { key: "service_type", label: "Type" },
      { key: "cluster_ip", label: "Cluster IP" },
      { key: "ports", label: "Ports" },
      age,
    ];
  }
  if (resource === "ingresses") {
    return [
      { key: "name", label: "Name" },
      ns,
      { key: "host", label: "Hosts" },
      { key: "status", label: "Class" },
      age,
    ];
  }
  if (resource === "persistentvolumeclaims" || resource === "persistentvolumes") {
    return [
      { key: "name", label: "Name" },
      ns,
      { key: "status", label: "Status", render: (r) => badgeStatus(r.status) },
      { key: "capacity", label: "Capacity" },
      { key: "access_modes", label: "Access Modes" },
      { key: "storage_class", label: "Storage Class" },
      age,
    ].filter(function (c) { return resource !== "persistentvolumes" || c.key !== "namespace"; });
  }
  const cols = [
    { key: "name", label: "Name" },
    ns,
    { key: "status", label: "Status", render: (r) => badgeStatus(r.status) },
    age,
  ];
  if (!data.items || !data.items.some((i) => i.namespace)) {
    cols.splice(1, 1);
  }
  return cols;
}

function listToolbar(resource, namespaced) {
  let html =
    '<div class="list-toolbar">' +
    '<div class="field-search">' +
    '<svg class="search-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">' +
    '<circle cx="11" cy="11" r="7"/><path d="M20 20l-3-3"/></svg>' +
    '<input type="search" id="list-search" placeholder="Tìm theo tên…" value="' +
    esc(state.search) +
    '" autocomplete="off">' +
    "</div>";
  if (namespaced) {
    html +=
      '<div class="field-group">' +
      '<span class="field-label">Namespace</span>' +
      '<div class="select-wrap">' +
      '<select id="ns-filter"><option value="">Tất cả</option></select>' +
      '<svg class="select-chev" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M6 9l6 6 6-6"/></svg>' +
      "</div></div>";
  }
  html += "</div>";
  return html;
}

async function bindListToolbar(main, resource, namespaced, reload) {
  const search = document.getElementById("list-search");
  if (search) {
    search.oninput = () => {
      state.search = search.value.trim().toLowerCase();
      reload();
    };
  }
  if (namespaced) {
    const sel = document.getElementById("ns-filter");
    const data = await api(
      "/api/v1/rancher/namespaces" + (state.clusterId ? "?cluster_id=" + encodeURIComponent(state.clusterId) : "")
    );
    const items = data.items || [];
    sel.innerHTML =
      '<option value="">Tất cả</option>' +
      items.map((n) => '<option value="' + esc(n) + '">' + esc(n) + "</option>").join("");
    sel.value = state.namespace;
    sel.onchange = () => {
      state.namespace = sel.value;
      localStorage.setItem("filter-ns", state.namespace);
      state.page[resource] = 1;
      reload();
    };
  }
}

function filterRows(rows) {
  if (!state.search) return rows;
  return rows.filter((r) => (r.name || "").toLowerCase().includes(state.search));
}

async function pageK8s(main, resource, label, page, limit) {
  const route = resource;
  page = page || state.page[route] || 1;
  limit = limit || state.limit;
  state.page[route] = page;
  state.limit = limit;

  const resMeta = await api("/api/v1/explorer/menu");
  const item = resMeta.find((m) => m.key === resource);
  const namespaced = item && item.type === "k8s" && resource !== "namespaces" && resource !== "nodes" && resource !== "events" && resource !== "persistentvolumes" && resource !== "storageclasses";

  main.innerHTML = '<p class="loading">Đang tải ' + esc(label) + "…</p>";
  const data = await api(
    "/api/v1/k8s/" + resource + qs({ page: page, limit: limit })
  );
  const cols = k8sColumns(resource, data);
  const onPage = (p, l) => pageK8s(main, resource, label, p, l);
  const rows = filterRows(data.items || []);

  main.innerHTML = listPage(
    label,
    data.total,
    renderTable(cols, rows, resource) +
      renderPagination(route, data.total, data.page, data.limit, onPage),
    listToolbar(resource, namespaced)
  );
  await bindListToolbar(main, resource, namespaced, () => pageK8s(main, resource, label, page, limit));
}

async function pageAddWorker(main) {
  main.innerHTML = '<p class="loading">Đang tải…</p>';
  const info = await api("/api/v1/cluster/join-info" + qs());
  let gate = getJoinGate();
  let scriptHtml = '<p class="muted">Nhập PIN join (xem trên VPS: config/join-gate.env) rồi bấm lấy script.</p>';
  let script = "";

  main.innerHTML =
    '<div class="page-header"><h2 class="page-title">Thêm worker (RKE2)</h2>' +
    '<p class="page-subtitle">Copy script chạy trên VPS mới — token không hiện công khai.</p></div>' +
    '<div class="card detail-card">' +
    '<div class="meta-chips">' +
    chip("Server IP", info.server_ip || "—") +
    chip("Supervisor URL", info.server_url || "—") +
    chip("Nodes hiện tại", String(info.node_count || 0)) +
    chip("Join ready", info.join_configured ? "yes" : "no") +
    "</div>" +
    "<h3>Ports cần mở (VPS mới → server)</h3><ul class='port-list'>" +
    (info.required_ports || []).map((p) => "<li>" + esc(p) + "</li>").join("") +
    "</ul>" +
    '<div class="toolbar"><label>PIN join <input type="password" id="join-gate-in" placeholder="JOIN_GATE_SECRET" /></label>' +
    '<button type="button" id="join-fetch-btn">Lấy script join</button>' +
    '<button type="button" id="join-poll-btn">Refresh nodes</button></div>' +
    '<pre id="join-script" class="yaml-box">' +
    esc(scriptHtml) +
    "</pre>" +
    '<p class="muted">Bảo mật: PIN lưu session trình duyệt; script chứa token — không share. Xóa sau khi join.</p>' +
    "</div>";

  const gateIn = document.getElementById("join-gate-in");
  if (gate) gateIn.value = gate;

  document.getElementById("join-fetch-btn").onclick = async () => {
    gate = gateIn.value.trim();
    if (!gate) {
      toastWarn("Nhập PIN join trước");
      return;
    }
    setJoinGate(gate);
    try {
      const resp = await api("/api/v1/cluster/join-script", {
        method: "POST",
        headers: { "X-Join-Gate": gate },
        body: { gate: gate },
      });
      script = resp.script || "";
      document.getElementById("join-script").textContent = script;
    } catch (e) {
      document.getElementById("join-script").textContent = "Lỗi: " + e.message;
    }
  };

  document.getElementById("join-poll-btn").onclick = async () => {
    const i = await api("/api/v1/cluster/join-info" + qs());
    toastInfo("Nodes hiện tại: " + (i.node_count || 0));
    location.hash = "#/nodes";
  };
}

async function pageK8sOps(main, opts) {
  opts = opts || {};
  if (typeof window.__k8sOpsCloseMenu === "function") {
    document.removeEventListener("mousedown", window.__k8sOpsCloseMenu);
    window.__k8sOpsCloseMenu = null;
  }
  const scope = opts.scope || "platform";
  const embed = opts.embed === true;
  if (!main) return;
  main.innerHTML = '<p class="loading">Đang tải sổ lệnh K8s…</p>';

  let namespaces = [];
  try {
    const nsRes = await api("/api/v1/rancher/namespaces");
    namespaces = nsRes.items || [];
  } catch (_) {
    namespaces = opts.namespace ? [opts.namespace] : [];
  }
  const data = await api("/api/v1/k8s/commands" + qs({ scope: scope }));
  const commands = data.items || [];
  const defaultNs = opts.namespace || state.namespace || namespaces[0] || "";

  function fillTemplate(tpl, vars) {
    return tpl.replace(/\{(\w+)\}/g, function (_, key) {
      return vars[key] != null && vars[key] !== "" ? String(vars[key]) : "{" + key + "}";
    });
  }

  function defaultVars() {
    return {
      namespace: defaultNs,
      pod: "{pod}",
      deployment: "{deployment}",
      name: "{name}",
      container: "{container}",
      tail: "300",
      label: "app={app}",
    };
  }

  function formatRunOutput(result) {
    if (!result) return "";
    if (result.kind === "logs") return String(result.output || "");
    if (result.kind === "table" && Array.isArray(result.output)) {
      if (!result.output.length) return "(không có dữ liệu)";
      return result.output
        .map(function (row) {
          if (typeof row === "string") return row;
          const name = row.name || row.Name || "—";
          const status = row.status || row.Status || row.phase || row.Phase || "";
          const ns = row.namespace || row.Namespace || "";
          return [name, ns, status].filter(Boolean).join("\t");
        })
        .join("\n");
    }
    if (result.kind === "json") {
      return JSON.stringify(result.output, null, 2);
    }
    return String(result.output || "");
  }

  const nsOptions = namespaces
    .map(function (n) {
      return '<option value="' + esc(n) + '"' + (n === defaultNs ? " selected" : "") + ">" + esc(n) + "</option>";
    })
    .join("");

  const byCat = {};
  commands.forEach(function (cmd) {
    const cat = cmd.category || "Khác";
    if (!byCat[cat]) byCat[cat] = [];
    byCat[cat].push(cmd);
  });
  const catOrder = ["Pods", "Deployments", "Workloads", "Networking", "Config & Storage", "Debug", "Cluster", "Khác"];
  const sortedCats = catOrder.filter(function (c) { return byCat[c]; });
  Object.keys(byCat).forEach(function (c) {
    if (sortedCats.indexOf(c) < 0) sortedCats.push(c);
  });

  let quickMenuHtml =
    '<div class="k8s-term-quick-head">' +
    '<p class="k8s-term-quick-head-title">Chèn mẫu lệnh <span class="muted">· ngay trên ô gõ</span></p>' +
    '<input type="search" class="k8s-term-quick-search" id="k8s-term-quick-search" placeholder="Tìm lệnh… (pods, logs, ingress…)" autocomplete="off" />' +
    "</div>" +
    '<div class="k8s-term-quick-list" id="k8s-term-quick-list">';
  sortedCats.forEach(function (cat) {
    quickMenuHtml += '<div class="k8s-term-quick-cat">' + esc(cat) + "</div>";
    byCat[cat].forEach(function (cmd) {
      const copyOnly = cmd.runnable === false;
      quickMenuHtml +=
        '<button type="button" class="k8s-term-quick-item" data-cmd-id="' +
        esc(cmd.id) +
        '" data-search="' +
        esc((cmd.label + " " + cmd.kubectl + " " + (cmd.description || "")).toLowerCase()) +
        '" title="' +
        esc(cmd.kubectl) +
        '">' +
        "<span>" +
        esc(cmd.label) +
        (copyOnly ? ' <em class="k8s-term-copy-tag">copy</em>' : "") +
        "</span>" +
        '<code class="k8s-term-quick-code">' +
        esc(cmd.kubectl) +
        "</code></button>";
    });
  });
  quickMenuHtml += "</div>";

  const headerHtml = embed
    ? ""
    : '<div class="page-header k8s-term-page-header">' +
      '<div class="k8s-term-page-title-row">' +
      "<h2 class=\"page-title\">Sổ lệnh K8s</h2>" +
      renderK8sOpsHelpButton("start", "btn-help-header") +
      "</div>" +
      '<p class="page-subtitle">Terminal read-only — gõ kubectl tự do hoặc chèn mẫu từ <strong>+</strong>.</p></div>';

  main.innerHTML =
    headerHtml +
    '<div class="card detail-card k8s-terminal">' +
    '<div class="k8s-terminal-toolbar">' +
    '<label class="k8s-term-ns-label">Namespace mặc định' +
    '<select id="k8s-term-ns"' +
    (scope === "project" ? " disabled" : "") +
    ">" +
    nsOptions +
    "</select></label>" +
    '<span class="muted k8s-term-scope">Phạm vi <strong>' +
    (scope === "project" ? "Project" : "Platform") +
    "</strong></span>" +
    (embed ? renderK8sOpsHelpButton("start", "btn-help-inline") : "") +
    "</div>" +
    '<div class="k8s-terminal-feed" id="k8s-term-feed" tabindex="0">' +
    '<div class="k8s-term-line k8s-term-system">' +
    "Gõ lệnh <code>kubectl get|describe|logs</code> bên dưới. Bấm <strong>+</strong> chèn mẫu. Sau khi chạy — <strong>Copy kết quả</strong> để lấy log/output." +
    "</div></div>" +
    '<div class="k8s-term-quick-panel" id="k8s-term-plus-menu" hidden>' +
    quickMenuHtml +
    "</div>" +
    '<div class="k8s-terminal-composer">' +
    '<button type="button" class="k8s-term-plus" id="k8s-term-plus" title="Chèn lệnh mẫu">+</button>' +
    '<span class="k8s-term-prompt" aria-hidden="true">$</span>' +
    '<input type="text" class="k8s-term-input" id="k8s-term-input" autocomplete="off" spellcheck="false" placeholder="kubectl get pods -n ' +
    esc(defaultNs || "namespace") +
    '" />' +
    '<button type="button" class="btn-primary btn-sm" id="k8s-term-run">Chạy</button>' +
    '<button type="button" class="btn-ghost btn-sm" id="k8s-term-copy" disabled title="Copy kết quả lệnh vừa chạy">Copy kết quả</button>' +
    "</div></div>";

  bindK8sOpsHelpTriggers(main);

  const feed = main.querySelector("#k8s-term-feed");
  const input = main.querySelector("#k8s-term-input");
  const nsSelect = main.querySelector("#k8s-term-ns");
  const plusBtn = main.querySelector("#k8s-term-plus");
  const plusMenu = main.querySelector("#k8s-term-plus-menu");
  const termCard = main.querySelector(".k8s-terminal");
  const copyResultBtn = main.querySelector("#k8s-term-copy");
  const runBtnEl = main.querySelector("#k8s-term-run");
  if (!feed || !input || !plusBtn || !plusMenu || !runBtnEl) {
    main.innerHTML =
      '<div class="card"><p class="error-text">Không dựng được terminal K8s (thiếu phần tử UI).</p></div>';
    return;
  }
  let lastOutputText = "";

  function copyOutputText(text, okMsg) {
    text = (text || "").trim();
    if (!text) {
      toastInfo("Chưa có kết quả để copy — chạy lệnh trước");
      return;
    }
    navigator.clipboard.writeText(text).then(
      function () { toastSuccess(okMsg || "Đã copy kết quả"); },
      function () { toastInfo(text.slice(0, 500)); }
    );
  }

  function setLastOutput(text) {
    lastOutputText = text || "";
    if (copyResultBtn) {
      copyResultBtn.disabled = !lastOutputText.trim();
    }
  }

  function currentNs() {
    if (scope === "project" && defaultNs) return defaultNs;
    return (nsSelect && nsSelect.value) || defaultNs || "";
  }

  function appendLine(className, html) {
    const line = document.createElement("div");
    line.className = "k8s-term-line " + className;
    line.innerHTML = html;
    feed.appendChild(line);
    feed.scrollTop = feed.scrollHeight;
  }

  function appendUserCmd(cmdText) {
    appendLine("k8s-term-user", '<span class="k8s-term-prompt-inline">$</span> ' + esc(cmdText));
  }

  function appendOutput(text, isError) {
    const outText = text == null ? "" : String(text);
    setLastOutput(outText);
    const wrap = document.createElement("div");
    wrap.className = "k8s-term-line k8s-term-result";
    const bar = document.createElement("div");
    bar.className = "k8s-term-result-bar";
    const copyOne = document.createElement("button");
    copyOne.type = "button";
    copyOne.className = "btn-ghost btn-sm k8s-term-copy-one";
    copyOne.textContent = "Copy";
    copyOne.title = "Copy khối kết quả này";
    copyOne.onclick = function () {
      copyOutputText(outText);
    };
    bar.appendChild(copyOne);
    const pre = document.createElement("pre");
    pre.className = "k8s-term-output" + (isError ? " k8s-term-error" : "");
    pre.textContent = outText;
    wrap.appendChild(bar);
    wrap.appendChild(pre);
    feed.appendChild(wrap);
    feed.scrollTop = feed.scrollHeight;
  }

  function insertTemplate(cmd) {
    const vars = defaultVars();
    vars.namespace = currentNs();
    const line = fillTemplate(cmd.kubectl, vars);
    input.value = line;
    input.focus();
    const pos = line.indexOf("{");
    if (pos >= 0) {
      input.setSelectionRange(pos, line.indexOf("}", pos) + 1 || line.length);
    } else {
      input.select();
    }
    plusMenu.hidden = true;
  }

  async function runCommand(cmdText) {
    cmdText = (cmdText || "").trim();
    if (!cmdText) return;
    appendUserCmd(cmdText);
    const runBtn = main.querySelector("#k8s-term-run");
    if (runBtn) runBtn.disabled = true;
    input.disabled = true;
    try {
      const result = await api("/api/v1/k8s/commands/run", {
        method: "POST",
        body: { kubectl: cmdText },
      });
      appendOutput(formatRunOutput(result), false);
      if (result.summary) {
        appendLine("k8s-term-meta muted", esc(result.summary));
      }
    } catch (err) {
      appendOutput(errorMessage(err), true);
    } finally {
      if (runBtn) runBtn.disabled = false;
      input.disabled = false;
      input.focus();
    }
  }

  function closePlusMenu() {
    if (plusMenu) plusMenu.hidden = true;
    if (plusBtn) plusBtn.classList.remove("is-active");
    if (termCard) termCard.classList.remove("k8s-terminal-picks-open");
  }

  function openPlusMenu() {
    if (plusMenu) plusMenu.hidden = false;
    if (plusBtn) plusBtn.classList.add("is-active");
    if (termCard) termCard.classList.add("k8s-terminal-picks-open");
    const search = main.querySelector("#k8s-term-quick-search");
    if (search) {
      search.value = "";
      search.dispatchEvent(new Event("input"));
      setTimeout(function () { search.focus(); }, 0);
    }
  }

  plusBtn.onclick = function (e) {
    e.stopPropagation();
    if (plusMenu.hidden) openPlusMenu();
    else closePlusMenu();
  };

  main.querySelectorAll(".k8s-term-quick-item").forEach(function (btn) {
    btn.onclick = function (e) {
      e.stopPropagation();
      const cmd = commands.find(function (c) { return c.id === btn.dataset.cmdId; });
      if (cmd) insertTemplate(cmd);
    };
  });

  const quickSearch = main.querySelector("#k8s-term-quick-search");
  if (quickSearch) {
    quickSearch.addEventListener("input", function () {
      const q = quickSearch.value.trim().toLowerCase();
      main.querySelectorAll(".k8s-term-quick-item").forEach(function (btn) {
        const hay = btn.getAttribute("data-search") || "";
        btn.hidden = q !== "" && hay.indexOf(q) < 0;
      });
      main.querySelectorAll(".k8s-term-quick-cat").forEach(function (head) {
        let next = head.nextElementSibling;
        let any = false;
        while (next && !next.classList.contains("k8s-term-quick-cat")) {
          if (next.classList.contains("k8s-term-quick-item") && !next.hidden) any = true;
          next = next.nextElementSibling;
        }
        head.hidden = q !== "" && !any;
      });
    });
    quickSearch.addEventListener("click", function (e) { e.stopPropagation(); });
  }

  function onDocPointerDown(e) {
    if (!plusMenu || plusMenu.hidden) return;
    if (plusMenu.contains(e.target) || (plusBtn && plusBtn.contains(e.target))) return;
    closePlusMenu();
  }
  document.addEventListener("mousedown", onDocPointerDown);
  window.__k8sOpsCloseMenu = onDocPointerDown;

  feed.addEventListener("mousedown", closePlusMenu);

  runBtnEl.onclick = function () {
    runCommand(input.value);
    input.value = "";
  };

  if (copyResultBtn) {
    copyResultBtn.onclick = function () {
      copyOutputText(lastOutputText);
    };
  }

  input.addEventListener("keydown", function (e) {
    if (e.key === "Enter") {
      e.preventDefault();
      runCommand(input.value);
      input.value = "";
    }
    if (e.key === "Escape" && plusMenu) {
      closePlusMenu();
    }
  });

  if (nsSelect && scope !== "project") {
    nsSelect.addEventListener("change", function () {
      input.placeholder = "kubectl get pods -n " + nsSelect.value;
    });
  }

  input.focus();
}

async function pageResourceDetail(main, resource, ns, name) {
  main.innerHTML = '<p class="loading">Đang tải chi tiết…</p>';
  const nsQ = ns && ns !== "_" ? ns : "";
  const base = "/api/v1/k8s/" + resource + "/" + encodeURIComponent(name) + qs({ namespace: nsQ });

  let detail = {};
  try {
    detail = await api(base);
  } catch (e) {
    main.innerHTML = '<p class="error">Lỗi: ' + esc(e.message) + "</p>";
    return;
  }

  const title = name + (nsQ ? " · " + nsQ : "");
  let actions =
    '<div class="action-bar">' +
    '<a href="#/' + esc(resource) + '" class="btn-ghost">← Quay lại</a>';

  if (resource === "deployments" && nsQ && canWriteK8s()) {
    actions +=
      ' <label>Replicas <input type="number" id="scale-replicas" min="0" max="100" style="width:4rem" />' +
      ' <button type="button" id="scale-btn">Scale</button></label>';
  }
  if (resource === "pods" && nsQ && canWriteK8s()) {
    actions += ' <button type="button" id="restart-btn">Restart (delete)</button>';
  }
  actions += ' <button type="button" id="yaml-btn">YAML</button>';
  if (resource === "pods" && nsQ) {
    actions += ' <button type="button" id="logs-btn">Logs</button>';
  }
  if (canWriteK8s()) {
    actions += ' <button type="button" id="delete-btn" class="btn-danger">Xóa</button>';
  }
  actions += "</div>";

  main.innerHTML =
    '<div class="page-header"><h2 class="page-title">' +
    esc(title) +
    '</h2><p class="page-subtitle">' +
    esc(resource) +
    "</p></div>" +
    actions +
    '<div class="card detail-card"><h3>Overview</h3><pre class="yaml-box">' +
    esc(JSON.stringify(detail, null, 2)) +
    '</pre></div><div id="extra-panel"></div>';

  document.getElementById("yaml-btn").onclick = async () => {
    const y = await api(base + "/yaml");
    document.getElementById("extra-panel").innerHTML =
      '<div class="card detail-card"><h3>YAML</h3><pre class="yaml-box">' + esc(y.yaml || "") + "</pre></div>";
  };

  if (resource === "pods" && nsQ) {
    document.getElementById("logs-btn").onclick = async () => {
      const l = await api(
        "/api/v1/k8s/pods/" + encodeURIComponent(name) + "/logs" + qs({ namespace: nsQ, tail: 300 })
      );
      document.getElementById("extra-panel").innerHTML =
        '<div class="card detail-card"><h3>Logs</h3><pre class="log-box">' + esc(l.logs || "") + "</pre></div>";
    };
  }

  if (resource === "deployments" && nsQ && canWriteK8s()) {
    document.getElementById("scale-btn").onclick = async () => {
      const n = parseInt(document.getElementById("scale-replicas").value, 10);
      if (isNaN(n)) return;
      await api(
        "/api/v1/k8s/deployments/" + encodeURIComponent(name) + "/scale" + qs({ namespace: nsQ }),
        { method: "PATCH", body: { replicas: n } }
      );
      toastSuccess("Đã scale — đang refresh");
      pageResourceDetail(main, resource, ns, name);
    };
  }

  if (resource === "pods" && nsQ && canWriteK8s()) {
    document.getElementById("restart-btn").onclick = async () => {
      if (!(await uiConfirm("Restart pod " + name + "?", { title: "Restart pod", confirmText: "Restart" }))) return;
      await api(base, { method: "DELETE" });
      toastInfo("Pod đang recreate…");
      location.hash = "#/pods";
    };
  }

  const delBtn = document.getElementById("delete-btn");
  if (delBtn) {
    delBtn.onclick = async () => {
      if (!(await uiConfirm("Xóa " + resource + "/" + name + "?", { danger: true, title: "Xóa resource" }))) return;
      await api(base, { method: "DELETE" });
      toastSuccess("Đã xóa");
      location.hash = "#/" + resource;
    };
  }
}

const routes = {
  overview: (main) => pageOverview(main),
  "my-projects": (main) => pageMyProjects(main),
  "platform-projects": (main) => pagePlatformProjects(main),
  addons: (main) => pageAddons(main),
  gitops: (main) => pageGitOps(main),
  policy: (main) => pagePlatformPolicy(main),
  "add-worker": (main) => pageAddWorker(main),
  audit: (main) => pageAudit(main),
  users: (main) => pageUsers(main),
  clusters: (main) =>
    pageRancherList(main, "Clusters", "/api/v1/rancher/clusters", [
      { key: "name", label: "Name" },
      { key: "id", label: "ID" },
      { key: "state", label: "State", render: (r) => badgeStatus(r.state) },
      { key: "provider", label: "Provider" },
      { key: "k8s_version", label: "Kubernetes" },
      { key: "nodes", label: "Nodes" },
      { key: "driver", label: "Driver" },
      { key: "created", label: "Age", render: (r) => esc(fmtTime(r.created)) },
    ]),
  projects: (main) =>
    pageRancherList(main, "Projects", "/api/v1/rancher/projects", [
      { key: "name", label: "Name" },
      { key: "id", label: "ID" },
      { key: "cluster_id", label: "Cluster" },
      { key: "state", label: "State", render: (r) => badgeStatus(r.state) },
      { key: "description", label: "Description" },
    ]),
  "k8s-ops": (main) => pageK8sOps(main, { scope: "platform" }),
};
