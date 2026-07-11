/* Project hub — Deploy / Git tab (extracted from app.js). */

async function loadProjectDeploy(main, slug, p, data, env, ns) {
    try {
    const navToken = state.navToken;
    const repo = data.repo || {};
    const reg = p.registry || {};
    const env = state.projectEnv || "dev";

    const hashQ = (location.hash.split("?")[1] || "");
    if (hashQ.indexOf("github=connected") >= 0) {
      toastSuccess("Đã kết nối GitHub");
      location.hash = "#/project/" + slug + "/deploy";
    }

    const ghStatusP = api("/api/v1/github/status").catch(function () {
      return { enabled: false, connected: false };
    });
    const planP = api(
      "/api/v1/projects/" + encodeURIComponent(slug) + "/deploy/plan" + projectQs({ environment: env })
    ).catch(function (err) {
      return { error: err.message };
    });
    const svcP = api("/api/v1/projects/" + encodeURIComponent(slug) + "/services").catch(function () {
      return { layout: "single", items: [] };
    });
    const gitopsPubP = api("/api/v1/gitops/public").catch(function () { return {}; });
    const gitopsStatusP = api("/api/v1/projects/" + encodeURIComponent(slug) + "/gitops/status").catch(function () { return {}; });
    const [ghStatus, plan, svcData, gitopsPub, gitopsStatus] = await Promise.all([ghStatusP, planP, svcP, gitopsPubP, gitopsStatusP]);

    const stepsHtml = (plan.steps || [])
      .map(function (s) {
        return "<li>" + esc(s) + "</li>";
      })
      .join("");
    const secretsHtml = (plan.workflow && plan.workflow.secrets_hint || [])
      .map(function (s) {
        return "<li><code>" + esc(s) + "</code></li>";
      })
      .join("");

    const ghReposPlaceholder = ghStatus.connected ? { items: null } : { items: [] };

    main.innerHTML =
      projectHeader(p, "Deploy / Git", { help: "deploy" }) +
      projectEnvToolbar(slug, p, function () { pageProjectHub(main, slug, "deploy"); }) +
      renderGitOpsProjectCard(slug, gitopsPub, gitopsStatus, canWriteK8s()) +
      renderPipelineSetupCard(slug, svcData, repo, ghStatus, ghReposPlaceholder, canWriteK8s()) +
      renderDeployCollapsibleCard(
        slug,
        "summary",
        '<div class="deploy-collapsible-summary-inner">' +
          '<span class="deploy-collapsible-chev" aria-hidden="true"></span>' +
          '<div class="deploy-collapsible-title-row"><h3 style="margin:0">Tóm tắt</h3>' +
          (reg.ready || plan.registry_ready
            ? '<span class="badge ok">Registry OK</span>'
            : '<span class="badge warn">Registry?</span>') +
          "</div></div>",
        '<div class="meta-chips">' +
          chip(reg.label || p.registry_provider || "GHCR", reg.provider || p.registry_provider) +
          chip("Môi trường", env) +
          chip("Namespace", plan.namespace || ns) +
          (reg.ready || plan.registry_ready
            ? '<span class="badge ok">Registry OK</span>'
            : '<span class="badge warn">Registry chưa sẵn sàng</span>') +
          "</div>" +
          (plan.image ? '<p class="muted" style="margin-top:8px">Image: <code>' + esc(plan.image) + "</code></p>" : "") +
          (repo.auto_deploy_enabled
            ? '<p class="muted" style="margin-top:6px">Auto-deploy · branch <code>' + esc(repo.branch || "main") + "</code></p>"
            : ""),
        false
      ) +
      renderDeployActivityCard({ loading: true }, { slug: slug, expectedEnv: env }) +
      renderDeployCollapsibleCard(
        slug,
        "manual",
        '<div class="deploy-collapsible-summary-inner">' +
          '<span class="deploy-collapsible-chev" aria-hidden="true"></span>' +
          '<div class="deploy-collapsible-title-row"><h3 style="margin:0">Deploy thủ công</h3>' +
          '<span class="badge muted">fallback</span></div></div>',
        '<p class="muted">Dùng khi cần hotfix nhanh hoặc muốn deploy lại tag cụ thể.</p>' +
          '<form id="deploy-apply-form" class="login-form" style="max-width:420px">' +
          '<label>Image tag<input name="image_tag" value="latest" placeholder="latest hoặc git sha" /></label>' +
          (plan.can_apply && canWriteK8s()
            ? '<button type="submit" class="btn-primary" style="margin-top:12px">Deploy ngay</button>'
            : '<p class="muted" style="margin-top:12px">' +
              (plan.rancher_ready
                ? "Bạn không có quyền deploy."
                : "Rancher chưa sẵn sàng — bật addon Rancher và cài cluster trước.") +
              "</p>") +
          "</form>",
        false
      ) +
      (plan.error
        ? '<div class="card"><p class="error-text">' + esc(plan.error) + "</p></div>"
        : renderDeployCollapsibleCard(
            slug,
            "advanced",
            '<div class="deploy-collapsible-summary-inner">' +
              '<span class="deploy-collapsible-chev" aria-hidden="true"></span>' +
              '<div class="deploy-collapsible-title-row"><h3 style="margin:0">Nâng cao</h3>' +
              '<span class="badge muted">Git · workflow · manifest</span></div></div>',
            '<form id="project-repo-form" class="login-form" style="max-width:560px">' +
              '<label>Git URL<input name="git_url" type="url" value="' + esc(repo.git_url || "") + '" placeholder="https://github.com/org/repo" /></label>' +
              '<div class="form-row"><label>Branch<input name="branch" value="' + esc(repo.branch || "main") + '" /></label></div>' +
              '<label>Dockerfile (để quét)<input name="dockerfile_path" value="' +
              esc(repo.dockerfile_path || "Dockerfile") +
              '" placeholder="Dockerfile" /><span class="muted" style="font-size:12px;display:block;margin-top:4px">Platform ưu tiên file này, rồi <code>Dockerfile</code>, <code>docker/Dockerfile</code>.</span></label>' +
              '<label>Build context<input name="build_context" value="' + esc(repo.build_context || ".") + '" /></label>' +
              (canWriteK8s()
                ? '<button type="submit" class="btn-primary">Lưu cấu hình</button>'
                : '<p class="muted">Read-only — không chỉnh sửa được.</p>') +
              "</form>" +
              '<ol class="deploy-steps">' + (stepsHtml || "<li>Chưa có bước</li>") + "</ol>" +
              (secretsHtml ? '<p class="muted">Secrets GitHub Actions:</p><ul>' + secretsHtml + "</ul>" : "") +
              (plan.workflow && plan.workflow.content
                ? snippetBlock(
                    "deploy-wf-" + slug,
                    "GitHub Actions — " + (plan.workflow.filename || "workflow.yml"),
                    plan.workflow.content,
                    "Copy workflow"
                  )
                : "") +
              (plan.manifest && plan.manifest.yaml
                ? snippetBlock(
                    "deploy-manifest-" + slug,
                    "Kubernetes — " + (plan.manifest.filename || "manifest.yaml"),
                    plan.manifest.yaml,
                    "Copy manifest"
                  )
                : "") +
              ((plan.manifests || []).length > 1
                ? plan.manifests
                    .slice(1)
                    .map(function (m, i) {
                      return m && m.yaml
                        ? snippetBlock(
                            "deploy-manifest-" + slug + "-" + i,
                            "Kubernetes — " + (m.filename || "manifest-" + i),
                            m.yaml,
                            "Copy manifest"
                          )
                        : "";
                    })
                    .join("")
                : ""),
            false
          ));

    bindSnippetCopyButtons(main);
    bindPipelineSetupForm(main, slug, svcData, repo, ghStatus, env, navToken);
    bindGitOpsProjectCard(main, slug);
    bindDeployCollapsibleCards(main, slug);
    bindDeployHelpTriggers(main);

    Promise.all([
      ghStatus.connected
        ? api("/api/v1/github/repos").catch(function () { return { items: [] }; })
        : Promise.resolve({ items: [] }),
      api(
        "/api/v1/projects/" + encodeURIComponent(slug) + "/deploy/activity" + projectQs({ environment: env, scope: "current" })
      ).catch(function () {
        return { items: [] };
      }),
    ]).then(function (results) {
      if (!isNavTokenActive(navToken)) return;
      const ghRepos = results[0];
      const activity = results[1];
      const repoSel = document.getElementById("github-repo-select");
      const branchSel = document.getElementById("github-branch-select");
      if (repoSel) {
        repoSel.innerHTML =
          '<option value="">— chọn repo —</option>' + githubRepoOptionsHtml(repo, ghRepos);
      }
      const linked =
        repo.github_owner && repo.github_repo
          ? { owner: repo.github_owner, repo: repo.github_repo }
          : parseGitHubRepoValue(repoSel && repoSel.value);
      if (branchSel && linked) {
        loadGitHubBranchSelect(branchSel, linked.owner, linked.repo, repo.branch || "main").then(function () {
          const pipelineForm = document.getElementById("pipeline-setup-form");
          if (pipelineForm) refreshPipelineCrosscheck(slug, pipelineForm, svcData, repo);
        });
      }
      updateDeployActivityDOM(activity, slug, undefined, env, {
        showHistory: false,
        showPromotePrep: false,
        showPromoteBar: false,
      });
      if (promoteFollowActive(slug, env)) {
        scrollToDeployProgress();
        handlePromoteFollowTerminal(activity, slug);
      }
      bindDeployActivityPoll(slug, env, navToken);
    });

    const ghConnect = document.getElementById("github-connect-btn");
    if (ghConnect) {
      ghConnect.onclick = function () {
        const oauthURL =
          "/api/v1/github/oauth/start?popup=1&return=" +
          encodeURIComponent("#/project/" + slug + "/deploy");
        const pop = window.open(
          oauthURL,
          "github-oauth",
          "width=560,height=740,menubar=no,toolbar=no,location=yes,status=no"
        );
        if (!pop) {
          toastError("Trình duyệt chặn popup — cho phép popup rồi thử lại");
          return;
        }
        const onMsg = function (ev) {
          if (ev.origin !== window.location.origin) return;
          const data = ev.data || {};
          if (data.type !== "github_oauth") return;
          window.removeEventListener("message", onMsg);
          if (data.status === "connected") {
            toastSuccess("Đã kết nối GitHub");
          } else if (data.status === "login_required") {
            toastError("Phiên đăng nhập hết hạn — đăng nhập lại");
          } else {
            toastError("Kết nối GitHub thất bại");
          }
          pageProjectHub(main, slug, "deploy");
        };
        window.addEventListener("message", onMsg);
      };
    }
    const ghRepoSel = document.getElementById("github-repo-select");
    const ghBranchSel = document.getElementById("github-branch-select");
    if (ghRepoSel && ghBranchSel && !document.getElementById("pipeline-setup-form")) {
      ghRepoSel.onchange = function () {
        const parsed = parseGitHubRepoValue(ghRepoSel.value);
        if (!parsed) {
          ghBranchSel.innerHTML = '<option value="main" selected>main</option>';
          return;
        }
        const opt = ghRepoSel.options[ghRepoSel.selectedIndex];
        const defBranch = (opt && opt.dataset.branch) || "main";
        loadGitHubBranchSelect(ghBranchSel, parsed.owner, parsed.repo, defBranch);
      };
      if (!ghBranchSel.options.length || ghBranchSel.options[0].textContent.indexOf("Đang tải") >= 0) {
        const parsed =
          parseGitHubRepoValue(ghRepoSel.value) ||
          (repo.github_owner && repo.github_repo
            ? { owner: repo.github_owner, repo: repo.github_repo }
            : null);
        if (parsed) {
          loadGitHubBranchSelect(ghBranchSel, parsed.owner, parsed.repo, repo.branch || "main");
        }
      }
    }
    const ghDisc = document.getElementById("github-disconnect-btn");
    if (ghDisc) {
      ghDisc.onclick = async function () {
        if (!(await uiConfirm("Ngắt kết nối GitHub?", { title: "GitHub" }))) return;
        await api("/api/v1/github/disconnect", { method: "DELETE" });
        pageProjectHub(main, slug, "deploy");
      };
    }
    const autoToggle = document.getElementById("auto-deploy-toggle");
    if (autoToggle) {
      autoToggle.onchange = async function () {
        const enabled = autoToggle.checked;
        autoToggle.disabled = true;
        try {
          await api("/api/v1/projects/" + encodeURIComponent(slug) + "/repo/auto-deploy", {
            method: "PATCH",
            body: { enabled: enabled },
          });
          const badge = document.getElementById("auto-deploy-badge");
          if (badge) {
            badge.textContent = enabled ? "Auto-deploy bật" : "Auto-deploy tắt";
            badge.className = "badge " + (enabled ? "ok" : "warn");
          }
          toastSuccess(enabled ? "Đã bật auto-deploy" : "Đã tắt auto-deploy — build vẫn chạy, không deploy cluster");
        } catch (err) {
          autoToggle.checked = !enabled;
          toastError(err.message);
        } finally {
          autoToggle.disabled = false;
        }
      };
    }

    const repoFormEl = document.getElementById("project-repo-form");
    if (repoFormEl && canWriteK8s()) {
      repoFormEl.onsubmit = async function (e) {
        e.preventDefault();
        const fd = new FormData(repoFormEl);
        await api("/api/v1/projects/" + encodeURIComponent(slug) + "/repo", {
          method: "PATCH",
          body: {
            git_url: fd.get("git_url"),
            branch: fd.get("branch"),
            dockerfile_path: fd.get("dockerfile_path"),
            build_context: fd.get("build_context"),
          },
        });
        toastSuccess("Đã lưu cấu hình Git");
        pageProjectHub(main, slug, "deploy");
      };
    }

    const applyForm = document.getElementById("deploy-apply-form");
    if (applyForm && plan.can_apply && canWriteK8s()) {
      applyForm.onsubmit = async function (e) {
        e.preventDefault();
        const fd = new FormData(applyForm);
        const tag = (fd.get("image_tag") || "latest").toString().trim() || "latest";
        if (!(await uiConfirm("Deploy image vào " + (plan.namespace || ns) + "?", { title: "Deploy workload" }))) {
          return;
        }
        try {
          await api("/api/v1/projects/" + encodeURIComponent(slug) + "/deploy/apply", {
            method: "POST",
            body: { environment: env, image_tag: tag },
          });
          toastSuccess("Đã deploy — xem tab Runtime");
          pageProjectHub(main, slug, "runtime");
        } catch (err) {
          toastError(err.message);
        }
      };
    }
    bindEnvSuggestButtons(main, slug, env);
    } catch (err) {
      main.innerHTML =
        projectHeader(p, "Deploy / Git", { help: "deploy" }) +
        '<div class="card"><p class="error-text">Lỗi: ' +
        esc(errorMessage(err, "Không tải được trang Deploy")) +
        '</p><button type="button" class="btn-ghost btn-sm" onclick="location.reload()">Tải lại</button></div>';
    }
}
