/* Pipeline layout: collapsible sections, GitHub selects, service resources. */

function deploySectionStorageKey(slug, sectionId) {
  return "deploy-section-" + String(slug || "") + "-" + sectionId;
}

function isDeploySectionOpen(slug, sectionId, defaultOpen) {
  try {
    var v = localStorage.getItem(deploySectionStorageKey(slug, sectionId));
    if (v === "0") return false;
    if (v === "1") return true;
  } catch (e) {}
  return defaultOpen !== false;
}

function setDeploySectionOpen(slug, sectionId, open) {
  try {
    localStorage.setItem(deploySectionStorageKey(slug, sectionId), open ? "1" : "0");
  } catch (e) {}
}

function renderDeployCollapsibleCard(slug, sectionId, summaryHtml, bodyHtml, defaultOpen, opts) {
  opts = opts || {};
  var open = isDeploySectionOpen(slug, sectionId, defaultOpen);
  var extraClass = opts.extraClass ? " " + opts.extraClass : "";
  var idAttr = opts.id ? ' id="' + esc(opts.id) + '"' : "";
  var style = opts.style || "margin-bottom:16px";
  return (
    '<details class="card deploy-collapsible' +
    extraClass +
    '"' +
    (open ? " open" : "") +
    idAttr +
    ' style="' +
    esc(style) +
    '" data-deploy-section="' +
    esc(sectionId) +
    '" data-deploy-slug="' +
    esc(slug) +
    '">' +
    '<summary class="deploy-collapsible-summary">' +
    summaryHtml +
    "</summary>" +
    '<div class="deploy-collapsible-body">' +
    bodyHtml +
    "</div></details>"
  );
}

function bindDeployCollapsibleCards(root, slug) {
  if (!root) return;
  var nodes = [];
  if (root.classList && root.classList.contains("deploy-collapsible") && root.getAttribute("data-deploy-section")) {
    nodes.push(root);
  }
  root.querySelectorAll(".deploy-collapsible[data-deploy-section]").forEach(function (el) {
    nodes.push(el);
  });
  nodes.forEach(function (el) {
    if (el.dataset.deployCollapseBound === "1") return;
    el.dataset.deployCollapseBound = "1";
    var sid = el.getAttribute("data-deploy-section");
    var sslug = el.getAttribute("data-deploy-slug") || slug;
    el.addEventListener("toggle", function () {
      setDeploySectionOpen(sslug, sid, el.open);
    });
  });
  root.querySelectorAll(".deploy-collapsible-no-toggle").forEach(function (el) {
    if (el.dataset.deployNoToggleBound === "1") return;
    el.dataset.deployNoToggleBound = "1";
    el.addEventListener("click", function (e) {
      e.stopPropagation();
    });
  });
  root.querySelectorAll(".deploy-collapsible-summary-actions").forEach(function (el) {
    if (el.dataset.deploySummaryActionsBound === "1") return;
    el.dataset.deploySummaryActionsBound = "1";
    el.addEventListener("click", function (e) {
      e.stopPropagation();
    });
  });
}

function pipelineCollapsibleSummaryHtml(summaryExtras) {
  summaryExtras = summaryExtras || "";
  return (
    '<div class="deploy-collapsible-summary-inner">' +
    '<span class="deploy-collapsible-chev" aria-hidden="true"></span>' +
    '<div class="card-title-row deploy-collapsible-title-row">' +
    '<h3 style="margin:0">Pipeline · GitHub &amp; Kiểu chạy</h3>' +
    '<span class="deploy-collapsible-no-toggle">' +
    renderDeployHelpButton("steps") +
    "</span></div>" +
    summaryExtras +
    "</div>"
  );
}

function wrapPipelineSetupCard(slug, bodyHtml, summaryExtras, defaultOpen) {
  return renderDeployCollapsibleCard(slug, "pipeline", pipelineCollapsibleSummaryHtml(summaryExtras), bodyHtml, defaultOpen !== false, {
    id: "pipeline-setup-card",
    extraClass: "pipeline-setup-collapsible",
  });
}

function githubRepoOptionsHtml(repo, ghRepos) {
  if (ghRepos && ghRepos.items === null) {
    return '<option value="" disabled>Đang tải danh sách repo…</option>';
  }
  const items = (ghRepos && ghRepos.items) || [];
  return items
    .map(function (r) {
      const sel = repo.github_owner === r.owner && repo.github_repo === r.name ? " selected" : "";
      return (
        '<option value="' +
        esc(r.owner + "/" + r.name) +
        '" data-branch="' +
        esc(r.default_branch || "main") +
        '"' +
        sel +
        ">" +
        esc(r.full_name || r.name) +
        (r.private ? " 🔒" : "") +
        "</option>"
      );
    })
    .join("");
}

