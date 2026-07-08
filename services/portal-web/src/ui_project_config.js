/* Project hub — Domains + Settings tabs (extracted from app.js). */

async function loadProjectDomains(main, slug, p, data, canManage) {

    const domRes = await api("/api/v1/projects/" + encodeURIComponent(slug) + "/domains" + qs()).catch(function () {
      return { items: data.domains || [] };
    });
    const domains = domRes.items || [];
    let drows = domains
      .map(function (d) {
        const urlCell = d.url
          ? '<a href="' + esc(d.url) + '" target="_blank" rel="noopener">' + esc(d.hostname) + "</a>"
          : esc(d.hostname);
        return (
          "<tr><td>" +
          urlCell +
          "</td><td>" +
          esc(d.environment) +
          "</td><td>" +
          domainKindBadge(d.kind) +
          "</td><td>" +
          domainSyncBadge(d.sync_status) +
          (d.sync_error ? '<br><span class="muted">' + esc(d.sync_error) + "</span>" : "") +
          "</td><td>" +
          domainCertBadge(d.cert_status, d.tls_enabled) +
          "</td><td>" +
          (canManage
            ? '<button type="button" class="btn-sm domain-sync" data-id="' +
              d.id +
              '">Sync</button> ' +
              (d.kind !== "auto"
                ? '<button type="button" class="btn-sm btn-danger domain-del" data-id="' + d.id + '">Xóa</button>'
                : "")
            : "") +
          "</td></tr>" +
          (d.kind === "custom" && d.dns
            ? '<tr class="dns-row"><td colspan="6">' + renderDNSHint(d.dns) + "</td></tr>"
            : d.kind === "auto" && d.dns
              ? '<tr class="dns-row"><td colspan="6">' + renderDNSHint(d.dns) + "</td></tr>"
              : "")
        );
      })
      .join("");
    main.innerHTML =
      projectHeader(p, "Domains · URL & Ingress") +
      '<div class="card"><h3>URL truy cập app</h3>' +
      '<p class="muted">Domain <strong>Tự động</strong> dùng ngay (sslip / subdomain platform). <strong>Custom</strong> cần cấu hình DNS trỏ về cluster.</p>' +
      '<p class="muted deploy-help-note-optional" style="margin-top:8px;font-size:12px">Mỗi hostname chỉ gắn <strong>một project</strong> trên toàn platform — project khác dùng cùng tên sẽ bị chặn. Cùng project có thể thêm <strong>nhiều domain khác tên</strong> trên một env (dev/prod).</p>' +
      (canManage
        ? '<form id="add-domain-form" class="login-form" style="max-width:520px;margin-bottom:16px">' +
          '<div class="form-row"><label>Custom hostname<input name="hostname" required placeholder="api.congty.com" /></label>' +
          '<label>Env<select name="environment"><option value="dev">dev</option><option value="prod">prod</option></select></label></div>' +
          '<button type="submit" class="btn-primary">Thêm & đồng bộ Ingress</button></form>'
        : "") +
      '<div class="table-wrap"><table><thead><tr><th>Hostname</th><th>Env</th><th>Loại</th><th>Ingress</th><th>TLS</th><th></th></tr></thead><tbody>' +
      (drows || '<tr><td colspan="6" class="muted">Chưa có domain — tạo project sẽ có URL dev/prod tự động</td></tr>') +
      "</tbody></table></div></div>";
    const addDom = document.getElementById("add-domain-form");
    if (addDom) {
      addDom.onsubmit = async function (e) {
        e.preventDefault();
        const fd = new FormData(addDom);
        try {
          await api("/api/v1/projects/" + encodeURIComponent(slug) + "/domains", {
            method: "POST",
            body: { hostname: fd.get("hostname"), environment: fd.get("environment") },
          });
          toastSuccess("Đã thêm domain và đồng bộ Ingress");
          pageProjectHub(main, slug, "domains");
        } catch (err) {
          toastError(err.message);
        }
      };
    }
    main.querySelectorAll(".domain-sync").forEach(function (btn) {
      btn.onclick = async function () {
        try {
          await api(
            "/api/v1/projects/" + encodeURIComponent(slug) + "/domains/" + btn.dataset.id + "/sync" + qs(),
            { method: "POST" }
          );
          toastSuccess("Đã đồng bộ Ingress");
          pageProjectHub(main, slug, "domains");
        } catch (err) {
          toastError(err.message);
        }
      };
    });
    main.querySelectorAll(".domain-del").forEach(function (btn) {
      btn.onclick = async function () {
        if (!(await uiConfirm("Xóa domain này?", { danger: true, title: "Xóa domain" }))) return;
        await api("/api/v1/projects/" + encodeURIComponent(slug) + "/domains/" + btn.dataset.id, { method: "DELETE" });
        pageProjectHub(main, slug, "domains");
      };
    });
}

