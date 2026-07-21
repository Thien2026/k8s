/* GitHub pipeline setup form. */

function parseRepoFromForm(form) {
  form = form || document.getElementById("pipeline-setup-form");
  if (!form) return null;
  const repoEl = form.querySelector('[name="repo"]');
  return parseGitHubRepoValue(repoEl && repoEl.value);
}

function layoutKindLabel(layout) {
  return layout === "multi" ? "Web + API riêng" : "Một website";
}

function contractServiceToFormRow(s) {
  s = s || {};
  return {
    name: s.name || "",
    display_name: s.display_name || s.name || "",
    build_mode: s.build_mode || "dockerfile",
    stack: s.stack || "",
    build_context: s.build_context || ".",
    dockerfile_path: s.dockerfile_path || "Dockerfile",
    ingress_path: s.ingress_path || "/",
    expose_ingress: s.expose_ingress !== false,
    health_path: s.health_path || "/health",
    container_port: s.container_port || 8080,
  };
}

function updatePipelineBuildHint(contract) {
  const el = document.getElementById("pipeline-build-hint");
  if (!el || !contract) return;
  el.innerHTML = buildModeAutoHintHtml({
    build_mode: contract.build_mode,
    build_mode_detected_path: contract.build_mode_detected_path,
  });
}

function applyRepoLayoutSuggestion(form, contract) {
  if (!form || !contract || contract.parse_error || !contract.found) return false;
  if (state.pipelineLayoutUserTouched) return false;
  const suggested = contract.suggested_layout || contract.layout || "single";
  const radio = form.querySelector('input[name="layout"][value="' + suggested + '"]');
  if (!radio || radio.checked) return false;
  radio.checked = true;
  radio.dispatchEvent(new Event("change", { bubbles: true }));
  toastSuccess("Repo gợi ý: " + layoutKindLabel(suggested) + " — đã chọn sẵn");
  return true;
}

function applyRepoServicesSuggestion(form, contract) {
  if (!form || !contract || contract.parse_error || !contract.found) return false;
  if (state.pipelineServicesUserTouched) return false;
  if ((contract.suggested_layout || contract.layout) !== "multi") return false;
  const services = contract.services || [];
  if (services.length < 2) return false;
  const tbody = document.getElementById("project-services-tbody");
  if (!tbody) return false;
  const rows = services.map(contractServiceToFormRow);
  tbody.innerHTML = rows
    .map(function (s, idx) {
      return buildServiceTableRowHtml(s, idx);
    })
    .join("");
  toastSuccess("Đã điền " + rows.length + " service từ repo");
  return true;
}

function applyRepoContractSuggestion(form, contract, opts) {
  opts = opts || {};
  const layoutChanged = applyRepoLayoutSuggestion(form, contract);
  const servicesFilled = applyRepoServicesSuggestion(form, contract);
  updatePipelineBuildHint(contract);
  if ((layoutChanged || servicesFilled) && opts.onPrefilled) {
    opts.onPrefilled({ layoutChanged: layoutChanged, servicesFilled: servicesFilled });
  }
  return layoutChanged || servicesFilled;
}

function refreshRepoDetectBanner(contract, canEdit) {
  const panel = document.getElementById("repo-detect-banner-slot");
  if (!panel) return;
  panel.innerHTML = renderServicesContractBanner(contract, canEdit);
  const syncBtn = document.getElementById("sync-services-contract");
  if (syncBtn && state._repoDetectSyncHandler) {
    syncBtn.onclick = state._repoDetectSyncHandler;
  }
}

function renderPipelineCrosscheckHtml(repo, svcData, contract) {
  repo = repo || {};
  svcData = svcData || {};
  contract = contract || svcData.repo_contract || {};
  const layout = (svcData.layout || "single");
  const lines = [];
  let cls = "ok";
  if (repo.workflow_stale && repo.workflow_stale_reason) {
    lines.push(esc(repo.workflow_stale_reason));
    cls = "warn";
  } else if (!repo.workflow_synced_at) {
    lines.push("Workflow chưa khớp Console — cần bấm 「Lưu & đồng bộ GitHub」.");
    cls = "warn";
  }
  if (contract.found && !contract.parse_error) {
    if (contract.layout === "multi" && layout !== "multi") {
      lines.push("Repo cần <strong>Web + API riêng</strong> — chọn đúng kiểu hoặc bấm 「Áp dụng từ repo」.");
      cls = "warn";
    }
    if (contract.layout !== "multi" && layout === "multi") {
      lines.push("Console chọn Web + API nhưng repo chưa có cấu hình multi — cần 「Lưu & đồng bộ GitHub」.");
      cls = "warn";
    }
    if (layout === "multi" && !contract.in_sync && contract.layout === "multi") {
      lines.push("Cấu hình Console khác file <code>services.yaml</code> trên branch <code>" + esc(contract.branch || "?") + "</code>.");
      cls = "warn";
    }
    if (contract.git_submodules && contract.git_submodules_in_sync === false) {
      lines.push("Submodule <code>" + esc(contract.git_submodules) + "</code> chưa sync workflow.");
      cls = "warn";
    }
  }
  if (lines.length === 0) {
    return '<div id="pipeline-crosscheck" class="pipeline-crosscheck ok">✓ Repo và kiểu chạy nhất quán — sẵn sàng đồng bộ workflow.</div>';
  }
  return '<div id="pipeline-crosscheck" class="pipeline-crosscheck ' + cls + '">' + lines.map(function (l) { return "• " + l; }).join("<br>") + "</div>";
}