function githubBranchOptionsHtml(branches, selected) {
  selected = (selected || "main").toString();
  const items = branches && branches.length ? branches : [{ name: selected, is_default: true }];
  return items
    .map(function (b) {
      const name = (b.name || b).toString();
      const sel = name === selected ? " selected" : "";
      const label = name + (b.is_default ? " (default)" : "");
      return '<option value="' + esc(name) + '"' + sel + ">" + esc(label) + "</option>";
    })
    .join("");
}

function parseGitHubRepoValue(value) {
  const full = (value || "").toString().trim();
  const parts = full.split("/");
  if (parts.length < 2) return null;
  return { owner: parts[0], repo: parts.slice(1).join("/") };
}

async function loadGitHubBranchSelect(selectEl, owner, repo, selectedBranch) {
  if (!selectEl) return;
  const keep = selectedBranch || selectEl.value || "main";
  selectEl.innerHTML = '<option value="">Đang tải branch…</option>';
  selectEl.disabled = true;
  try {
    const data = await api(
      "/api/v1/github/repos/" + encodeURIComponent(owner) + "/" + encodeURIComponent(repo) + "/branches"
    );
    const branches = data.items || [];
    let sel = keep;
    if (!branches.some(function (b) { return b.name === sel; })) {
      const def = branches.find(function (b) { return b.is_default; });
      sel = def ? def.name : branches[0] ? branches[0].name : "main";
    }
    selectEl.innerHTML = githubBranchOptionsHtml(branches, sel);
  } catch (err) {
    selectEl.innerHTML = githubBranchOptionsHtml([{ name: keep }], keep);
    toastError(err.message || "Không tải được branch từ GitHub");
  } finally {
    selectEl.disabled = false;
  }
}

function buildModeAutoHintHtml(repo) {
  const mode = ((repo && repo.build_mode) || "").toLowerCase();
  const path = repo && repo.build_mode_detected_path;
  if (mode === "buildpack") {
    return (
      '<p class="muted github-setup-hint">Platform tự quét repo: không thấy <code>Dockerfile</code> → build bằng <strong>Buildpack</strong> (tự nhận stack Node, Go, Python…). App listen <code>8080</code>.</p>'
    );
  }
  if (mode === "dockerfile" && path) {
    return (
      '<p class="muted github-setup-hint">Platform tự quét repo: thấy <code>' +
      esc(path) +
      "</code> → build <strong>Docker</strong>. App listen <code>8080</code>.</p>"
    );
  }
  return (
    '<p class="muted github-setup-hint">Platform tự quét repo khi kết nối: có <code>Dockerfile</code> → Docker; không → Buildpack. App listen <code>8080</code>.</p>'
  );
}

function renderBackFrontConventionBanner(conv, canEdit) {
  if (!conv || !conv.enabled) return "";
  const apiBase = (conv.recommended_build && conv.recommended_build.VITE_API_BASE) || "/api";
  return (
    '<div class="convention-banner">' +
    "<strong>Chuẩn Backend + Frontend</strong>" +
    '<p class="muted" style="margin:6px 0">Prod: 1 domain · web <code>/</code> · API <code>/api</code> · frontend gọi <code>' +
    esc(apiBase) +
    "</code></p>" +
    '<p class="muted" style="margin:0;font-size:12px">' +
    esc(conv.dev_local_hint || "Dev máy: proxy /api → backend — không hardcode localhost trong code prod.") +
    "</p>" +
    (canEdit
      ? '<button type="button" class="btn-ghost btn-sm" id="apply-conventions-btn" style="margin-top:8px">Áp dụng env mặc định</button>'
      : "") +
    '<span class="muted" style="font-size:11px;display:block;margin-top:6px">Repo mẫu: <code>templates/back-front/</code> · Prod cấm <code>localhost</code> trong env</span>' +
    "</div>"
  );
}

function bindApplyConventionsButton(main, slug, onDone) {
  const btn = document.getElementById("apply-conventions-btn");
  if (!btn) return;
  btn.onclick = async function () {
    setButtonLoading(btn, true, "Đang áp dụng…");
    try {
      const res = await api("/api/v1/projects/" + encodeURIComponent(slug) + "/conventions/apply", { method: "POST" });
      const n = (res.seeds || []).filter(function (s) { return s.created; }).length;
      toastSuccess(n ? "Đã thêm " + n + " biến env mặc định" : "Env chuẩn đã có sẵn");
      if (onDone) onDone();
    } catch (err) {
      toastError(err.message);
    } finally {
      setButtonLoading(btn, false, "Áp dụng env mặc định");
    }
  };
}

