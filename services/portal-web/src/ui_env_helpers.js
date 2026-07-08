/* Env helpers. */

function renderEnvReadinessPanel(readiness, slug, env, scope) {
  if (!readiness) return "";
  const c = readiness.contract || {};
  const issues = (c.issues || []).filter(function (i) { return i.severity === "error"; });
  const warnIssues = (c.issues || []).filter(function (i) { return i.severity === "warning"; });
  const warnings = c.warnings || [];
  const suggestions = readiness.suggestions || [];
  let html = "";

  function renderWarnBlock(title, lines) {
    if (!lines.length) return "";
    return (
      '<div class="env-readiness env-readiness-warn">' +
      "<strong>" + esc(title) + "</strong><ul>" +
      lines.map(function (w) { return "<li>" + esc(w) + "</li>"; }).join("") +
      "</ul></div>"
    );
  }

  if (!readiness.ready && issues.length) {
    html +=
      '<div class="env-readiness env-readiness-error">' +
      "<strong>Chưa sẵn sàng" + (scope === "build" ? " build" : "") + "</strong>" +
      "<ul>" +
      issues.map(function (i) {
        let line =
          '<li class="env-readiness-issue"><span class="env-readiness-issue-msg">' + esc(i.message) + "</span>";
        if (i.description) {
          line += '<span class="env-readiness-issue-desc muted">' + esc(i.description) + "</span>";
        }
        return line + "</li>";
      }).join("") +
      "</ul>";
    if (suggestions.length) {
      html +=
        '<p class="muted" style="margin:8px 0 4px">Từ <code>.platform/' +
        (scope === "build" ? "build.yaml" : "runtime.yaml") +
        '</code> — thêm trên Console hoặc bấm <strong>Lấy key từ contract</strong>:</p><div class="env-suggest-row">';
      suggestions.forEach(function (s) {
        html +=
          '<button type="button" class="btn-sm btn-ghost env-suggest-add" data-key="' + esc(s.key) + '" data-scope="' + esc(scope) + '"' +
          (s.required ? ' title="Bắt buộc"' : "") +
          ">+ " + esc(s.key) + (s.required ? " *" : "") + "</button> ";
      });
      html += "</div>";
    }
    html += "</div>";
  } else if (c.contract_found && readiness.ready) {
    html =
      '<div class="env-readiness env-readiness-ok"><span class="badge ok">Đã đủ cấu hình</span>' +
      ' <span class="muted">Contract <code>' + esc(c.contract_path || "") + "</code></span></div>";
  } else if (c.contract_found && warnings.length && !issues.length) {
    html = renderWarnBlock("Cảnh báo", warnings);
  }

  const driftLines = warnIssues.map(function (i) { return i.message || i.description || ""; }).filter(Boolean);
  const extraWarn = warnings.filter(function (w) {
    return driftLines.indexOf(w) < 0;
  });
  const allWarn = driftLines.concat(extraWarn);
  if (readiness.ready && allWarn.length) {
    html += renderWarnBlock("Cảnh báo contract / Dockerfile", allWarn);
  } else if (!readiness.ready && allWarn.length && issues.length) {
    html += renderWarnBlock("Cảnh báo thêm", allWarn);
  }

  return html;
}

function renderMissingContractRows(suggestions, scope) {
  return (suggestions || [])
    .map(function (s) {
      return (
        '<tr class="env-row-missing"><td><code>' +
        esc(s.key) +
        '</code> <span class="badge warn">contract</span></td><td class="muted">' +
        esc(s.description || "Chưa có trên Console") +
        '</td><td><button type="button" class="btn-sm btn-primary env-suggest-add" data-key="' +
        esc(s.key) +
        '" data-scope="' +
        esc(scope) +
        '">Thêm</button></td></tr>'
      );
    })
    .join("");
}