async function refreshPipelineCrosscheck(slug, form, svcData, repo, opts) {
  opts = opts || {};
  const el = document.getElementById("pipeline-crosscheck");
  if (!el || !form) return null;
  const parsed = parseRepoFromForm(form);
  const branch = selectedGitHubBranch(repo && repo.branch);
  if (!parsed) {
    el.className = "pipeline-crosscheck warn";
    el.innerHTML = "• Chọn repository và branch — Console sẽ quét repo và gợi ý kiểu chạy.";
    return null;
  }
  el.className = "pipeline-crosscheck";
  el.innerHTML = '<span class="btn-spinner"></span> Đang quét branch <code>' + esc(branch) + "</code>…";
  try {
    const contract = await api(
      "/api/v1/projects/" +
        encodeURIComponent(slug) +
        "/services/detect" +
        qs({ branch: branch, owner: parsed.owner, repo: parsed.repo })
    );
    applyRepoContractSuggestion(form, contract, opts);
    refreshRepoDetectBanner(contract, true);
    const merged = Object.assign({}, svcData || {}, {
      repo_contract: contract,
      layout: collectProjectLayoutPayload(form).layout,
    });
    const html = renderPipelineCrosscheckHtml(repo, merged, contract);
    const newEl = document.getElementById("pipeline-crosscheck");
    if (newEl) {
      newEl.outerHTML = html;
    }
    return contract;
  } catch (err) {
    el.className = "pipeline-crosscheck warn";
    el.innerHTML = "• Không quét được repo: " + esc(errorMessage(err));
    return null;
  }
}

var PIPELINE_SETUP_STEPS = [
  "Lưu kiểu chạy",
  "Lưu repo & branch",
  "Push workflow GitHub",
  "Inject secrets (Harbor + deploy token)",
];

function renderPipelinePolicyCallout() {
  return (
    '<div class="pipeline-policy-callout">' +
    '<div class="pipeline-policy-head">' +
    "<strong>Quy tắc</strong>" +
    renderDeployHelpButton("rules", "btn-help-inline") +
    "</div>" +
    "<ul>" +
    "<li><strong>Deploy lại</strong> — đổi tag/image, <em>cùng</em> kiểu chạy (Một website hoặc Web + API).</li>" +
    "<li><strong>Đổi kiểu chạy</strong> — đổi topology (single ↔ multi), deploy bản mới; <em>không</em> dùng rollback.</li>" +
    "<li>Kiểu chạy chốt ở <strong>bước 2</strong> — Console gợi ý từ repo sau khi chọn branch.</li>" +
    "</ul></div>"
  );
}

function pipelineCardTitleHtml() {
  return (
    '<div class="card-title-row">' +
    '<h3 style="margin:0">Pipeline · GitHub &amp; Kiểu chạy</h3>' +
    renderDeployHelpButton("steps") +
    "</div>"
  );
}

function collectProjectLayoutPayload(form) {
  if (!form) return { layout: "single", services: [] };
  const checked = form.querySelector('input[name="layout"]:checked');
  const layout = checked ? checked.value : "single";
  const services = [];
  if (layout === "single") {
    const res = readServiceResourcesFromForm(form, "app");
    services.push(Object.assign({ name: "app", container_port: 8080, health_path: "/health", ingress_path: "/" }, res));
  }
  if (layout === "multi") {
    const tbody = document.getElementById("project-services-tbody");
    if (tbody) {
      Array.prototype.slice.call(tbody.querySelectorAll("tr")).forEach(function (tr) {
        const idx = tr.getAttribute("data-svc-idx");
        const modeEl = form.querySelector('[name="svc_mode_' + idx + '"]');
        const pubEl = form.querySelector('[name="svc_public_' + idx + '"]');
        const expose = pubEl ? pubEl.checked : true;
        const ingressEl = form.querySelector('[name="svc_ingress_' + idx + '"]');
        let ingress = ingressEl ? ingressEl.value : "/";
        if (!expose) ingress = "-";
        const stackEl = form.querySelector('[name="svc_stack_' + idx + '"]');
        const ctxSel = form.querySelector('[name="svc_ctx_sel_' + idx + '"]');
        const ctxHidden = form.querySelector('[name="svc_ctx_' + idx + '"]');
        const ctxCustom = form.querySelector('[name="svc_ctx_custom_' + idx + '"]');
        let buildContext = ".";
        if (ctxSel && ctxSel.value === "__custom__" && ctxCustom) {
          buildContext = ctxCustom.value || ".";
        } else if (ctxHidden && ctxHidden.value) {
          buildContext = ctxHidden.value;
        } else if (ctxSel && ctxSel.value && ctxSel.value !== "__custom__") {
          buildContext = ctxSel.value;
        }
        const healthEl = form.querySelector('[name="svc_health_' + idx + '"]');
        const ingressVal = expose ? (ingressEl ? ingressEl.value : "/") : "-";
        let healthPath = healthEl ? healthEl.value : "/health";
        if (expose && (ingressVal === "/" || ingressVal === "") && !healthEl) healthPath = "/";
        services.push({
          name: (form.querySelector('[name="svc_name_' + idx + '"]') || {}).value || "",
          build_mode: modeEl ? modeEl.value : "dockerfile",
          stack: stackEl ? stackEl.value : "",
          build_context: buildContext,
          dockerfile_path: (form.querySelector('[name="svc_df_' + idx + '"]') || {}).value || "Dockerfile",
          ingress_path: ingressVal,
          expose_ingress: expose,
          container_port: 8080,
          health_path: healthPath,
          sort_order: parseInt(idx, 10) || 0,
        });
        const res = readServiceResourcesFromForm(form, idx);
        Object.assign(services[services.length - 1], res);
      });
    }
  }
  return { layout: layout, services: services };
}

