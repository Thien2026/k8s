import { useEffect, useState } from "react";

type Health = { status: string; database?: string };
type Project = {
  id: number;
  name: string;
  namespace_dev: string;
  namespace_prod: string;
};

export default function App() {
  const [health, setHealth] = useState<Health | null>(null);
  const [projects, setProjects] = useState<Project[]>([]);
  const [error, setError] = useState("");

  useEffect(() => {
    fetch("/api/v1/health/db")
      .then((r) => r.json())
      .then(setHealth)
      .catch((e) => setError(String(e)));

    fetch("/api/v1/projects")
      .then((r) => r.json())
      .then(setProjects)
      .catch(() => {});
  }, []);

  return (
    <div style={{ maxWidth: 960, margin: "0 auto", padding: 24 }}>
      <header style={{ marginBottom: 32 }}>
        <h1 style={{ margin: 0 }}>Platform Console</h1>
        <p style={{ color: "#8b98a5", margin: "8px 0 0" }}>
          Quản lý tập trung — K8s, deploy, image
        </p>
      </header>

      <section
        style={{
          background: "#1a2332",
          borderRadius: 12,
          padding: 20,
          marginBottom: 24,
        }}
      >
        <h2 style={{ marginTop: 0, fontSize: 18 }}>Trạng thái hệ thống</h2>
        {error && <p style={{ color: "#f87171" }}>{error}</p>}
        {health && (
          <p>
            API: <strong>{health.status}</strong>
            {health.database && (
              <>
                {" "}
                · DB: <strong>{health.database}</strong>
              </>
            )}
          </p>
        )}
      </section>

      <section
        style={{
          background: "#1a2332",
          borderRadius: 12,
          padding: 20,
        }}
      >
        <h2 style={{ marginTop: 0, fontSize: 18 }}>Projects</h2>
        {projects.length === 0 ? (
          <p style={{ color: "#8b98a5" }}>Chưa có project — thêm sau qua API.</p>
        ) : (
          <ul>
            {projects.map((p) => (
              <li key={p.id}>
                {p.name} — dev: {p.namespace_dev}, prod: {p.namespace_prod}
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}