function defaultMultiTemplate(svcData) {
  const template = (svcData && svcData.template) || [];
  if (template.length >= 2) return template;
  return [
    { name: "api", display_name: "API", build_context: "backend", build_mode: "dockerfile", dockerfile_path: "backend/Dockerfile", ingress_path: "/api", health_path: "/health" },
    { name: "web", display_name: "Web", build_context: "frontend", build_mode: "dockerfile", dockerfile_path: "frontend/Dockerfile", ingress_path: "/", health_path: "/" },
  ];
}

function buildModeLabel(mode) {
  return (mode || "").toLowerCase() === "buildpack" ? "Buildpack" : "Docker";
}

function serviceRowIsPublic(s) {
  return s.expose_ingress !== false && String(s.ingress_path || "").toLowerCase() !== "internal" && s.ingress_path !== "-";
}

function serviceCtxCellHtml(idx, s) {
  const val = s.build_context || ".";
  return (
    '<td class="svc-ctx-cell">' +
    '<select name="svc_ctx_sel_' + idx + '" class="svc-ctx-select" data-idx="' + idx + '">' +
    '<option value="' + esc(val) + '" selected>' + esc(val) + "</option>" +
    '<option value="__custom__">Tự nhập…</option></select>' +
    '<input name="svc_ctx_custom_' + idx + '" class="svc-ctx-custom" value="' + esc(val) + '" placeholder="vd. backend" hidden />' +
    '<input name="svc_ctx_' + idx + '" type="hidden" value="' + esc(val) + '" />' +
    "</td>"
  );
}

function stackLabel(stack, mode) {
  if ((mode || "").toLowerCase() === "buildpack" && stack) {
    return " · " + stack;
  }
  return "";
}

function defaultHealthPath(s) {
  s = s || {};
  const ingress = (s.ingress_path || "/").trim();
  if (ingress === "/" || ingress === "") return "/";
  return s.health_path || "/health";
}

function platformDefaultResourcesForService(name) {
  name = String(name || "app").toLowerCase();
  var cpuReq = "100m";
  var memReq = "128Mi";
  var cpuLim = "500m";
  var memLim = "512Mi";
  if (name === "web") {
    cpuReq = "50m";
    memReq = "128Mi";
    cpuLim = "500m";
    memLim = "768Mi";
  } else if (name === "worker") {
    cpuReq = "100m";
    memReq = "256Mi";
    cpuLim = "1000m";
    memLim = "1Gi";
  } else if (name === "dotnet") {
    cpuReq = "100m";
    memReq = "256Mi";
    cpuLim = "1000m";
    memLim = "1Gi";
  } else if (name === "node") {
    cpuReq = "100m";
    memReq = "192Mi";
    cpuLim = "750m";
    memLim = "768Mi";
  }
  return {
    cpu_request: cpuReq,
    memory_request: memReq,
    cpu_limit: cpuLim,
    memory_limit: memLim,
  };
}

function parseK8sCpu(val) {
  val = String(val || "").trim();
  if (!val) return { num: "", unit: "m" };
  if (/m$/i.test(val)) return { num: val.replace(/m$/i, ""), unit: "m" };
  return { num: val, unit: "core" };
}

function parseK8sMem(val) {
  val = String(val || "").trim();
  if (!val) return { num: "", unit: "Mi" };
  if (/Gi$/i.test(val)) return { num: val.replace(/Gi$/i, ""), unit: "Gi" };
  if (/Mi$/i.test(val)) return { num: val.replace(/Mi$/i, ""), unit: "Mi" };
  return { num: val, unit: "Mi" };
}

function formatK8sCpu(num, unit) {
  num = String(num || "").trim();
  if (!num) return "";
  unit = unit || "m";
  if (unit === "core") return num;
  return num + "m";
}

function formatK8sMem(num, unit) {
  num = String(num || "").trim();
  if (!num) return "";
  return num + (unit === "Gi" ? "Gi" : "Mi");
}