async function runGitHubPipelineSetup(slug, opts) {
  opts = opts || {};
  const body = {
    owner: opts.owner,
    repo: opts.repo,
    branch: opts.branch || "main",
    environment: opts.environment || "dev",
  };
  if (opts.apply_repo_contract) {
    body.apply_repo_contract = true;
  } else {
    const layoutPayload = opts.layoutPayload || { layout: "single", services: [] };
    body.layout = layoutPayload.layout;
    body.services = layoutPayload.services;
  }
  const progress = opts.progressEl;
  const submitBtn = opts.submitBtn;
  const formRoot = opts.formRoot;
  const steps = PIPELINE_SETUP_STEPS;
  if (progress) {
    progress.hidden = false;
    progress.innerHTML =
      '<div class="setup-progress-title"><span class="btn-spinner"></span> Đang đồng bộ pipeline…</div>' +
      steps
        .map(function (s) {
          return '<div class="setup-step setup-step-pending">' + esc(s) + "</div>";
        })
        .join("");
  }
  if (submitBtn) setButtonLoading(submitBtn, true, "Đang đồng bộ…");
  if (formRoot) {
    formRoot.querySelectorAll("input, select, button").forEach(function (el) {
      if (el !== submitBtn) el.disabled = true;
    });
  }
  let stepIdx = 0;
  const stepTimer = setInterval(function () {
    if (!progress) return;
    progress.querySelectorAll(".setup-step").forEach(function (el, i) {
      el.className = "setup-step " + (i < stepIdx ? "setup-step-done" : i === stepIdx ? "setup-step-run" : "setup-step-pending");
    });
    if (stepIdx < steps.length - 1) stepIdx++;
  }, 900);
  try {
    const res = await api("/api/v1/projects/" + encodeURIComponent(slug) + "/github/setup", {
      method: "POST",
      body: body,
    });
    clearInterval(stepTimer);
    if (progress) {
      progress.querySelectorAll(".setup-step").forEach(function (el) {
        el.className = "setup-step setup-step-done";
      });
      progress.innerHTML =
        '<div class="setup-progress-title setup-progress-ok">✓ Pipeline sẵn sàng — layout + workflow đồng bộ</div>' +
        steps
          .map(function (s) {
            return '<div class="setup-step setup-step-done">' + esc(s) + "</div>";
          })
          .join("");
      setTimeout(function () {
        progress.hidden = true;
      }, 4000);
    }
    return res;
  } catch (err) {
    clearInterval(stepTimer);
    if (progress) {
      renderSetupSyncError(progress, errorMessage(err, "Đồng bộ pipeline thất bại"), steps);
    }
    throw err;
  } finally {
    if (submitBtn) setButtonLoading(submitBtn, false, "Lưu & đồng bộ GitHub");
    if (formRoot) {
      formRoot.querySelectorAll("input, select, button").forEach(function (el) {
        el.disabled = false;
      });
    }
  }
}