function renderEnvSyncNote(syncStatus) {
  if (!syncStatus) return "";
  const synced = !!syncStatus.synced;
  const badge = synced
    ? '<span class="badge ok">Đã khớp cluster</span>'
    : '<span class="badge warn">Chưa khớp cluster</span>';
  return (
    '<p class="env-sync-note' +
    (synced ? " env-sync-ok" : " env-sync-pending") +
    '">' +
    badge +
    ' <span class="muted">' +
    esc(syncStatus.detail || "") +
    "</span></p>"
  );
}

async function promptContractKeys(slug, env, suggestions, scope) {
  const list = suggestions || [];
  if (!list.length) {
    toastSuccess("Đã đủ key từ contract");
    return;
  }
  for (let i = 0; i < list.length; i++) {
    const s = list[i];
    const result = await openEnvVarDialog(slug, env, { key: s.key, scope: scope || "build" });
    if (!result) {
      if (i === 0) return;
      break;
    }
  }
  const main = document.getElementById("main");
  if (main) pageProjectHub(main, slug, "env");
}

function bindEnvSuggestButtons(main, slug, env) {
  main.querySelectorAll(".env-suggest-add").forEach(function (btn) {
    btn.onclick = function () {
      openEnvVarDialog(slug, env, { key: btn.dataset.key, scope: btn.dataset.scope || "build" });
    };
  });
}

function envVarIsBuildScope(v) {
  return String((v && v.scope) || "").toLowerCase() === "build";
}

function envVarIsRuntimeScope(v) {
  const s = String((v && v.scope) || "").toLowerCase();
  return s === "" || s === "runtime";
}

function renderPlatformBuildArgRows() {
  return [
    { key: "GIT_SHA", desc: "Tự động mỗi lần build (commit SHA)" },
    { key: "GIT_REF", desc: "Tự động mỗi lần build (branch/tag)" },
  ]
    .map(function (r) {
      return (
        '<tr class="env-row-platform"><td><code>' +
        esc(r.key) +
        '</code> <span class="badge neutral">platform</span></td><td class="muted">' +
        esc(r.desc) +
        "</td><td></td></tr>"
      );
    })
    .join("");
}

function renderEnvVarTable(rows, slug, env, canEditEnv, scope) {
  return (rows || [])
    .map(function (v) {
      const valCell = v.is_secret
        ? '<span class="env-secret-val">' + esc(v.value || "—") + ' <span class="badge neutral">secret</span></span>'
        : "<code>" + esc(v.value || "—") + "</code>";
      return (
        "<tr><td><code>" + esc(v.key) + "</code></td><td>" + valCell + "</td><td>" +
        (canEditEnv
          ? '<button type="button" class="btn-sm env-edit" data-id="' + v.id + '" data-key="' + esc(v.key) + '" data-secret="' + (v.is_secret ? "1" : "0") + '" data-scope="' + esc(scope) + '">Sửa</button> ' +
            '<button type="button" class="btn-sm btn-danger env-del" data-id="' + v.id + '" data-key="' + esc(v.key) + '">Xóa</button>'
          : "") +
        "</td></tr>"
      );
    })
    .join("");
}

function bindEnvVarTableActions(main, slug, env) {
  main.querySelectorAll(".env-del").forEach(function (btn) {
    btn.onclick = async function () {
      if (!(await uiConfirm('Xóa biến "' + btn.dataset.key + '"?', { danger: true, title: "Xóa cấu hình" }))) return;
      try {
        const res = await api("/api/v1/projects/" + encodeURIComponent(slug) + "/env/" + btn.dataset.id, {
          method: "DELETE",
        });
        if (res.sync_warning) toastError(res.sync_warning);
        else toastSuccess("Đã xóa");
        pageProjectHub(main, slug, "env");
      } catch (err) {
        toastError(err.message);
      }
    };
  });
  main.querySelectorAll(".env-edit").forEach(function (btn) {
    btn.onclick = function () {
      openEnvVarDialog(slug, env, {
        id: parseInt(btn.dataset.id, 10),
        key: btn.dataset.key,
        is_secret: btn.dataset.secret === "1",
        scope: btn.dataset.scope || "runtime",
      });
    };
  });
}

