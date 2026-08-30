# exo-gui

Telemetry pipeline for an exoskeleton: an STM32 (C++) reads sensors/motors and a
dedicated ESP32 WiFi coprocessor ships batched telemetry to a headless Debian
server (Go backend + Postgres). A React SPA renders it live for an operator signed in through OAuthManager.

> **Mid-refactor** (branch `la/auth`).

## Layout

| Path             | What                                                                       |
|------------------|----------------------------------------------------------------------------|
| `frontend/`      | React dashboard (Tailwind 4) — Vite SPA.                                   |
| `backend/`       | Go service (Fiber HTTP ingest + WebSocket fan-out + Postgres). Skeleton today. |
| `prisma/`        | Postgres schema + migrations.                                              |
| `.devcontainer/` | Slim Debian development image with Node 22 + Go 1.23.                     |
| `docker-compose.yml` | Migration, API, and web containers on the shared `biotron` network.   |

## Develop & test

No Node/Go needed on your host — just Docker.

### Option A — IDE dev container (recommended)

1. Open the repo in VS Code / Cursor with the **Dev Containers** extension.
2. **Reopen in Container** — installs frontend/Prisma packages and downloads Go modules.
   The container does not start a database; point `DATABASE_URL` at the shared
   Postgres instance. Host services are available as `host.docker.internal`.
3. In the container terminal:
   ```bash
   cd frontend && npm run dev     # → http://localhost:5173 (auto-forwards, opens browser)
   cd backend  && go run .        # → curl http://localhost:8080/health  ->  {"status":"ok"}
   ```
   Ports 5173 / 8080 forward to your host automatically (labeled in the Ports panel).

### Option B — plain Docker (no IDE)

```bash
# Backend  → http://localhost:8080/health
docker run --rm -p 8080:8080 -v "$PWD/backend":/app -w /app golang:1.23 go run .

# Frontend → http://localhost:5173
docker run --rm -p 5173:5173 -v "$PWD/frontend":/app -w /app node:22 \
  sh -lc "npm install && npm run dev"
```

Vite is configured with `server.host: true` (`vite.config.ts`), so it binds all
interfaces and is reachable through the published port.

### Deployment-shaped Compose stack

Start the one shared Postgres and Redis stack from `../Server`, copy
`.env.example` to `.env`, replace the example Postgres password, then run:

```bash
docker compose up --build
```

The web container binds to port 5174 by default so OAuthManager can retain 5173.
The backend, frontend, Prisma, and Compose all use the single root `.env`; do
not create component-level environment files.
