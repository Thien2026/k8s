/* Project hub — Env / cấu hình app tab (extracted from app.js). */

async function loadProjectEnv(main, slug, p, env) {

    const envRes = await api(
      "/api/v1/projects/" + encodeURIComponent(slug) + "/env" + qs({ environment: env })
    ).catch(function () { return { items: [] }; });
    const convP = api("/api/v1/projects/" + encodeURIComponent(slug) + "/conventions").catch(function () {
      return { enabled: false };
    });
    const buildReadyP =
      env === "prod"
        ? Promise.resolve(null)
        : api(
            "/api/v1/projects/" + encodeURIComponent(slug) + "/env/readiness" + qs({ environment: env, scope: "build" })
          ).catch(function () { return null; });
    const runtimeReadyP = api(
      "/api/v1/projects/" + encodeURIComponent(slug) + "/env/readiness" + qs({ environment: env, scope: "runtime" })
    ).catch(function () { return null; });
    const envSyncP = api(
      "/api/v1/projects/" + encodeURIComponent(slug) + "/env/sync-status" + qs({ environment: env })
    ).catch(function () { return null; });
    const buildReady = await buildReadyP;
    const runtimeReady = await runtimeReadyP;
    const envSyncStatus = await envSyncP;
    const conventions = await convP;
    const envItems = envRes.items || [];
    const runtimeItems = envItems.filter(envVarIsRuntimeScope);
    const buildItems = envItems.filter(envVarIsBuildScope);
    const buildSuggestions = (buildReady && buildReady.suggestions) || [];
    const runtimeSuggestions = (runtimeReady && runtimeReady.suggestions) || [];
    const canEditEnv = canWriteK8s() && (env !== "prod" || state.user.role === "admin" || state.user.role === "tech_lead");
    const runtimeRows = renderEnvVarTable(runtimeItems, slug, env, canEditEnv, "runtime");
    const buildRows = renderEnvVarTable(buildItems, slug, env, canEditEnv, "build");
    const runtimeTableBody =
      renderMissingContractRows(runtimeSuggestions, "runtime") +
      (runtimeRows ||
        (runtimeSuggestions.length
          ? ""
          : '<tr><td colspan="3" class="muted">Chưa có — ví dụ <code>APP_GREETING</code></td></tr>'));
    const buildTableBody =
      renderMissingContractRows(buildSuggestions, "build") +
      (buildRows ||
        (buildSuggestions.length
          ? ""
          : '<tr><td colspan="3" class="muted">Chưa có biến trên Console — bấm <strong>+ Thêm</strong> hoặc <strong>Lấy key từ contract</strong></td></tr>')) +
      renderPlatformBuildArgRows();
    const runtimeHeadBtns =
      (canEditEnv && runtimeSuggestions.length
        ? '<button type="button" class="btn-ghost btn-sm" id="env-contract-runtime-keys">Lấy key từ contract</button> '
        : "") +
      (canEditEnv ? '<button type="button" class="btn-primary btn-sm" id="open-add-runtime-env">+ Thêm</button>' : "");
    const buildHeadBtns =
      env === "prod"
        ? '<a class="btn-primary btn-sm" href="#/project/' + esc(slug) + '/env" id="env-go-dev-build">Cấu hình build tại Dev →</a>'
        : (canEditEnv && buildSuggestions.length
            ? '<button type="button" class="btn-ghost btn-sm" id="env-contract-build-keys">Lấy key từ contract</button> '
            : "") +
          (canEditEnv ? '<button type="button" class="btn-primary btn-sm" id="open-add-build-env">+ Thêm</button>' : "");
    const buildCardHtml =
      env === "prod"
        ? '<div class="card env-vars-card" style="margin-top:16px">' +
          '<div class="env-vars-head"><div><h3>Khi build image</h3>' +
          '<p class="muted">Biến build (<code>ARG</code> Dockerfile) chỉ cấu hình trên <strong>Dev</strong>. Promote prod tái sử dụng <strong>cùng image</strong> đã build từ dev — không cần khai báo lại trên prod.</p></div>' +
          '<div class="env-vars-head-actions">' + buildHeadBtns + "</div></div></div>"
        : '<div class="card env-vars-card" style="margin-top:16px">' +
          renderEnvReadinessPanel(buildReady, slug, env, "build") +
          '<div class="env-vars-head"><div><h3>Khi build image</h3>' +
          '<p class="muted">Truyền vào Dockerfile (<code>ARG</code>) lúc GitHub Actions build. Đổi phải <strong>push lại</strong> để image mới. Promote prod dùng cùng image.</p></div>' +
          '<div class="env-vars-head-actions">' + buildHeadBtns + "</div>" +
          "</div>" +
          (canEditEnv
            ? '<div class="toolbar" style="margin:12px 0"><button type="button" class="btn-ghost btn-sm" id="env-sync-workflow-btn">Đồng bộ workflow GitHub</button></div>'
            : "") +
          '<div class="table-wrap"><table><thead><tr><th>Key</th><th>Value</th><th></th></tr></thead><tbody>' +
          buildTableBody +
          "</tbody></table></div>" +
          '<p class="muted env-hint">Biến <code>platform</code> do hệ thống inject — không cần thêm trên Console. Contract bắt buộc: <code>.platform/build.yaml</code></p></div>';
    main.innerHTML =
      projectHeader(p, "Cấu hình app") +
      projectEnvToolbar(slug, p, function () { pageProjectHub(main, slug, "env"); }) +
      (conventions.enabled ? renderBackFrontConventionBanner(conventions, canEditEnv) : "") +
      '<div class="card env-vars-card">' +
      renderEnvReadinessPanel(runtimeReady, slug, env, "runtime") +
      '<div class="env-vars-head"><div><h3>Khi app chạy (Pod)</h3>' +
      '<p class="muted">Thay file <code>.env</code> — inject Secret <code>app-env</code>, đổi là restart pod (không build lại).</p></div>' +
      '<div class="env-vars-head-actions">' + runtimeHeadBtns + "</div>" +
      "</div>" +
      (canEditEnv
        ? '<div class="toolbar" style="margin:12px 0"><button type="button" class="btn-ghost btn-sm" id="env-sync-btn">Đồng bộ cluster &amp; restart pod</button></div>'
        : "") +
      renderEnvSyncNote(envSyncStatus) +
      '<div class="table-wrap"><table><thead><tr><th>Key</th><th>Value</th><th></th></tr></thead><tbody>' +
      runtimeTableBody +
      "</tbody></table></div></div>" +
      buildCardHtml +
      '<p class="muted env-hint" style="margin-top:8px">Runtime bắt buộc: <code>.platform/runtime.yaml</code></p>';

    const addRuntime = document.getElementById("open-add-runtime-env");
    if (addRuntime) addRuntime.onclick = function () { openEnvVarDialog(slug, env, { scope: "runtime" }); };
    const addBuild = document.getElementById("open-add-build-env");
    if (addBuild) addBuild.onclick = function () { openEnvVarDialog(slug, env, { scope: "build" }); };
    const goDevBuild = document.getElementById("env-go-dev-build");
    if (goDevBuild) {
      goDevBuild.onclick = function () {
        state.projectEnv = "dev";
        localStorage.setItem("project-env", "dev");
      };
    }
    const contractBuildBtn = document.getElementById("env-contract-build-keys");
    if (contractBuildBtn) {
      contractBuildBtn.onclick = function () { promptContractKeys(slug, env, buildSuggestions, "build"); };
    }
    const contractRuntimeBtn = document.getElementById("env-contract-runtime-keys");
    if (contractRuntimeBtn) {
      contractRuntimeBtn.onclick = function () { promptContractKeys(slug, env, runtimeSuggestions, "runtime"); };
    }
    const syncBtn = document.getElementById("env-sync-btn");
    if (syncBtn) {
      syncBtn.onclick = async function () {
        try {
          await api("/api/v1/projects/" + encodeURIComponent(slug) + "/env/sync" + qs({ environment: env }), { method: "POST" });
          toastSuccess("Đã đồng bộ lên cluster");
        } catch (err) {
          toastError(err.message);
        }
      };
    }
    const syncWf = document.getElementById("env-sync-workflow-btn");
    if (syncWf) {
      syncWf.onclick = async function () {
        try {
          await api("/api/v1/projects/" + encodeURIComponent(slug) + "/env/sync-workflow", { method: "POST" });
          toastSuccess("Đã cập nhật workflow + build-args trên GitHub");
        } catch (err) {
          toastError(err.message);
        }
      };
    }
    bindEnvVarTableActions(main, slug, env);
    bindEnvSuggestButtons(main, slug, env);
    bindApplyConventionsButton(main, slug, function () {
      pageProjectHub(main, slug, "env");
    });
}
