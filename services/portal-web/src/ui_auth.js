/* Auth / login */

function handleSessionLost(msg) {
  stopDeployPoll();
  state.user = null;
  state.projectCtx = null;
  if (state.onLoginPage) {
    if (msg) {
      setLoginError(msg);
      toastError(msg);
    }
    return;
  }
  showLoginPage(msg);
}

function roleLabel(role) {
  const m = { admin: "Admin", tech_lead: "Tech Lead", dev: "Developer", readonly: "Read-only" };
  return m[role] || role;
}

const ROLE_HELP = [
  {
    role: "admin",
    title: "Admin",
    desc: "Toàn quyền platform",
    perms: ["Quản lý user (tạo, đổi role, vô hiệu)", "Platform Policy (trần Redis/MinIO/Ingress + Unlock)", "Xem audit log", "Thêm worker node", "Xem & thao tác mọi project/namespace", "Xóa project (sau này)"],
  },
  {
    role: "tech_lead",
    title: "Tech Lead",
    desc: "Giám sát team, không quản lý user",
    perms: ["Quản lý Projects (wizard Harbor + namespace)", "Xem mọi project & cluster", "Xem audit log", "Thêm worker node", "Deploy/restart prod (theo policy)"],
  },
  {
    role: "dev",
    title: "Developer",
    desc: "Chỉ project được gán",
    perms: ["Menu Dự án + Pods/Deployments/Services/Ingress trong namespace dev", "Không thấy Hạ tầng (nodes, events cluster-wide…)", "Không thêm worker / quản lý user", "Không thao tác namespace prod"],
  },
  {
    role: "readonly",
    title: "Read-only",
    desc: "Chỉ xem",
    perms: ["Xem pod, deployment, log trong namespace dev + prod được gán", "Không scale, restart, xóa", "Không menu Hạ tầng cluster"],
  },
];

function roleHelpHtml() {
  return (
    '<div class="role-help-grid">' +
    ROLE_HELP.map(function (r) {
      return (
        '<div class="role-help-card">' +
        '<h4>' + esc(r.title) + ' <span class="muted">(' + esc(r.role) + ")</span></h4>" +
        "<p>" + esc(r.desc) + "</p><ul>" +
        r.perms.map(function (p) { return "<li>" + esc(p) + "</li>"; }).join("") +
        "</ul></div>"
      );
    }).join("") +
    "</div>"
  );
}

function showLoginPage(msg) {
  state.onLoginPage = true;
  stopDeployPoll();
  document.querySelector(".sidebar")?.classList.add("hidden");
  const nav = $("#sidebar-nav");
  if (nav) nav.innerHTML = "";
  const main = $("#main");
  if (!main) return;
  const existingForm = document.getElementById("login-form");
  if (existingForm) {
    if (msg) setLoginError(msg);
    else clearLoginError();
    return;
  }
  main.innerHTML =
    '<div class="login-wrap">' +
    '<div class="login-card">' +
    "<h2>Platform Console</h2>" +
    "<p class=\"muted\">Đăng nhập để quản lý cluster</p>" +
    '<p class="login-session-error" role="alert" aria-live="polite" hidden></p>' +
    '<form id="login-form" class="login-form">' +
    '<label>Email<input type="email" name="email" autocomplete="username" required /></label>' +
    '<label>Mật khẩu<input type="password" name="password" autocomplete="current-password" required minlength="12" /></label>' +
    '<button type="submit" class="btn-primary" id="login-submit-btn">Đăng nhập</button>' +
    "</form>" +
    '<div id="quick-login-box" class="quick-login-box quick-login-loading">' +
    '<p class="muted quick-login-placeholder">Đang tải đăng nhập nhanh…</p></div>' +
    '<p class="login-hint muted">Mật khẩu ≥ 12 ký tự, có chữ và số</p>' +
    "</div></div>";
  if (msg) setLoginError(msg);
  $("#login-form").onsubmit = async (e) => {
    e.preventDefault();
    clearLoginError();
    const fd = new FormData(e.target);
    const btn = document.getElementById("login-submit-btn");
    if (btn) {
      btn.disabled = true;
      btn.dataset.label = btn.textContent;
      btn.innerHTML = '<span class="btn-spinner"></span> Đang đăng nhập…';
    }
    try {
      await performLogin(String(fd.get("email") || ""), String(fd.get("password") || ""));
    } catch (err) {
      setLoginError(err.message || "Đăng nhập thất bại");
      toastError(err.message || "Đăng nhập thất bại");
    } finally {
      if (btn) {
        btn.disabled = false;
        btn.textContent = btn.dataset.label || "Đăng nhập";
      }
    }
  };
  bindQuickLoginBox();
}