function resourceFieldsForService(s) {
  s = s || {};
  var mode = s.resources_mode || "platform";
  if (mode === "custom") {
    return {
      mode: mode,
      disabled: false,
      cpu_request: s.cpu_request || "",
      memory_request: s.memory_request || "",
      cpu_limit: s.cpu_limit || "",
      memory_limit: s.memory_limit || "",
    };
  }
  if (mode === "none") {
    return {
      mode: mode,
      disabled: true,
      none: true,
      cpu_request: "",
      memory_request: "",
      cpu_limit: "",
      memory_limit: "",
    };
  }
  var defs = platformDefaultResourcesForService(s.name || "app");
  return {
    mode: "platform",
    disabled: true,
    preset: true,
    cpu_request: defs.cpu_request,
    memory_request: defs.memory_request,
    cpu_limit: defs.cpu_limit,
    memory_limit: defs.memory_limit,
  };
}

function resourceFieldNames(prefix, idx) {
  if (prefix === "app") {
    return {
      cpuReqVal: "app_cpu_req_val",
      cpuReqUnit: "app_cpu_req_unit",
      memReqVal: "app_mem_req_val",
      memReqUnit: "app_mem_req_unit",
      cpuLimVal: "app_cpu_lim_val",
      cpuLimUnit: "app_cpu_lim_unit",
      memLimVal: "app_mem_lim_val",
      memLimUnit: "app_mem_lim_unit",
    };
  }
  return {
    cpuReqVal: "svc_cpu_req_val_" + idx,
    cpuReqUnit: "svc_cpu_req_unit_" + idx,
    memReqVal: "svc_mem_req_val_" + idx,
    memReqUnit: "svc_mem_req_unit_" + idx,
    cpuLimVal: "svc_cpu_lim_val_" + idx,
    cpuLimUnit: "svc_cpu_lim_unit_" + idx,
    memLimVal: "svc_mem_lim_val_" + idx,
    memLimUnit: "svc_mem_lim_unit_" + idx,
  };
}

function resourceCellHtml(kind, k8sValue, nameVal, nameUnit, fields, label) {
  var parsed = kind === "cpu" ? parseK8sCpu(k8sValue) : parseK8sMem(k8sValue);
  var cls =
    "svc-res-input svc-res-num" +
    (fields.preset ? " svc-res-preset" : "") +
    (fields.none ? " svc-res-none" : "");
  var unitCls = "svc-res-unit" + (fields.disabled ? " svc-res-unit-readonly" : "");
  if (fields.disabled) {
    var unitLabel = fields.none ? "—" : kind === "cpu" ? (parsed.unit === "core" ? "lõi" : "m") : parsed.unit;
    return (
      '<div class="svc-res-cell" title="' + esc(label) + '">' +
      '<span class="svc-res-cell-label">' + esc(label) + "</span>" +
      '<div class="svc-res-cell-inputs">' +
      '<input name="' +
      nameVal +
      '" class="' +
      cls +
      '" value="' +
      esc(parsed.num) +
      '" readonly="readonly" disabled />' +
      '<span class="' +
      unitCls +
      '">' +
      esc(unitLabel) +
      "</span></div></div>"
    );
  }
  var unitHtml;
  if (kind === "cpu") {
    unitHtml =
      '<select name="' +
      nameUnit +
      '" class="' +
      unitCls +
      '">' +
      '<option value="m"' +
      (parsed.unit === "m" ? " selected" : "") +
      ">m</option>" +
      '<option value="core"' +
      (parsed.unit === "core" ? " selected" : "") +
      ">lõi</option></select>";
  } else {
    unitHtml =
      '<select name="' +
      nameUnit +
      '" class="' +
      unitCls +
      '">' +
      '<option value="Mi"' +
      (parsed.unit === "Mi" ? " selected" : "") +
      ">Mi</option>" +
      '<option value="Gi"' +
      (parsed.unit === "Gi" ? " selected" : "") +
      ">Gi</option></select>";
  }
  return (
    '<div class="svc-res-cell" title="' + esc(label) + '">' +
    '<span class="svc-res-cell-label">' + esc(label) + "</span>" +
    '<div class="svc-res-cell-inputs">' +
    '<input type="number" min="1" step="1" name="' +
    nameVal +
    '" class="' +
    cls +
    '" value="' +
    esc(parsed.num) +
    '" placeholder="—" />' +
    unitHtml +
    "</div></div>"
  );
}

