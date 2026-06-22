type Health = { status: string; database?: string };
type Project = {
  id: number;
  name: string;
  namespace_dev: string;
  namespace_prod: string;
};

const healthEl = document.getElementById("health-status")!;
const projectsStatusEl = document.getElementById("projects-status")!;
const projectsListEl = document.getElementById("projects-list")!;

async function loadHealth() {
  try {
    const res = await fetch("/api/v1/health/db");
    const data = (await res.json()) as Health;
    healthEl.className = "ok";
    healthEl.innerHTML = `API: <strong>${data.status}</strong>${
      data.database ? ` · DB: <strong>${data.database}</strong>` : ""
    }`;
  } catch (e) {
    healthEl.className = "error";
    healthEl.textContent = `Lỗi kết nối API: ${e}`;
  }
}

async function loadProjects() {
  try {
    const res = await fetch("/api/v1/projects");
    const data = (await res.json()) as Project[];
    if (!data.length) {
      projectsStatusEl.textContent = "Chưa có project — thêm sau qua API.";
      return;
    }
    projectsStatusEl.hidden = true;
    projectsListEl.hidden = false;
    projectsListEl.innerHTML = data
      .map(
        (p) =>
          `<li>${p.name} — dev: ${p.namespace_dev}, prod: ${p.namespace_prod}</li>`,
      )
      .join("");
  } catch {
    projectsStatusEl.textContent = "Không tải được danh sách project.";
  }
}

loadHealth();
loadProjects();