function setLoginError(msg) {
  msg = String(msg || "").trim();
  if (!msg) return;
  const card = document.querySelector(".login-card");
  if (!card) return;
  let errEl = card.querySelector(".login-session-error");
  if (!errEl) {
    errEl = document.createElement("p");
    errEl.className = "login-session-error";
    errEl.setAttribute("role", "alert");
    errEl.setAttribute("aria-live", "polite");
    const form = document.getElementById("login-form");
    if (form) card.insertBefore(errEl, form);
    else card.appendChild(errEl);
  }
  errEl.hidden = false;
  errEl.removeAttribute("hidden");
  errEl.classList.add("login-session-error--visible");
  errEl.textContent = msg;
  errEl.scrollIntoView({ block: "nearest", behavior: "smooth" });
}

function clearLoginError() {
  const errEl = document.querySelector(".login-card .login-session-error");
  if (!errEl) return;
  errEl.textContent = "";
  errEl.hidden = true;
  errEl.classList.remove("login-session-error--visible");
}

async function performLogin(email, password) {
  const data = await api("/api/v1/auth/login", {
    method: "POST",
    body: { email: email, password: password },
  });
  state.user = data.user;
  state.onLoginPage = false;
  document.querySelector(".sidebar")?.classList.remove("hidden");
  const keepHash = location.hash && location.hash.length > 2;
  if (!keepHash) {
    location.hash = "#/" + defaultHomeRoute();
  }
  await navigate();
}

async function bindQuickLoginBox() {
  const box = document.getElementById("quick-login-box");
  if (!box) return;
  try {
    const hint = await api("/api/v1/auth/quick-login", { silent401: true });
    if (!hint || !hint.enabled || !hint.email || !hint.password) {
      box.className = "quick-login-box quick-login-empty";
      box.innerHTML = "";
      return;
    }
    box.className = "quick-login-box";
    box.innerHTML =
      '<p class="quick-login-title muted">' + esc(hint.label || "Đăng nhập nhanh") + " <span class=\"quick-login-temp\">(tạm)</span></p>" +
      '<button type="button" class="quick-login-btn" id="quick-login-btn">' +
      '<span class="quick-login-role">Admin</span>' +
      '<span class="quick-login-cred"><code>' + esc(hint.email) + "</code> · <code>" + esc(hint.password) + "</code></span>" +
      '<span class="quick-login-action">Bấm để đăng nhập →</span>' +
      "</button>";
    document.getElementById("quick-login-btn").onclick = async function () {
      const form = document.getElementById("login-form");
      if (form) {
        const emailEl = form.querySelector('[name="email"]');
        const passEl = form.querySelector('[name="password"]');
        if (emailEl) emailEl.value = hint.email;
        if (passEl) passEl.value = hint.password;
      }
      clearLoginError();
      try {
        await performLogin(hint.email, hint.password);
      } catch (err) {
        setLoginError(err.message || "Đăng nhập thất bại");
        toastError(err.message || "Đăng nhập thất bại");
      }
    };
  } catch (_e) {
    box.className = "quick-login-box quick-login-empty";
    box.innerHTML = "";
  }
}

async function logout() {
  try {
    await api("/api/v1/auth/logout", { method: "POST" });
  } catch (_) {}
  state.user = null;
  showLoginPage();
}

async function ensureAuth() {
  const ctrl = new AbortController();
  const timer = setTimeout(function () { ctrl.abort(); }, 8000);
  try {
    state.user = await api("/api/v1/auth/me", { signal: ctrl.signal, silent401: true });
    state.onLoginPage = false;
    return true;
  } catch (_) {
    showLoginPage();
    return false;
  } finally {
    clearTimeout(timer);
  }
}