function resourceInputGridHtml(prefix, idx, s) {
  var fields = resourceFieldsForService(s);
  var names = resourceFieldNames(prefix, idx);
  return (
    '<div class="svc-res-grid">' +
    resourceCellHtml("cpu", fields.cpu_request, names.cpuReqVal, names.cpuReqUnit, fields, "CPU req") +
    resourceCellHtml("mem", fields.memory_request, names.memReqVal, names.memReqUnit, fields, "RAM req") +
    resourceCellHtml("cpu", fields.cpu_limit, names.cpuLimVal, names.cpuLimUnit, fields, "CPU lim") +
    resourceCellHtml("mem", fields.memory_limit, names.memLimVal, names.memLimUnit, fields, "RAM lim") +
    "</div>"
  );
}

function resourcesModeSelectHtml(name, mode, idx) {
  mode = mode || "platform";
  const idxAttr = idx != null && idx !== "" ? ' data-idx="' + idx + '"' : "";
  return (
    '<select name="' + name + '" class="svc-res-mode"' + idxAttr + ">" +
    '<option value="platform"' + (mode === "platform" ? " selected" : "") + ">Mặc định platform</option>" +
    '<option value="none"' + (mode === "none" ? " selected" : "") + ">Không set</option>" +
    '<option value="custom"' + (mode === "custom" ? " selected" : "") + ">Tùy chỉnh</option>" +
    "</select>"
  );
}

function serviceResourcesInputsHtml(s, fieldPrefix) {
  if ((fieldPrefix || "app") === "app") {
    return resourceInputGridHtml("app", "app", s);
  }
  return resourceInputGridHtml("svc", fieldPrefix, s);
}

function readServiceResourcesFromForm(form, idx) {
  if (!form) return { resources_mode: "platform", cpu_request: "", memory_request: "", cpu_limit: "", memory_limit: "" };
  var mode;
  if (idx === "app") {
    mode = (form.querySelector('[name="app_res_mode"]') || {}).value || "platform";
  } else {
    mode = (form.querySelector('[name="svc_res_mode_' + idx + '"]') || {}).value || "platform";
  }
  var out = {
    resources_mode: mode,
    cpu_request: "",
    memory_request: "",
    cpu_limit: "",
    memory_limit: "",
  };
  if (mode !== "custom") return out;
  var names = idx === "app" ? resourceFieldNames("app", "app") : resourceFieldNames("svc", idx);
  out.cpu_request = formatK8sCpu(
    (form.querySelector('[name="' + names.cpuReqVal + '"]') || {}).value,
    (form.querySelector('[name="' + names.cpuReqUnit + '"]') || {}).value || "m"
  );
  out.memory_request = formatK8sMem(
    (form.querySelector('[name="' + names.memReqVal + '"]') || {}).value,
    (form.querySelector('[name="' + names.memReqUnit + '"]') || {}).value || "Mi"
  );
  out.cpu_limit = formatK8sCpu(
    (form.querySelector('[name="' + names.cpuLimVal + '"]') || {}).value,
    (form.querySelector('[name="' + names.cpuLimUnit + '"]') || {}).value || "m"
  );
  out.memory_limit = formatK8sMem(
    (form.querySelector('[name="' + names.memLimVal + '"]') || {}).value,
    (form.querySelector('[name="' + names.memLimUnit + '"]') || {}).value || "Mi"
  );
  return out;
}

function serviceNameForResourceIdx(form, idx) {
  if (idx === "app") return "app";
  var nameEl = form.querySelector('[name="svc_name_' + idx + '"]');
  return nameEl && nameEl.value ? nameEl.value : "";
}

function applyResourceFields(form, idx, partial) {
  if (!form) return;
  var s = Object.assign(
    {
      name: serviceNameForResourceIdx(form, idx),
      resources_mode: idx === "app"
        ? ((form.querySelector('[name="app_res_mode"]') || {}).value || "platform")
        : ((form.querySelector('[name="svc_res_mode_' + idx + '"]') || {}).value || "platform"),
    },
    partial || {}
  );
  if (idx === "app") {
    var grid = form.querySelector(".single-resources-panel .svc-res-grid");
    if (grid) {
      grid.outerHTML = resourceInputGridHtml("app", "app", s);
    }
    return;
  }
  var card = form.querySelector('.service-resources-card[data-svc-res-idx="' + idx + '"]');
  if (!card) return;
  var gridEl = card.querySelector(".svc-res-grid");
  if (gridEl) gridEl.outerHTML = resourceInputGridHtml("svc", idx, s);
}