function renderPipelineSetupCard(slug, svcData, repo, ghStatus, ghRepos, canEdit) {
  repo = repo || {};
  ghStatus = ghStatus || {};
  ghRepos = ghRepos || { items: [] };
  svcData = svcData || {};
  const layout = svcData.layout || "single";
  const items = svcData.items || [];
  const isMulti = layout === "multi";
  const tpl = defaultMultiTemplate(svcData);
  const multiItems = isMulti && items.length >= 2 ? items : tpl;
  const conv = svcData.conventions || null;
  const repoContract = svcData.repo_contract || null;
  const conventionBanner = isMulti ? renderBackFrontConventionBanner(conv, canEdit) : "";
  const contractBanner = renderServicesContractBanner(repoContract, canEdit);
  const ghRepoOpts = githubRepoOptionsHtml(repo, ghRepos);

  const singleHint =
    '<div id="layout-single-hint" class="layout-hint-panel"' + (isMulti ? ' hidden' : "") + ">" +
    "<strong>Một website</strong> — một link, một app. Platform tự quét Dockerfile trên branch đã chọn.<br>" +
    (repo.build_mode
      ? "Gần nhất: <strong>" + esc(buildModeLabel(repo.build_mode)) + "</strong>" +
        (repo.build_mode_detected_path ? " · <code>" + esc(repo.build_mode_detected_path) + "</code>" : "") +
        " · listen <code>8080</code>"
      : "Chọn branch → kiểm tra tự động bên trên.") +
    renderSingleResourcesPanel((items[0] || {})) +
    "</div>";

  const multiPanel =
    '<div id="layout-multi-panel"' + (!isMulti ? ' hidden' : "") + ">" +
    conventionBanner +
    '<p class="muted" style="margin:0 0 10px">N service · public (Ingress) hoặc internal. Env discovery <code>SVC_&lt;TÊN&gt;_URL</code>.</p>' +
    '<div class="service-preview-grid" id="service-preview-grid">' +
    multiItems.map(renderServicePreviewCard).join("") +
    "</div>" +
    '<div class="service-resources-panel" id="service-resources-panel">' +
    "<h4>CPU / RAM từng service</h4>" +
    '<p class="muted" style="margin:0 0 10px;font-size:12px">Chọn <strong>Mặc định platform</strong> · <strong>Không set</strong> · <strong>Tùy chỉnh</strong> (số + đơn vị). Bấm <strong>Lưu &amp; áp dụng</strong> để restart pod với limits mới — không cần đồng bộ GitHub.</p>' +
    '<div class="service-resources-grid" id="service-resources-grid">' +
    multiItems.map(function (s, idx) { return renderServiceResourcesCard(s, idx); }).join("") +
    "</div>" +
    renderResourcesActionsHtml() +
    "</div>" +
    '<details class="layout-advanced-details"><summary>Tùy chỉnh service (dev) — build path, ingress</summary>' +
    '<p class="muted" id="github-dir-hint" style="margin:8px 0">Thư mục build theo branch đã chọn ở bước 1.</p>' +
    '<button type="button" class="btn-ghost btn-sm" id="refresh-github-dirs" style="margin-bottom:8px">Quét thư mục từ GitHub</button>' +
    '<table class="data-table"><thead><tr><th>Tên</th><th>Build</th><th>Stack</th><th>Thư mục</th><th>Dockerfile</th><th>Public</th><th>Ingress</th><th></th></tr></thead>' +
    '<tbody id="project-services-tbody">' +
    multiItems.map(function (s, idx) { return buildServiceTableRowHtml(s, idx); }).join("") +
    "</tbody></table>" +
    '<button type="button" class="btn-ghost btn-sm" id="project-services-add-row" style="margin-top:8px">+ Thêm service</button>' +
    "</details></div>";

  const statusChips =
    '<div class="pipeline-status-chips">' +
    chip("Kiểu chạy", isMulti ? "Web + API riêng" : "Một website") +
    (ghStatus.connected ? chip("GitHub", "@" + (ghStatus.login || "?")) : "") +
    (repo.workflow_stale
      ? '<span class="badge warn" title="' + esc(repo.workflow_stale_reason || "") + '">Cần đồng bộ</span>'
      : repo.workflow_synced_at && repo.auto_deploy_enabled
        ? '<span class="badge ok">Workflow OK</span>'
        : repo.workflow_synced_at
          ? '<span class="badge warn">Workflow cũ</span>'
          : '<span class="badge warn">Chưa đồng bộ</span>') +
    "</div>";

  const crosscheck = renderPipelineCrosscheckHtml(repo, svcData, repoContract);

  if (!ghStatus.enabled) {
    return wrapPipelineSetupCard(
      slug,
      renderDeployHelpInlineCard(slug) + '<p class="muted">GitHub OAuth chưa cấu hình trên VPS.</p>',
      "",
      true
    );
  }

  if (!ghStatus.connected) {
    return wrapPipelineSetupCard(
      slug,
      renderDeployHelpInlineCard(slug) +
        renderPipelinePolicyCallout() +
        '<p class="muted">Kết nối GitHub → chọn repo/branch → chốt kiểu chạy → đồng bộ workflow.</p>' +
        (canEdit ? '<button type="button" class="btn-primary" id="github-connect-btn">① Kết nối GitHub</button>' : ""),
      "",
      true
    );
  }

  if (!canEdit) {
    return wrapPipelineSetupCard(
      slug,
      renderDeployHelpInlineCard(slug) +
        renderPipelinePolicyCallout() +
        crosscheck +
        '<div class="meta-chips">' +
        chip("Repo", (repo.github_owner || "") + "/" + (repo.github_repo || "")) +
        chip("Branch", repo.branch || "main") +
        "</div>" +
        (isMulti
          ? '<div class="service-preview-grid" style="margin-top:12px">' + multiItems.map(renderServicePreviewCard).join("") + "</div>"
          : singleHint),
      statusChips,
      true
    );
  }

  return wrapPipelineSetupCard(
    slug,
    renderDeployHelpInlineCard(slug) +
    renderPipelinePolicyCallout() +
    crosscheck +
    '<form id="pipeline-setup-form" class="login-form pipeline-wizard">' +
    renderPipelineStepCollapsible(slug, 1, "Nguồn GitHub",
    '<p class="muted" style="margin:0 0 10px">Đã kết nối <strong>@' + esc(ghStatus.login || "") + "</strong></p>" +
    '<label>Repository' +
    selectWrapHtml("github-repo-select", '<option value="">— chọn repo —</option>' + ghRepoOpts, { name: "repo", required: true }) +
    "</label>" +
    '<div class="form-row">' +
    '<label>Branch' +
    selectWrapHtml("github-branch-select", githubBranchOptionsHtml([], repo.branch || "main"), { name: "branch", required: true }) +
    "</label>" +
    '<label>Deploy env' +
    selectWrapHtml(
      "",
      '<option value="dev"' + (repo.deploy_environment !== "prod" ? " selected" : "") + ">dev</option>" +
        '<option value="prod"' + (repo.deploy_environment === "prod" ? " selected" : "") + ">prod (push → deploy thẳng)</option>",
      { name: "environment" }
    ) +
    "</label></div>" +
    (repo.deploy_environment === "prod"
      ? '<p class="muted pipeline-prod-warn">⚠ Deploy env = <strong>prod</strong> — mỗi push lên branch này build và deploy <em>trực tiếp</em> lên production (không qua dev).</p>'
      : "") +
    '<div id="pipeline-build-hint">' + buildModeAutoHintHtml(repo) + "</div>" +
    (repo.workflow_synced_at
      ? '<label class="auto-deploy-toggle"><input type="checkbox" id="auto-deploy-toggle" ' +
        (repo.auto_deploy_enabled ? "checked" : "") +
        " /> Tự deploy lên cluster khi build xong</label>"
      : "") +
    "", true) +
    renderPipelineStepCollapsible(slug, 2, "Chốt kiểu chạy",
    '<div id="repo-detect-banner-slot">' + (isMulti ? contractBanner : renderServicesContractBanner(repoContract, canEdit)) + "</div>" +
    '<div class="layout-change-row">' +
    '<p class="muted pipeline-layout-hint">Chọn <em>một lần</em> trước khi sync workflow. Đã deploy mà muốn đổi topology?</p>' +
    '<div class="layout-change-actions">' +
    '<button type="button" class="btn-ghost btn-sm layout-change-btn" id="open-change-layout-wizard">↔ Đổi kiểu chạy</button>' +
    '<span class="muted layout-change-note">Không phải rollback — cần sync workflow rồi deploy bản mới</span>' +
    "</div></div>" +
    '<div class="layout-picker">' +
    '<label class="layout-option">' +
    '<input type="radio" name="layout" value="single"' + (!isMulti ? " checked" : "") + " />" +
    '<div class="layout-option-body"><span class="layout-option-icon" aria-hidden="true">🌐</span><strong>Một website</strong><span>Một link duy nhất</span></div></label>' +
    '<label class="layout-option">' +
    '<input type="radio" name="layout" value="multi"' + (isMulti ? " checked" : "") + " />" +
    '<div class="layout-option-body"><span class="layout-option-icon" aria-hidden="true">⚡</span><strong>Web + API riêng</strong><span>Giao diện + API tách path</span></div></label></div>' +
    singleHint +
    multiPanel +
    "", true) +
    renderPipelineStepCollapsible(slug, 3, "Đồng bộ workflow",
    (repo.workflow_stale
      ? '<div class="banner warn" style="margin-bottom:10px">' + esc(repo.workflow_stale_reason || "Cần đồng bộ workflow trước khi push.") + "</div>"
      : "") +
    '<div id="github-setup-progress" class="setup-progress" hidden></div>' +
    '<div class="form-actions" style="flex-wrap:wrap;gap:8px">' +
    '<button type="submit" class="btn-primary" id="github-setup-submit">Lưu &amp; đồng bộ GitHub</button>' +
    '<button type="button" class="btn-ghost btn-sm" id="pipeline-save-draft">Chỉ lưu Console</button>' +
    '<button type="button" class="btn-ghost btn-sm" id="github-disconnect-btn">Ngắt GitHub</button>' +
    "</div>", true) +
    "</form>",
    statusChips,
    true
  );
}