function openEnvVarDialog(slug, environment, existing) {
  const initScope = (existing && existing.scope) || "runtime";
  const scopeLabel =
    initScope === "build" ? "Khi build image (Dockerfile ARG)" : "Khi app chạy (Pod)";
  return new Promise(function (resolve) {
    const isEdit = !!(existing && existing.id);
    const overlay = document.createElement("div");
    overlay.className = "ui-overlay";
    overlay.innerHTML =
      '<div class="ui-dialog" role="dialog" aria-modal="true">' +
      '<div class="ui-dialog-glow"></div>' +
      "<h3 class=\"ui-dialog-title\">" + (isEdit ? "Sửa cấu hình" : "Thêm cấu hình") + "</h3>" +
      '<form id="env-var-dialog-form" class="login-form dialog-form">' +
      (isEdit
        ? '<p class="muted">Key: <code>' + esc(existing.key) + "</code> · " +
          (initScope === "build" ? "Khi build image" : "Khi app chạy") + "</p>"
        : '<p class="muted">Loại: <strong>' + esc(scopeLabel) + "</strong></p>" +
          '<input type="hidden" name="scope" value="' + esc(initScope) + '" />' +
          '<label>Key<input name="key" required placeholder="APP_VERSION" pattern="[^\\s]+" value="' + esc((existing && existing.key) || "") + '" /></label>') +
      '<label>Value<textarea name="value" rows="3" placeholder="giá trị…" required></textarea></label>' +
      '<label class="checkbox-row"><input type="checkbox" name="is_secret"' + (existing && existing.is_secret ? " checked" : "") + " /> Secret (ẩn trên UI" + (initScope === "build" ? " + đẩy GitHub Secrets" : "") + ")</label>" +
      '<p class="muted">Môi trường: <strong>' + esc(environment) + "</strong></p>" +
      '<div class="ui-dialog-actions" style="margin-top:16px;padding-top:0;border:0">' +
      '<button type="button" class="btn-ghost ui-dialog-cancel">Huỷ</button>' +
      '<button type="submit" class="btn-primary">' + (isEdit ? "Lưu" : "Thêm") + "</button></div></form></div>";

    function close(result) {
      overlay.classList.remove("show");
      setTimeout(function () {
        overlay.remove();
        resolve(result);
      }, 200);
    }

    overlay.querySelector(".ui-dialog-cancel").onclick = function () { close(null); };
    overlay.onclick = function (e) {
      if (e.target === overlay) close(null);
    };
    document.body.appendChild(overlay);
    requestAnimationFrame(function () { overlay.classList.add("show"); });

    const form = overlay.querySelector("#env-var-dialog-form");
    form.onsubmit = async function (e) {
      e.preventDefault();
      const fd = new FormData(form);
      const submitBtn = form.querySelector('button[type="submit"]');
      submitBtn.disabled = true;
      try {
        let res;
        if (isEdit) {
          res = await api("/api/v1/projects/" + encodeURIComponent(slug) + "/env/" + existing.id, {
            method: "PATCH",
            body: {
              value: fd.get("value"),
              is_secret: !!form.querySelector('input[name="is_secret"]').checked,
            },
          });
        } else {
          res = await api("/api/v1/projects/" + encodeURIComponent(slug) + "/env", {
            method: "POST",
            body: {
              key: fd.get("key"),
              value: fd.get("value"),
              is_secret: !!form.querySelector('input[name="is_secret"]').checked,
              environment: environment,
              scope: (form.querySelector('[name="scope"]') && form.querySelector('[name="scope"]').value) || initScope,
            },
          });
        }
        if (res.sync_warning) {
          toastError(res.sync_warning);
        } else {
          toastSuccess(isEdit ? "Đã lưu" : "Đã thêm biến");
        }
        close(res);
        const main = document.getElementById("main");
        if (main) pageProjectHub(main, slug, "env");
      } catch (err) {
        toastError(err.message);
        submitBtn.disabled = false;
      }
    };
    const firstInput = form.querySelector('input[name="key"], textarea[name="value"]');
    if (firstInput) firstInput.focus();
  });
}