function toggleServiceResourceInputs(form, idx) {
  if (!form) return;
  var mode;
  if (idx === "app") {
    mode = (form.querySelector('[name="app_res_mode"]') || {}).value || "platform";
  } else {
    mode = (form.querySelector('[name="svc_res_mode_' + idx + '"]') || {}).value || "platform";
  }
  var s = {
    name: serviceNameForResourceIdx(form, idx),
    resources_mode: mode,
  };
  if (mode === "custom") {
    var names = idx === "app" ? resourceFieldNames("app", "app") : resourceFieldNames("svc", idx);
    var cpuReqEl = form.querySelector('[name="' + names.cpuReqVal + '"]');
    if (cpuReqEl && !cpuReqEl.value) {
      var defs = platformDefaultResourcesForService(s.name || "app");
      s = Object.assign(s, defs);
    } else {
      s.cpu_request = formatK8sCpu(
        (cpuReqEl || {}).value,
        (form.querySelector('[name="' + names.cpuReqUnit + '"]') || {}).value || "m"
      );
      s.memory_request = formatK8sMem(
        (form.querySelector('[name="' + names.memReqVal + '"]') || {}).value,
        (form.querySelector('[name="' + names.memReqUnit + '"]') || {}).value || "Mi"
      );
      s.cpu_limit = formatK8sCpu(
        (form.querySelector('[name="' + names.cpuLimVal + '"]') || {}).value,
        (form.querySelector('[name="' + names.cpuLimUnit + '"]') || {}).value || "m"
      );
      s.memory_limit = formatK8sMem(
        (form.querySelector('[name="' + names.memLimVal + '"]') || {}).value,
        (form.querySelector('[name="' + names.memLimUnit + '"]') || {}).value || "Mi"
      );
    }
  }
  applyResourceFields(form, idx, s);
}

function cpuToMilliClient(num, unit) {
  num = parseInt(String(num || "").trim(), 10);
  if (!num || num < 1) return 0;
  if (unit === "core") return num * 1000;
  return num;
}

function memToMiClient(num, unit) {
  num = parseInt(String(num || "").trim(), 10);
  if (!num || num < 1) return 0;
  if (unit === "Gi") return num * 1024;
  return num;
}

function validateServiceResourcesEntry(res, serviceName) {
  if (!res || res.resources_mode !== "custom") return null;
  serviceName = serviceName || "app";
  var hasAny =
    res.cpu_request || res.memory_request || res.cpu_limit || res.memory_limit;
  if (!hasAny) {
    return serviceName + ": tùy chỉnh cần ít nhất một giá trị CPU hoặc RAM";
  }
  var cpuReqMilli = res.cpu_request ? cpuToMilliClient(parseK8sCpu(res.cpu_request).num, parseK8sCpu(res.cpu_request).unit) : 0;
  var cpuLimMilli = res.cpu_limit ? cpuToMilliClient(parseK8sCpu(res.cpu_limit).num, parseK8sCpu(res.cpu_limit).unit) : 0;
  var memReqMi = res.memory_request ? memToMiClient(parseK8sMem(res.memory_request).num, parseK8sMem(res.memory_request).unit) : 0;
  var memLimMi = res.memory_limit ? memToMiClient(parseK8sMem(res.memory_limit).num, parseK8sMem(res.memory_limit).unit) : 0;
  if (res.cpu_request) {
    var cpuReq = parseK8sCpu(res.cpu_request);
    var n = parseInt(cpuReq.num, 10);
    if (cpuReq.unit === "core" && (n < 1 || n > 32)) return serviceName + ": CPU request lõi: 1–32";
    if (cpuReq.unit === "m" && (n < 1 || n > 32000)) return serviceName + ": CPU request m: 1–32000";
  }
  if (res.cpu_limit) {
    var cpuLim = parseK8sCpu(res.cpu_limit);
    var ln = parseInt(cpuLim.num, 10);
    if (cpuLim.unit === "core" && (ln < 1 || ln > 32)) return serviceName + ": CPU limit lõi: 1–32";
    if (cpuLim.unit === "m" && (ln < 1 || ln > 32000)) return serviceName + ": CPU limit m: 1–32000";
  }
  if (res.memory_request) {
    var memReq = parseK8sMem(res.memory_request);
    var mn = parseInt(memReq.num, 10);
    if (memReq.unit === "Gi" && (mn < 1 || mn > 64)) return serviceName + ": RAM request Gi: 1–64";
    if (memReq.unit === "Mi" && (mn < 32 || mn > 32768)) return serviceName + ": RAM request Mi: 32–32768";
  }
  if (res.memory_limit) {
    var memLim = parseK8sMem(res.memory_limit);
    var lmn = parseInt(memLim.num, 10);
    if (memLim.unit === "Gi" && (lmn < 1 || lmn > 64)) return serviceName + ": RAM limit Gi: 1–64";
    if (memLim.unit === "Mi" && (lmn < 32 || lmn > 32768)) return serviceName + ": RAM limit Mi: 32–32768";
  }
  if (cpuReqMilli && cpuLimMilli && cpuReqMilli > cpuLimMilli) {
    return serviceName + ": CPU request không được lớn hơn CPU limit";
  }
  if (memReqMi && memLimMi && memReqMi > memLimMi) {
    return serviceName + ": RAM request không được lớn hơn RAM limit";
  }
  return null;
}

