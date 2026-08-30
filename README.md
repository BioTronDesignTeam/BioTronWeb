# Logger

Structured logs, service health checks, and the internal BioTron log portal.

## Layout

| Path | What |
|---|---|
| `frontend/` | React + TypeScript log and health portal |
| `backend/` | Go/Fiber ingest and query API |
| `prisma/` | Logger-owned Postgres schema and migrations |
| `.devcontainer/` | Node 22 + Go 1.23 development environment |
| `docker-compose.yml` | Logger containers on the shared `biotron` network |

Start the shared Postgres and Redis containers from `../Server`, copy
`.env.example` to `.env`, replace the example password, then run
`docker compose up --build`.

The initial API exposes `GET /health`. Log ingestion and cache behavior build on
this scaffold without adding another database or Redis container.
