# Platform Console services

| Service | Stack | Port dev |
|---------|-------|----------|
| `portal-api` | Go + chi + pgx | 8080 |
| `portal-web` | React + Vite + TS | 5173 |

## Chạy local (sau khi có Postgres)

```bash
# API — đọc DATABASE_URL từ config/postgres.env trên VPS hoặc local
cd services/portal-api
export DATABASE_URL="postgres://platform:PASSWORD@localhost:5432/platform?sslmode=disable"
go run ./cmd/server

# Web
cd services/portal-web
npm install && npm run dev
```

## Deploy lên K8s

Sẽ thêm `bootstrap/steps/08-portal.sh` (đã có) và manifest trong `platform/console/`.