function validateAllServiceResources(form) {
  if (!form) return null;
  var checked = form.querySelector('input[name="layout"]:checked');
  var layout = checked ? checked.value : "single";
  var errors = [];
  if (layout === "single") {
    var e1 = validateServiceResourcesEntry(readServiceResourcesFromForm(form, "app"), "app");
    if (e1) errors.push(e1);
  } else {
    form.querySelectorAll(".service-resources-card").forEach(function (card) {
      var idx = card.getAttribute("data-svc-res-idx");
      if (idx == null) return;
      var name = serviceNameForResourceIdx(form, idx) || "service " + idx;
      var e2 = validateServiceResourcesEntry(readServiceResourcesFromForm(form, idx), name);
      if (e2) errors.push(e2);
    });
  }
  return errors.length ? errors.join(" · ") : null;
}

function renderResourcesActionsHtml() {
  if (typeof canWriteK8s === "function" && !canWriteK8s()) return "";
  return (
    '<div class="svc-res-actions">' +
    '<button type="button" class="btn-primary btn-sm" id="resources-save-apply">Lưu &amp; áp dụng CPU/RAM</button>' +
    '<button type="button" class="btn-ghost btn-sm" id="resources-save-draft">Chỉ lưu nháp</button>' +
    "</div>"
  );
}

function renderSingleResourcesPanel(s) {
  s = s || {};
  const mode = s.resources_mode || "platform";
  return (
    '<div class="single-resources-panel" style="margin-top:12px">' +
    "<strong>CPU / RAM</strong>" +
    '<p class="muted" style="margin:6px 0 8px;font-size:12px">Mặc định platform = preset an toàn · Không set = không inject limits · Tùy chỉnh: nhập số + chọn đơn vị</p>' +
    '<div class="svc-res-row">' +
    resourcesModeSelectHtml("app_res_mode", mode, "app") +
    serviceResourcesInputsHtml(s, "app") +
    "</div>" +
    renderResourcesActionsHtml() +
    "</div>"
  );
}

function renderServiceResourcesCard(s, idx) {
  return (
    '<div class="service-resources-card" data-svc-res-idx="' + idx + '">' +
    '<div class="service-resources-card-head"><strong>' + esc((s && (s.display_name || s.name)) || ("Service " + idx)) + "</strong></div>" +
    '<div class="svc-res-row">' +
    resourcesModeSelectHtml("svc_res_mode_" + idx, (s && s.resources_mode) || "platform", idx) +
    resourceInputGridHtml("svc", idx, s) +
    "</div></div>"
  );
}

function buildServiceTableRowHtml(s, idx) {
  const pub = serviceRowIsPublic(s);
  const stack = s.stack || "";
  return (
    '<tr data-svc-idx="' + idx + '">' +
    '<td><input name="svc_name_' + idx + '" value="' + esc(s.name || "") + '" /></td>' +
    '<td><select name="svc_mode_' + idx + '">' +
    '<option value="dockerfile"' + ((s.build_mode || "dockerfile") !== "buildpack" ? " selected" : "") + ">Docker</option>" +
    '<option value="buildpack"' + ((s.build_mode || "") === "buildpack" ? " selected" : "") + ">Buildpack</option>" +
    "</select></td>" +
    '<td><select name="svc_stack_' + idx + '">' +
    '<option value=""' + (!stack ? " selected" : "") + ">auto</option>" +
    '<option value="python"' + (stack === "python" ? " selected" : "") + ">python</option>" +
    '<option value="node"' + (stack === "node" ? " selected" : "") + ">node</option>" +
    '<option value="go"' + (stack === "go" ? " selected" : "") + ">go</option>" +
    '<option value="dotnet"' + (stack === "dotnet" ? " selected" : "") + ">dotnet</option>" +
    "</select></td>" +
    serviceCtxCellHtml(idx, s) +
    '<td><input name="svc_df_' + idx + '" value="' + esc(s.dockerfile_path || "Dockerfile") + '" placeholder="Dockerfile" /></td>' +
    '<td><label class="svc-public-label"><input type="checkbox" name="svc_public_' + idx + '"' + (pub ? " checked" : "") + ' /> Public</label></td>' +
    '<td><input name="svc_ingress_' + idx + '" value="' + esc(pub ? (s.ingress_path || "/") : "-") + '" placeholder="/ hoặc /api" /></td>' +
    '<td><input type="hidden" name="svc_health_' + idx + '" value="' + esc(s.health_path || defaultHealthPath(s)) + '" /></td>' +
    '<td><button type="button" class="btn-ghost btn-sm svc-remove-row" data-idx="' + idx + '">×</button></td>' +
    "</tr>"
  );
}

