/** Sổ lệnh K8s — dialog hướng dẫn (giống deploy help) */
var K8S_OPS_HELP_SECTIONS = [
  {
    id: "start",
    title: "Bắt đầu",
    html:
      '<p class="deploy-help-lead">Gõ lệnh <strong>kubectl read-only</strong> trực tiếp — hoặc bấm <strong>+</strong> để chèn mẫu rồi sửa.</p>' +
      '<div class="deploy-help-checklist">' +
      "<h4>Cách dùng</h4>" +
      '<ul class="deploy-help-list">' +
      "<li>Ô dưới cùng giống terminal — gõ tự do, Enter hoặc <strong>Chạy</strong>.</li>" +
      "<li><strong>+</strong> → chọn lệnh mẫu (chèn vào ô, không chạy ngay).</li>" +
      "<li><strong>Copy kết quả</strong> — copy log/output sau khi chạy (gửi support, lưu ticket).</li>" +
      "<li>Chỉ <code>get</code>, <code>describe</code>, <code>logs</code> — không apply/delete/exec.</li>" +
      "</ul>" +
      "<h4>Phạm vi</h4>" +
      '<ul class="deploy-help-list">' +
      "<li><strong>Project</strong> — namespace dev/prod của project.</li>" +
      "<li><strong>Platform</strong> — thêm cluster (nodes, namespaces toàn cục).</li>" +
      "</ul></div>",
  },
  {
    id: "pods",
    title: "Pods",
    html:
      '<pre class="plugin-install-cmd">kubectl get pods -n {namespace}\nkubectl describe pod {pod} -n {namespace}\nkubectl logs {pod} -n {namespace} --tail=300</pre>' +
      '<p class="muted">Thay <code>{namespace}</code>, <code>{pod}</code> bằng tên thật. Log thêm <code>-c {container}</code> nếu pod nhiều container.</p>',
  },
  {
    id: "workload",
    title: "Workload",
    html:
      '<pre class="plugin-install-cmd">kubectl get deployments -n {namespace}\nkubectl describe deployment {deployment} -n {namespace}\nkubectl get svc -n {namespace}\nkubectl get ingress -n {namespace}</pre>',
  },
  {
    id: "debug",
    title: "Debug",
    html:
      '<pre class="plugin-install-cmd">kubectl get events -n {namespace} --sort-by=.lastTimestamp</pre>' +
      '<p class="muted">Events hữu ích khi pod CrashLoop / ImagePullBackOff.</p>',
  },
  {
    id: "cluster",
    title: "Cluster",
    html:
      '<p class="deploy-help-lead deploy-help-note-optional">Chỉ <strong>admin / infra</strong>.</p>' +
      '<pre class="plugin-install-cmd">kubectl get namespaces\nkubectl get nodes -o wide</pre>',
  },
  {
    id: "dont",
    title: "Đừng làm",
    html:
      '<p class="deploy-help-lead">Console <em>không</em> chạy lệnh ghi — dùng Rancher UI hoặc SSH nếu cần.</p>' +
      '<table class="data-table deploy-help-table"><thead><tr><th>Lệnh</th><th>Lý do</th></tr></thead><tbody>' +
      "<tr><td><code>apply</code>, <code>delete</code>, <code>create</code></td><td>Thay đổi cluster</td></tr>" +
      "<tr><td><code>exec</code>, <code>port-forward</code></td><td>Shell tương tác — Phase sau</td></tr>" +
      "<tr><td><code>scale</code>, <code>rollout</code></td><td>Dùng tab Runtime / Deploy</td></tr>" +
      "</tbody></table>",
  },
];

function renderK8sOpsHelpButton(sectionId, extraClass) {
  sectionId = sectionId || "start";
  extraClass = extraClass || "";
  return (
    '<button type="button" class="btn-help k8s-ops-help-trigger ' +
    extraClass +
    '" data-k8s-help-section="' +
    esc(sectionId) +
    '" title="Hướng dẫn sổ lệnh K8s" aria-label="Hướng dẫn K8s">' +
    '<span aria-hidden="true">?</span></button>'
  );
}

function openK8sOpsHelpDialog(sectionId) {
  sectionId = sectionId || "start";
  return new Promise(function (resolve) {
    const overlay = document.createElement("div");
    overlay.className = "ui-overlay";
    const tabs = K8S_OPS_HELP_SECTIONS.map(function (s) {
      return (
        '<button type="button" class="deploy-help-tab' +
        (s.id === sectionId ? " active" : "") +
        '" data-section="' +
        esc(s.id) +
        '">' +
        esc(s.title) +
        "</button>"
      );
    }).join("");
    const panels = K8S_OPS_HELP_SECTIONS.map(function (s) {
      return (
        '<div class="deploy-help-panel' +
        (s.id === sectionId ? " active" : "") +
        '" data-section="' +
        esc(s.id) +
        '">' +
        s.html +
        "</div>"
      );
    }).join("");
    overlay.innerHTML =
      '<div class="ui-dialog ui-dialog-help" role="dialog" aria-modal="true">' +
      '<div class="ui-dialog-glow"></div>' +
      '<div class="deploy-help-header">' +
      '<h3 class="ui-dialog-title">Sổ lệnh K8s</h3>' +
      '<p class="muted deploy-help-sub">Read-only · gõ tự do hoặc chèn mẫu từ +</p>' +
      "</div>" +
      '<nav class="deploy-help-tabs" role="tablist">' +
      tabs +
      "</nav>" +
      '<div class="deploy-help-body">' +
      panels +
      "</div>" +
      '<div class="ui-dialog-actions deploy-help-actions">' +
      '<button type="button" class="btn-primary ui-dialog-ok">Đóng</button></div></div>';

    function close() {
      overlay.classList.remove("show");
      setTimeout(function () {
        overlay.remove();
        document.removeEventListener("keydown", onKey);
        resolve();
      }, 200);
    }
    function onKey(e) {
      if (e.key === "Escape") close();
    }
    overlay.querySelectorAll(".deploy-help-tab").forEach(function (tab) {
      tab.onclick = function () {
        const id = tab.dataset.section;
        overlay.querySelectorAll(".deploy-help-tab").forEach(function (t) {
          t.classList.toggle("active", t.dataset.section === id);
        });
        overlay.querySelectorAll(".deploy-help-panel").forEach(function (p) {
          p.classList.toggle("active", p.dataset.section === id);
        });
      };
    });
    overlay.querySelector(".ui-dialog-ok").onclick = close;
    overlay.onclick = function (e) {
      if (e.target === overlay) close();
    };
    document.body.appendChild(overlay);
    document.addEventListener("keydown", onKey);
    requestAnimationFrame(function () {
      overlay.classList.add("show");
      overlay.querySelector(".ui-dialog-ok").focus();
    });
  });
}

function bindK8sOpsHelpTriggers(root) {
  (root || document).querySelectorAll(".k8s-ops-help-trigger").forEach(function (btn) {
    if (btn.dataset.k8sHelpBound === "1") return;
    btn.dataset.k8sHelpBound = "1";
    btn.onclick = function (e) {
      e.preventDefault();
      e.stopPropagation();
      openK8sOpsHelpDialog(btn.dataset.k8sHelpSection || "start");
    };
  });
}