function selectedGitHubBranch(fallback) {
  const sel = document.getElementById("github-branch-select");
  if (sel && sel.value && String(sel.value).trim()) {
    return String(sel.value).trim();
  }
  return String(fallback || "main").trim() || "main";
}

function bindPipelineSetupForm(main, slug, svcData, repo, ghStatus, env, navToken) {
  const form = document.getElementById("pipeline-setup-form");
  state.pipelineLayoutUserTouched = false;
  state.pipelineServicesUserTouched = false;
  const singleHint = document.getElementById("layout-single-hint");
  const multiPanel = document.getElementById("layout-multi-panel");
  const previewGrid = document.getElementById("service-preview-grid");
  const resGrid = document.getElementById("service-resources-grid");
  const tbody = document.getElementById("project-services-tbody");
  const dirHint = document.getElementById("github-dir-hint");
  const refreshDirsBtn = document.getElementById("refresh-github-dirs");
  const template = defaultMultiTemplate(svcData);
  let nextSvcIdx = Math.max((template.length || 2) - 1, 0);
  repo = repo || {};
  ghStatus = ghStatus || {};
  env = env || state.projectEnv || "dev";
  if (!form) return;

  function readCtxValue(idx) {
    const sel = form.querySelector('[name="svc_ctx_sel_' + idx + '"]');
    const hidden = form.querySelector('[name="svc_ctx_' + idx + '"]');
    const custom = form.querySelector('[name="svc_ctx_custom_' + idx + '"]');
    if (sel && sel.value === "__custom__" && custom) {
      return (custom.value || ".").trim() || ".";
    }
    if (sel && sel.value && sel.value !== "__custom__") {
      return sel.value;
    }
    return hidden ? hidden.value : ".";
  }

  function syncCtxHidden(idx) {
    const val = readCtxValue(idx);
    const hidden = form.querySelector('[name="svc_ctx_' + idx + '"]');
    if (hidden) hidden.value = val;
    const modeEl = form.querySelector('[name="svc_mode_' + idx + '"]');
    const dfEl = form.querySelector('[name="svc_df_' + idx + '"]');
    if (modeEl && modeEl.value === "dockerfile" && dfEl && val && val !== ".") {
      const guess = val.replace(/\/$/, "") + "/Dockerfile";
      if (!dfEl.value || dfEl.value === "Dockerfile" || dfEl.value.endsWith("/Dockerfile")) {
        dfEl.value = guess;
      }
    }
    refreshPreview();
  }

  function bindCtxSelect(idx) {
    const sel = form.querySelector('[name="svc_ctx_sel_' + idx + '"]');
    const custom = form.querySelector('[name="svc_ctx_custom_' + idx + '"]');
    if (!sel) return;
    sel.onchange = function () {
      if (custom) custom.hidden = sel.value !== "__custom__";
      syncCtxHidden(idx);
    };
    if (custom) {
      custom.oninput = function () {
        syncCtxHidden(idx);
      };
    }
  }

  function bindResModeSelect(idx) {
    if (idx === "app") {
      const sel = form.querySelector('[name="app_res_mode"]');
      if (!sel) return;
      sel.onchange = function () {
        toggleServiceResourceInputs(form, "app");
      };
      toggleServiceResourceInputs(form, "app");
      return;
    }
    const sel = form.querySelector('[name="svc_res_mode_' + idx + '"]');
    if (!sel) return;
    sel.onchange = function () {
      toggleServiceResourceInputs(form, idx);
    };
    toggleServiceResourceInputs(form, idx);
  }

  async function loadGitHubDirs() {
    const parsed = parseRepoFromForm(form);
    const owner = parsed ? parsed.owner : (repo.github_owner || "").trim();
    const ghRepo = parsed ? parsed.repo : (repo.github_repo || "").trim();
    const branch = selectedGitHubBranch(repo.branch || "main");
    if (!ghStatus.connected || !owner || !ghRepo) {
      if (dirHint) {
        dirHint.textContent = "Chọn repo và branch ở bước 1 — rồi bấm Quét thư mục.";
      }
      return [];
    }
    if (dirHint) {
      dirHint.innerHTML =
        "Thư mục từ branch <code>" + esc(branch) + "</code> · repo <code>" + esc(owner + "/" + ghRepo) + "</code>";
    }
    const data = await api(
      "/api/v1/github/repos/" + encodeURIComponent(owner) + "/" + encodeURIComponent(ghRepo) + "/contents" +
        qs({ ref: branch, path: "" })
    );
    return (data.items || []).filter(function (e) {
      return e.type === "dir";
    });
  }

  function populateCtxSelects(dirs) {
    form.querySelectorAll(".svc-ctx-select").forEach(function (sel) {
      const idx = sel.getAttribute("data-idx");
      const cur = readCtxValue(idx);
      let html = '<option value="."' + (cur === "." ? " selected" : "") + ">. (root repo)</option>";
      dirs.forEach(function (d) {
        html += '<option value="' + esc(d.path) + '"' + (d.path === cur ? " selected" : "") + ">" + esc(d.path) + "/</option>";
      });
      const known = cur === "." || dirs.some(function (d) { return d.path === cur; });
      html += '<option value="__custom__"' + (!known ? " selected" : "") + ">Tự nhập…</option>";
      sel.innerHTML = html;
      const custom = form.querySelector('[name="svc_ctx_custom_' + idx + '"]');
      if (custom) {
        custom.hidden = known;
        if (!known) custom.value = cur;
      }
      bindCtxSelect(idx);
      syncCtxHidden(idx);
    });
  }

  async function refreshGitHubDirs(opts) {
    opts = opts || {};
    const silent = opts.silent === true;
    if (refreshDirsBtn) {
      refreshDirsBtn.disabled = true;
      refreshDirsBtn.textContent = "Đang quét…";
    }
    try {
      const dirs = await loadGitHubDirs();
      populateCtxSelects(dirs);
      if (!silent && dirs.length === 0 && ghStatus.connected) {
        toastError("Không thấy thư mục con — kiểm tra branch hoặc dùng Tự nhập");
      }
    } catch (err) {
      if (!silent) {
        toastError(err.message || "Không quét được GitHub");
      }
    } finally {
      if (refreshDirsBtn) {
        refreshDirsBtn.disabled = false;
        refreshDirsBtn.textContent = "Quét thư mục từ GitHub";
      }
    }
  }

  function currentLayout() {
    const checked = form.querySelector('input[name="layout"]:checked');
    return checked ? checked.value : "single";
  }

  function serviceFromRow(idx, defaults) {
    defaults = defaults || {};
    const modeEl = form.querySelector('[name="svc_mode_' + idx + '"]');
    const pubEl = form.querySelector('[name="svc_public_' + idx + '"]');
    const expose = pubEl ? pubEl.checked : defaults.expose_ingress !== false;
    const ingressEl = form.querySelector('[name="svc_ingress_' + idx + '"]');
    let ingress = ingressEl ? ingressEl.value : defaults.ingress_path || "/";
    if (!expose) ingress = "-";
    const stackEl = form.querySelector('[name="svc_stack_' + idx + '"]');
    const row = {
      name: (form.querySelector('[name="svc_name_' + idx + '"]') || {}).value || defaults.name || "",
      build_mode: modeEl ? modeEl.value : defaults.build_mode || "dockerfile",
      stack: stackEl ? stackEl.value : defaults.stack || "",
      build_context: readCtxValue(idx) || defaults.build_context || ".",
      dockerfile_path: (form.querySelector('[name="svc_df_' + idx + '"]') || {}).value || defaults.dockerfile_path || "Dockerfile",
      ingress_path: ingress,
      expose_ingress: expose,
      container_port: defaults.container_port || 8080,
      health_path: defaults.health_path || "/health",
      sort_order: parseInt(idx, 10) || 0,
      display_name: defaults.display_name || "",
    };
    return Object.assign(row, readServiceResourcesFromForm(form, idx));
  }

  function listServiceRows() {
    if (!tbody) return [];
    return Array.prototype.slice.call(tbody.querySelectorAll("tr"));
  }

  function servicesFromForm() {
    return listServiceRows().map(function (tr) {
      const idx = tr.getAttribute("data-svc-idx");
      return serviceFromRow(idx, {});
    });
  }

  function refreshPreview() {
    if (currentLayout() !== "multi") return;
    const rows = listServiceRows();
    const list = rows.map(function (tr) {
      return serviceFromRow(tr.getAttribute("data-svc-idx"), {});
    });
    if (previewGrid) {
      previewGrid.innerHTML = list
        .map(function (s) {
          const pub = s.expose_ingress !== false && s.ingress_path !== "-";
          return (
            '<div class="service-preview-card"><h4>' +
            esc(s.display_name || s.name) +
            ' <span class="badge">' +
            esc(buildModeLabel(s.build_mode)) +
            "</span></h4><p>Thư mục <code>" +
            esc(s.build_context) +
            "</code> → image <code>" +
            esc(s.name) +
            "</code><br>" +
            (pub
              ? "URL: <code>" + esc(s.ingress_path) + "*</code>"
              : '<span class="badge warn">Internal</span>') +
            "</p></div>"
          );
        })
        .join("");
    }
    if (resGrid) {
      resGrid.innerHTML = rows
        .map(function (tr, i) {
          const idx = tr.getAttribute("data-svc-idx");
          return renderServiceResourcesCard(list[i], idx);
        })
        .join("");
      rows.forEach(function (tr) {
        bindResModeSelect(tr.getAttribute("data-svc-idx"));
      });
    }
  }

  function appendServiceRow(s) {
    if (!tbody) return;
    nextSvcIdx += 1;
    const idx = String(nextSvcIdx);
    tbody.insertAdjacentHTML("beforeend", buildServiceTableRowHtml(s || { name: "worker", build_context: ".", expose_ingress: false, ingress_path: "-" }, idx));
    bindCtxSelect(idx);
    bindResModeSelect(idx);
    syncCtxHidden(idx);
    bindRemoveRow(idx);
    refreshPreview();
  }

  function bindRemoveRow(idx) {
    const btn = form.querySelector('.svc-remove-row[data-idx="' + idx + '"]');
    if (!btn) return;
    btn.onclick = function () {
      const rows = listServiceRows();
      if (rows.length <= 2) {
        toastError("Multi-service cần ít nhất 2 service");
        return;
      }
      const tr = btn.closest("tr");
      if (tr) tr.remove();
      refreshPreview();
    };
  }

  function rebindServiceRows() {
    listServiceRows().forEach(function (tr) {
      bindRemoveRow(tr.getAttribute("data-svc-idx"));
      bindCtxSelect(tr.getAttribute("data-svc-idx"));
      bindResModeSelect(tr.getAttribute("data-svc-idx"));
    });
    bindResModeSelect("app");
    refreshPreview();
    if (currentLayout() === "multi") {
      refreshGitHubDirs({ silent: true });
    }
  }

  function scheduleCrosscheck() {
    refreshPipelineCrosscheck(slug, form, svcData, repo, { onPrefilled: rebindServiceRows });
  }

  function togglePanels() {
    const multi = currentLayout() === "multi";
    if (singleHint) singleHint.hidden = multi;
    if (multiPanel) multiPanel.hidden = !multi;
    if (multi && tbody && !tbody.querySelector("tr")) {
      tbody.innerHTML = template
        .map(function (s, idx) {
          return buildServiceTableRowHtml(s, idx);
        })
        .join("");
    }
    if (multi) {
      template.forEach(function (_s, idx) {
        bindCtxSelect(idx);
        bindResModeSelect(idx);
      });
      refreshGitHubDirs({ silent: true });
    }
    bindResModeSelect("app");
    refreshPreview();
    scheduleCrosscheck();
  }

  form.querySelectorAll('input[name="layout"]').forEach(function (el) {
    el.onchange = function () {
      state.pipelineLayoutUserTouched = true;
      togglePanels();
    };
  });
  if (refreshDirsBtn) {
    refreshDirsBtn.onclick = function () {
      refreshGitHubDirs({ silent: false });
    };
  }
  if (tbody) {
    tbody.addEventListener("change", function () {
      state.pipelineServicesUserTouched = true;
      refreshPreview();
    });
    tbody.addEventListener("input", function () {
      state.pipelineServicesUserTouched = true;
      refreshPreview();
    });
    listServiceRows().forEach(function (tr) {
      bindRemoveRow(tr.getAttribute("data-svc-idx"));
    });
  }
  const addRowBtn = document.getElementById("project-services-add-row");
  if (addRowBtn) {
    addRowBtn.onclick = function () {
      appendServiceRow({ name: "worker", build_context: ".", expose_ingress: false, ingress_path: "-" });
    };
  }
  template.forEach(function (_s, idx) {
    bindCtxSelect(String(idx));
    bindRemoveRow(String(idx));
  });
  rebindServiceRows();
  if (currentLayout() === "multi") {
    refreshGitHubDirs({ silent: true });
  }
  scheduleCrosscheck();

  const repoSel = form.querySelector('[name="repo"]');
  const branchSel = form.querySelector('[name="branch"]');
  if (repoSel) {
    repoSel.onchange = function () {
      const parsed = parseGitHubRepoValue(repoSel.value);
      if (branchSel) {
        if (!parsed) {
          branchSel.innerHTML = '<option value="main" selected>main</option>';
        } else {
          const opt = repoSel.options[repoSel.selectedIndex];
          const defBranch = (opt && opt.dataset.branch) || "main";
          loadGitHubBranchSelect(branchSel, parsed.owner, parsed.repo, defBranch).then(scheduleCrosscheck);
          return;
        }
      }
      scheduleCrosscheck();
    };
  }
  if (branchSel) {
    branchSel.onchange = scheduleCrosscheck;
  }
  if (tbody) {
    tbody.addEventListener("change", scheduleCrosscheck);
  }

  bindApplyConventionsButton(main, slug, function () {
    pageProjectHub(main, slug, "deploy");
  });

  const syncContractBtn = document.getElementById("sync-services-contract");
  state._repoDetectSyncHandler = async function () {
    const btn = document.getElementById("sync-services-contract");
    if (!btn) return;
    btn.disabled = true;
    try {
      const parsed = parseRepoFromForm(form);
      if (ghStatus.connected && parsed) {
        const fd = new FormData(form);
        await runGitHubPipelineSetup(slug, {
          owner: parsed.owner,
          repo: parsed.repo,
          branch: fd.get("branch") || selectedGitHubBranch(repo.branch || "main"),
          environment: (fd.get("environment") || "dev").toString(),
          apply_repo_contract: true,
          progressEl: document.getElementById("github-setup-progress"),
          submitBtn: document.getElementById("github-setup-submit"),
          formRoot: form,
        });
        toastSuccess("Đã áp dụng services.yaml + đồng bộ workflow");
        pageProjectHub(main, slug, "deploy");
      } else {
        await api("/api/v1/projects/" + encodeURIComponent(slug) + "/services/sync-from-repo", {
          method: "POST",
          body: { branch: selectedGitHubBranch(repo.branch || "main") },
        });
        toastSuccess("Đã áp dụng services.yaml — chọn repo và bấm Lưu & đồng bộ GitHub");
        pageProjectHub(main, slug, "deploy");
      }
    } catch (err) {
      toastError(errorMessage(err));
    } finally {
      btn.disabled = false;
    }
  };
  if (syncContractBtn) {
    syncContractBtn.onclick = state._repoDetectSyncHandler;
  }

  const changeLayoutBtn = document.getElementById("open-change-layout-wizard");
  if (changeLayoutBtn) {
    changeLayoutBtn.onclick = async function () {
      const current = currentLayout();
      const targetLabel = current === "multi" ? "Một website" : "Web + API riêng";
      const ok = await uiConfirm({
        title: "Đổi kiểu chạy",
        message: "Đổi topology trên cluster — không phải rollback về bản cũ.",
        details: [
          "Hiện tại: " + layoutKindLabel(current),
          "Sẽ chuyển sang: " + targetLabel,
          "Bước tiếp: Lưu & đồng bộ GitHub → deploy bản mới",
          "Deploy lại trong lịch sử chỉ hoạt động cùng kiểu chạy",
        ],
        confirmText: "Chuyển sang " + targetLabel,
      });
      if (!ok) return;
      const newLayout = current === "multi" ? "single" : "multi";
      const radio = form.querySelector('input[name="layout"][value="' + newLayout + '"]');
      if (radio) {
        radio.checked = true;
        state.pipelineLayoutUserTouched = true;
        togglePanels();
      }
      let payload = collectProjectLayoutPayload(form);
      if (newLayout === "multi" && (!payload.services || payload.services.length < 2)) {
        payload = { layout: "multi", services: defaultMultiTemplate(svcData) };
      } else if (newLayout === "single") {
        const cur = collectProjectLayoutPayload(form);
        payload = { layout: "single", services: cur.services && cur.services.length ? cur.services : [{ name: "app", resources_mode: "platform" }] };
      }
      try {
        changeLayoutBtn.disabled = true;
        await api("/api/v1/projects/" + encodeURIComponent(slug) + "/services", {
          method: "PUT",
          body: Object.assign({ branch: selectedGitHubBranch(repo.branch || "main") }, payload),
        });
        toastSuccess("Đã đổi kiểu — bắt buộc bấm 「Lưu & đồng bộ GitHub」 rồi push/build lại (workflow cũ vẫn build kiểu cũ)");
        pageProjectHub(main, slug, "deploy");
      } catch (err) {
        toastError(errorMessage(err));
      } finally {
        changeLayoutBtn.disabled = false;
      }
    };
  }

  const draftBtn = document.getElementById("pipeline-save-draft");
  if (draftBtn) {
    draftBtn.onclick = async function () {
      const resErr = validateAllServiceResources(form);
      if (resErr) {
        toastError(resErr);
        return;
      }
      const layoutPayload = collectProjectLayoutPayload(form);
      try {
        const res = await api("/api/v1/projects/" + encodeURIComponent(slug) + "/services", {
          method: "PUT",
          body: Object.assign({ branch: selectedGitHubBranch(repo.branch || "main") }, layoutPayload),
        });
        if (res.convention_seeds && res.convention_seeds.length) {
          toastSuccess("Đã lưu nháp — đã gợi ý env · bấm Lưu & đồng bộ GitHub");
        } else {
          toastSuccess(res.hint || "Đã lưu Console — bấm Lưu & đồng bộ GitHub để push workflow");
        }
        scheduleCrosscheck();
      } catch (err) {
        toastError(errorMessage(err));
      }
    };
  }

  async function saveProjectResources(apply) {
    const resErr = validateAllServiceResources(form);
    if (resErr) {
      toastError(resErr);
      return;
    }
    const layoutPayload = collectProjectLayoutPayload(form);
    const applyBtn = document.getElementById("resources-save-apply");
    const draftResBtn = document.getElementById("resources-save-draft");
    try {
      if (applyBtn) applyBtn.disabled = true;
      if (draftResBtn) draftResBtn.disabled = true;
      await api("/api/v1/projects/" + encodeURIComponent(slug) + "/services", {
        method: "PUT",
        body: Object.assign({ branch: selectedGitHubBranch(repo.branch || "main") }, layoutPayload),
      });
      if (!apply) {
        toastSuccess("Đã lưu nháp CPU/RAM");
        scheduleCrosscheck();
        return;
      }
      const envLabel = env === "prod" ? "prod" : "dev";
      const ok = await uiConfirm({
        title: "Áp dụng cấu hình CPU/RAM",
        message: "Pod sẽ restart để nhận limits mới trên " + envLabel + ".",
        details: [
          "Không rebuild image — chỉ cập nhật manifest Kubernetes.",
          "Theo dõi rollout ở lịch sử deploy bên dưới.",
        ],
        confirmText: "Lưu & áp dụng",
      });
      if (!ok) {
        toastSuccess("Đã lưu — chưa áp dụng lên cluster");
        scheduleCrosscheck();
        return;
      }
      const result = await api("/api/v1/projects/" + encodeURIComponent(slug) + "/resources/apply", {
        method: "POST",
        body: { environment: env },
      });
      toastSuccess(result.message || "Đang áp dụng cấu hình CPU/RAM mới…");
      const activity = await api(
        "/api/v1/projects/" + encodeURIComponent(slug) + "/deploy/activity" + projectQs({ environment: env })
      ).catch(function () { return { items: [] }; });
      let readiness = null;
      if (env === "dev" && canManagePlatformProjects()) {
        readiness = await api(
          "/api/v1/projects/" + encodeURIComponent(slug) + "/deploy/promote-readiness"
        ).catch(function () { return null; });
      }
      updateDeployActivityDOM(activity, slug, readiness, env);
      bindDeployActivityPoll(slug, env, navToken);
      const actCard = document.getElementById("deploy-activity-card");
      if (actCard) actCard.scrollIntoView({ behavior: "smooth", block: "start" });
      scheduleCrosscheck();
    } catch (err) {
      toastError(errorMessage(err));
    } finally {
      if (applyBtn) applyBtn.disabled = false;
      if (draftResBtn) draftResBtn.disabled = false;
    }
  }

  const resApplyBtn = document.getElementById("resources-save-apply");
  if (resApplyBtn) {
    resApplyBtn.onclick = function () {
      saveProjectResources(true);
    };
  }
  const resDraftBtn = document.getElementById("resources-save-draft");
  if (resDraftBtn) {
    resDraftBtn.onclick = function () {
      saveProjectResources(false);
    };
  }

  form.onsubmit = async function (e) {
    e.preventDefault();
    const resErr = validateAllServiceResources(form);
    if (resErr) {
      toastError(resErr);
      return;
    }
    const fd = new FormData(form);
    const full = (fd.get("repo") || "").toString();
    const parts = full.split("/");
    if (parts.length < 2) {
      toastError("Chọn repository");
      return;
    }
    const submitBtn = document.getElementById("github-setup-submit");
    const progress = document.getElementById("github-setup-progress");
    try {
      await runGitHubPipelineSetup(slug, {
        owner: parts[0],
        repo: parts[1],
        branch: fd.get("branch"),
        environment: fd.get("environment"),
        layoutPayload: collectProjectLayoutPayload(form),
        progressEl: progress,
        submitBtn: submitBtn,
        formRoot: form,
      });
      toastSuccess("Pipeline sẵn sàng — theo dõi build bên dưới");
      const activity = await api(
        "/api/v1/projects/" + encodeURIComponent(slug) + "/deploy/activity" + projectQs({ environment: env })
      ).catch(function () { return { items: [] }; });
      let readiness = null;
      if (env === "dev" && canManagePlatformProjects()) {
        readiness = await api(
          "/api/v1/projects/" + encodeURIComponent(slug) + "/deploy/promote-readiness"
        ).catch(function () { return null; });
      }
      updateDeployActivityDOM(activity, slug, readiness, env);
      bindDeployActivityPoll(slug, env, navToken);
      const actCard = document.getElementById("deploy-activity-card");
      if (actCard) actCard.scrollIntoView({ behavior: "smooth", block: "start" });
      scheduleCrosscheck();
    } catch (err) {
      toastError(errorMessage(err, "Đồng bộ pipeline thất bại"));
    }
  };
}

/* --- pipeline extras --- */