function renderServicePreviewCard(s) {
  const mode = buildModeLabel(s.build_mode);
  const pub = serviceRowIsPublic(s);
  const access = pub
    ? "URL công khai: <code>" + esc(s.ingress_path || "/") + "*</code>"
    : '<span class="badge warn">Internal</span> · cluster <code>http://' + esc(s.name || "?") + ":80</code>";
  return (
    '<div class="service-preview-card">' +
    "<h4>" + esc(s.display_name || s.name || "?") + ' <span class="badge">' + esc(mode) + esc(stackLabel(s.stack, s.build_mode)) + "</span></h4>" +
    "<p>Thư mục <code>" + esc(s.build_context || ".") + "</code> → image <code>" + esc(s.name) + "</code><br>" +
    access + "</p></div>"
  );
}

function renderServicesContractBanner(contract, canEdit) {
  contract = contract || {};
  if (!contract.found) {
    return (
      '<p class="muted repo-detect-hint" style="font-size:12px;margin:0 0 10px">Chưa thấy <code>.platform/services.yaml</code> — Console quét Dockerfile để gợi ý build (Docker / Buildpack).</p>'
    );
  }
  if (contract.parse_error) {
    return (
      '<div class="banner warn" style="margin-bottom:10px"><strong>services.yaml</strong> — ' +
      esc(contract.parse_error) +
      "</div>"
    );
  }
  const svcs = contract.services || [];
  const names = (contract.service_names || svcs.map(function (s) { return s.name; })).filter(Boolean);
  const synced = !!contract.in_sync;
  const layoutLabel = layoutKindLabel(contract.suggested_layout || contract.layout || "single");
  let html =
    '<div class="banner repo-detect-banner' +
    (synced ? "" : " warn") +
    '" style="margin-bottom:8px"><strong>Repo gợi ý</strong> · ' +
    esc(layoutLabel);
  if (names.length) {
    html += " · <code>" + esc(names.join(" + ")) + "</code>";
  }
  html +=
    ' · branch <code>' +
    esc(contract.branch || "?") +
    "</code>";
  html += synced
    ? ' — <span style="color:#15803d">khớp Console</span>'
    : ' — <span style="color:#b45309">chưa khớp Console</span>';
  const gitSub = (contract.git_submodules || "").trim();
  if (gitSub || contract.has_gitmodules) {
    const subLabel = gitSub || "recursive";
    const subSync = contract.git_submodules_in_sync !== false;
    html +=
      ' · Submodule <code>' +
      esc(subLabel) +
      "</code>" +
      (subSync ? "" : ' <span style="color:#b45309">(chưa sync workflow)</span>');
  }
  html += "</div>";
  if (canEdit && !synced && !contract.parse_error) {
    const isMulti = (contract.suggested_layout || contract.layout) === "multi";
    const svcCount = (contract.service_names || names).length;
    const btnClass = isMulti ? "btn-primary btn-sm" : "btn-ghost btn-sm";
    const btnLabel = isMulti && svcCount > 2
      ? "Bước 2: Áp dụng fleet từ repo (" + svcCount + " service)"
      : isMulti
        ? "Bước 2: Áp dụng api + web từ repo"
        : "Áp dụng cấu hình từ repo";
    html +=
      '<p class="muted repo-detect-action-hint" style="font-size:12px;margin:0 0 8px">' +
      (isMulti
        ? "Repo đã có <code>services.yaml</code> (api + web) — bấm nút dưới, rồi <strong>Lưu &amp; đồng bộ GitHub</strong> trước khi push."
        : "Repo có cấu hình platform — áp dụng vào Console trước khi sync.") +
      "</p>" +
      '<button type="button" class="' +
      btnClass +
      '" id="sync-services-contract" style="margin-bottom:10px">' +
      esc(btnLabel) +
      "</button>";
  }
  return html;
}
