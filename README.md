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
| `.devcontainer/` | Slim Debian development image with Node 22 + Go 1.27.                     |
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
   cd frontend && npm run dev     # → http://localhost:5174 (auto-forwards, opens browser)
   cd backend  && PORT=8081 go run .   # → curl http://localhost:8081/health  ->  {"status":"ok"}
   ```
   Ports 5174 / 8081 forward to your host automatically (labeled in the Ports panel).

### Option B — plain Docker (no IDE)

```bash
# Backend  → http://localhost:8081/health
docker run --rm -p 8081:8080 -v "$PWD/backend":/app -w /app golang:1.27 go run .

# Frontend → http://localhost:5174
docker run --rm -p 5174:5174 -v "$PWD/frontend":/app -w /app node:22 \
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

Host ports are 5174 for the web container and 8081 for the API, leaving
OAuthManager on 5173 and 8080 where its GitHub callback is registered.
The backend, frontend, Prisma, and Compose all use the single root `.env`; do
not create component-level environment files.

## Authorization

OAuthManager owns identity and per-product permissions. Exo's app id is
`exo-gui` and it declares three permissions:

| Permission   | What it opens              | Daily guest key |
|--------------|----------------------------|-----------------|
| `live`       | live telemetry             | yes             |
| `historical` | recorded sessions          | yes             |
| `commands`   | commands sent to the rig   | **no**          |

`backend/internal/auth` is the single enforcement point. It asks
`GET {OAUTH_MANAGER_URL}/v1/check?app=exo-gui&permission=<key>`, forwarding the
caller's session cookie, and maps the answer to a status:

| Situation                                        | Exo responds |
|--------------------------------------------------|--------------|
| no cookie, or OAuthManager answers 401            | `401` |
| `allowed: false`, or OAuthManager answers 403     | `403` |
| OAuthManager unreachable, unexpected status, or `OAUTH_MANAGER_URL` unset | `503` |
| `allowed: true`                                   | handler runs |

401 and 403 are deliberately distinct: a signed-in operator who lacks a
permission must not be sent back to the sign-in button that already worked.
Every failure mode is a denial — there is no configuration in which the gate
opens.

### Wiring a new route

Mount routes on a group that is already gated, never by adding the check inside
a handler; that way a second route on the same group cannot forget it.

```go
live := app.Group("/v1/live", authz.Require(auth.PermissionLive))
live.Get("/stream", h.Stream)

hist := app.Group("/v1/historical", authz.Require(auth.PermissionHistorical))
hist.Get("/sessions", h.Sessions)

// Commands mutate hardware, so they also want the X-Requested-With CSRF guard
// the other products apply to every mutation.
cmd := app.Group("/v1/commands", requireXHR, authz.Require(auth.PermissionCommands))
cmd.Post("/stop", h.Stop)
```

`auth.DecisionFrom(c)` returns the decision the gate already made, so a handler
never needs a second round trip. The permission keys live in
`backend/internal/auth` as `auth.PermissionLive`, `auth.PermissionHistorical`
and `auth.PermissionCommands` — use the constants, not string literals.