async function loadProjectSettings(main, slug, p, data, canManage) {

    const members = data.members || [];
    const providersRes = canManage ? await api("/api/v1/registry/providers").catch(function () { return { items: [] }; }) : { items: [] };
    const providers = providersRes.items || [];
    const users = canManage ? await api("/api/v1/team/users").catch(() => ({ items: [] })) : { items: [] };
    let mrows = members
      .map(function (m) {
        const roleBadge =
          m.role === "owner"
            ? '<span class="badge ok">owner</span>'
            : m.role === "dev"
              ? '<span class="badge">dev</span>'
              : '<span class="badge muted">' + esc(m.role) + "</span>";
        return (
          "<tr><td>" +
          esc(m.email) +
          "</td><td>" +
          esc(m.display_name || "—") +
          "</td><td>" +
          roleBadge +
          "</td>" +
          (canManage && m.role !== "owner"
            ? '<td class="col-actions"><button type="button" class="btn-sm btn-danger mem-del" data-id="' +
              m.user_id +
              '">Gỡ</button></td>'
            : "<td></td>") +
          "</tr>"
        );
      })
      .join("");
    const addMemberOpts = (users.items || [])
      .filter(function (u) {
        return !members.some(function (m) { return m.user_id === u.id; });
      })
      .map(function (u) {
        return '<option value="' + u.id + '">' + esc(u.email) + "</option>";
      })
      .join("");
    main.innerHTML =
      projectHeader(p, "Cài đặt · registry & thành viên") +
      (canManage
        ? '<div class="card detail-card project-settings-card">' +
          "<h3>Registry</h3>" +
          '<p class="muted project-settings-desc">Nơi lưu image Docker của project (GHCR hoặc Harbor).</p>' +
          (p.registry && p.registry.image_prefix
            ? '<p class="project-settings-current muted">Hiện tại: <code>' + esc(p.registry.image_prefix) + "</code></p>"
            : "") +
          '<form id="registry-form" class="project-settings-form">' +
          registrySelectHtml(providers, p.registry_provider, "ghcr") +
          '<div class="project-settings-actions">' +
          '<button type="submit" class="btn-primary btn-sm">Lưu registry</button>' +
          "</div></form></div>"
        : '<div class="card detail-card project-settings-card"><h3>Registry</h3><p class="muted">' +
          esc((p.registry && p.registry.label) || p.registry_provider) +
          (p.registry && p.registry.image_prefix ? " · <code>" + esc(p.registry.image_prefix) + "</code>" : "") +
          "</p></div>") +
      '<div class="card detail-card project-settings-card">' +
      "<h3>Thành viên project</h3>" +
      '<p class="muted project-settings-desc">Ai được xem và deploy project này.</p>' +
      (canManage && addMemberOpts
        ? '<form id="add-member-form" class="project-member-add">' +
          '<div class="project-member-field project-member-field-grow">' +
          '<span class="field-label">Thành viên</span>' +
          '<div class="select-wrap">' +
          '<select name="user_id">' +
          addMemberOpts +
          '</select><span class="select-chev" aria-hidden="true">▾</span></div></div>' +
          '<div class="project-member-field">' +
          '<span class="field-label">Vai trò</span>' +
          '<div class="select-wrap select-wrap-compact">' +
          '<select name="role">' +
          '<option value="dev">dev</option><option value="readonly">readonly</option>' +
          '</select><span class="select-chev" aria-hidden="true">▾</span></div></div>' +
          '<button type="submit" class="btn-primary btn-sm project-member-add-btn">Thêm</button>' +
          "</form>"
        : "") +
      '<div class="table-wrap project-members-table"><table class="data-table"><thead><tr><th>Email</th><th>Tên</th><th>Vai trò</th><th></th></tr></thead><tbody>' +
      (mrows || '<tr><td colspan="4" class="muted">Chưa có thành viên</td></tr>') +
      "</tbody></table></div></div>";
    const regForm = document.getElementById("registry-form");
    if (regForm) {
      regForm.onsubmit = async function (e) {
        e.preventDefault();
        const fd = new FormData(regForm);
        try {
          await api("/api/v1/projects/" + encodeURIComponent(slug), {
            method: "PATCH",
            body: { registry_provider: fd.get("registry_provider") },
          });
          toastSuccess("Đã cập nhật registry");
          pageProjectHub(main, slug, "settings");
        } catch (err) {
          toastError(err.message);
        }
      };
    }
    const addMem = document.getElementById("add-member-form");
    if (addMem) {
      addMem.onsubmit = async function (e) {
        e.preventDefault();
        const fd = new FormData(addMem);
        await api("/api/v1/projects/" + encodeURIComponent(slug) + "/members", {
          method: "POST",
          body: { user_id: parseInt(fd.get("user_id"), 10), role: fd.get("role") },
        });
        pageProjectHub(main, slug, "settings");
      };
    }
    main.querySelectorAll(".mem-del").forEach(function (btn) {
      btn.onclick = async function () {
        if (!(await uiConfirm("Gỡ thành viên khỏi project?", { danger: true, title: "Gỡ thành viên" }))) return;
        await api("/api/v1/projects/" + encodeURIComponent(slug) + "/members/" + btn.dataset.id, { method: "DELETE" });
        pageProjectHub(main, slug, "settings");
      };
    });
}
