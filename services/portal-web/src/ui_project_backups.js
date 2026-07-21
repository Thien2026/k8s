/* Project-scoped backup and sandbox recovery. */
async function loadProjectBackups(main, slug, p) {
  const env = state.projectEnv || "dev";
  main.innerHTML = projectEnvToolbar(slug, p, function () { loadProjectBackups(main, slug, p); }) + '<p class="loading">Đang tải backup project…</p>';
  try {
    const data = await api("/api/v1/projects/" + encodeURIComponent(slug) + "/backups?environment=" + encodeURIComponent(env));
    const items = data.items || [];
    const rows = items.length
      ? items.map(function (v) {
          return "<tr><td>#" + esc(v.run_id) + "</td><td><code>" + esc(v.source_bucket || "app") + "</code></td><td>" + esc(fmtTime(v.finished_at || v.created_at)) + "</td><td>" +
            esc(v.target_name || "—") + "</td><td><code>" + esc(v.object_prefix) + "</code></td><td>" +
            esc(String(v.object_count || "—")) + " · " + esc(formatBytesShort(v.total_bytes || 0)) + "</td><td>" +
            (data.can_restore ? '<button class="btn-ghost btn-sm project-backup-restore" data-artifact="' + esc(String(v.artifact_id)) + '">Admin: restore vùng tạm</button>' : "Chỉ xem") +
            "</td></tr>";
        }).join("")
      : '<tr><td colspan="7" class="muted">Chưa có backup MinIO thành công cho môi trường này.</td></tr>';
    main.innerHTML =
      projectEnvToolbar(slug, p, function () { loadProjectBackups(main, slug, p); }) +
      '<div class="page-header"><h2 class="page-title">Backup &amp; Recovery</h2><p class="page-subtitle">Danh mục dữ liệu MinIO của project ' + esc(env) + " đã nằm trong backup toàn Platform; đây không phải lịch backup riêng của project.</p></div>" +
      '<div class="banner warn">Chỉ Admin Platform được quyền restore. Không có nút ghi đè production: restore chỉ tạo <code>app/__restore/&lt;id&gt;/</code> để kiểm tra trước.</div>' +
      '<div class="card detail-card"><h3>Backup có sẵn</h3><table class="data-table"><thead><tr><th>Run</th><th>Bucket</th><th>Thời gian</th><th>Target</th><th>Prefix backup</th><th>Dữ liệu</th><th>Thao tác</th></tr></thead><tbody>' +
      rows + "</tbody></table></div>";
    main.querySelectorAll(".project-backup-restore").forEach(function (btn) {
      btn.onclick = async function () {
        if (!window.confirm("Restore backup này vào vùng tạm của " + env + "? Dữ liệu đang chạy sẽ không bị thay đổi.")) return;
        btn.disabled = true;
        try {
          const result = await api("/api/v1/projects/" + encodeURIComponent(slug) + "/backups/restore", {
            method: "POST",
            body: { artifact_id: Number(btn.dataset.artifact), target_environment: env },
          });
          toastSuccess("Restore đã xếp hàng vào " + result.restore.target_prefix);
        } catch (err) {
          btn.disabled = false;
          toastError(err.message || "Không tạo được restore");
        }
      };
    });
  } catch (err) {
    main.innerHTML = '<p class="error-text">' + esc(err.message || "Không tải được backup project") + "</p>";
  }
}
