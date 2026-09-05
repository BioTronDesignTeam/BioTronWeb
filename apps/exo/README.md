# Exo

Telemetry for an exoskeleton. The goal: an STM32 (C++) reads sensors and
motors, a dedicated ESP32 WiFi coprocessor ships batched telemetry to a
headless Debian server (Go backend and Postgres), and a React SPA renders it
live for an operator signed in through Auth.

What exists today: the sign-in flow, the operator UI shell, the server-side
permission gate, and a health route. Telemetry ingest, storage, and WebSocket
fan-out are not built. The machine list in the UI is a placeholder.

This app was the `exo-gui` repository. `exo-gui` is still its app id in Auth,
its Go module path, and its Compose project name `biotron-exo-gui`. It reports
to Logger as `exo-api`. The code is MIT licensed; see `LICENSE`.

## Layout

| Path | What |
|------|------|
| `frontend/` | Vite and React dashboard, Tailwind 4. Its container serves the built files with Nginx. |
| `backend/` | Go Fiber service. `/health` and the authorization gate; nothing else yet. |
| `prisma/` | Postgres schema and migrations. No models yet. |
| `scripts/` | `mock_telemetry.sh`, a stand-in for the ESP32 that posts batched telemetry. See Scripts below. |
| `docker-compose.yml` | `exo-migrate`, `exo-api`, and `exo-web` on the shared `biotron` network. |
| `.env.example` | Every variable Compose, the backend, the frontend, and Prisma read. Copy it to `.env`. |

## Quick start

The shared Postgres and Redis live in `infra/`. Start them once, from the
monorepo root:

```bash
cp infra/.env.example infra/.env
# Replace the Postgres password, then:
docker compose -f infra/docker-compose.yml --env-file infra/.env up -d
```

That creates the external `biotron` network every app joins. Exo also needs
Auth running: the browser signs in against Auth on port 8080, and the API
reaches Auth at `OAUTH_MANAGER_URL` on the `biotron` network. Then:

```bash
cp apps/exo/.env.example apps/exo/.env
# Use the same Postgres password.
docker compose up -d --build exo-migrate exo-api exo-web
```

The same `docker compose up -d --build` works alone from `apps/exo/`. The
root compose file includes this app's file and reads `apps/exo/.env` for it.
`exo-api` waits for `exo-migrate` to finish; `exo-web` waits for `exo-api`.

- UI: http://localhost:5174
- API: http://localhost:8081/health, answers `{"status":"ok"}`

Host ports are 5174 for the web container and 8081 for the API. Auth keeps
5173 and 8080, where its GitHub callback is registered.

## Develop

Open the monorepo in its devcontainer. It has Node 24 and Go 1.27, and it
starts a Postgres 18 and a Redis 8 beside the workspace, named `postgres` and
`redis`, with user `biotron`, password `change-me`, and database `biotron`.
`.env.example` already points at them, with `?schema=exo`. The post-create
step runs `npm ci` and `go work sync`, copies `.env` from the example where
none exists, and applies this app's migrations. After that:

```bash
npm run dev -w apps/exo/frontend               # http://localhost:5174
cd apps/exo/backend && PORT=8081 go run .      # http://localhost:8081/health, reads ../.env
```

The backend defaults to port 8080, which Auth uses, so pass `PORT=8081`.
Vite binds all interfaces (`server.host: true`) on port 5174, so the port is
reachable through the container's forwarded port.

The root `package.json` owns the workspaces and the toolchain. From the root:

```bash
npm ci                                 # every frontend and packages/style
npm run build -w apps/exo/frontend     # tsc -b, then vite build
npm run lint                           # Oxlint, type-aware, .oxlintrc.json
go work sync                           # after changing any go.mod
```

`frontend/package.json` lists only `@biotron/style`, `react`, and
`react-dom`. Vite, TypeScript, Tailwind, and the type packages come from the
root. There is one root `package-lock.json`; `prisma/` keeps its own because
it is not a workspace.

### Environment

This app has one environment file, `apps/exo/.env`. Compose, the backend
(through `godotenv`), the frontend (Vite `envDir: '..'`), and Prisma all read
it. Do not create component-level environment files.

Backend variables: `PORT`, `FRONTEND_URL` (the one CORS origin),
`TRUSTED_PROXIES`, and `OAUTH_MANAGER_URL` (default
`http://oauth-manager:8080`). If `OAUTH_MANAGER_URL` is unset or unreachable,
every gated route answers 503 rather than opening. `TRUSTED_PROXIES` must
include the edge Nginx's network, because Fiber reads `Cf-Connecting-Ip`
only from a trusted proxy. Compose also passes `DATABASE_URL` to `exo-api`,
but the backend does not read it yet.

Frontend variables: `VITE_API_URL` (default `http://localhost:8081`) and
`VITE_AUTH_URL` (default `http://localhost:8080`). Both are compiled into the
bundle, and a production build fails if either is unset. Compose passes them
as build arguments to `exo-web`.

Logger: the API sends lifecycle events (`Exo API started`, `Exo API
stopping`) and completed requests (method, path, status, duration) to Logger
as `exo-api`. `/health` is never logged. `LOGGER_URL` defaults to
`http://logger-api:8080`. Set `LOGGER_INGEST_TOKEN` to the same shared secret
Logger uses; leaving it empty disables delivery without stopping Exo.
`LOG_LEVEL` sets the minimum level sent. `backend/internal/eventlog` is a
copy of `go/logclient` and should import the shared module instead.

## Authorization

Auth owns identity and per-product permissions. Exo's app id is `exo-gui`,
and it declares three permissions:

| Permission   | What it opens              | Daily guest key |
|--------------|----------------------------|-----------------|
| `live`       | live telemetry             | yes             |
| `historical` | recorded sessions          | yes             |
| `commands`   | commands sent to the rig   | **no**          |

`backend/internal/auth` is the single enforcement point. It asks
`GET {OAUTH_MANAGER_URL}/v1/check?app=exo-gui&permission=<key>`, forwarding
the caller's session cookie, and maps the answer to a status:

| Situation                                        | Exo responds |
|--------------------------------------------------|--------------|
| no cookie, or Auth answers 401                    | `401` |
| `allowed: false`, or Auth answers 403             | `403` |
| Auth unreachable, unexpected status, or `OAUTH_MANAGER_URL` unset | `503` |
| `allowed: true`                                   | handler runs |

401 and 403 are deliberately distinct: a signed-in operator who lacks a
permission must not be sent back to the sign-in button that already worked.
Every failure mode is a denial. There is no configuration in which the gate
opens.

### Wiring a new route

No data or command route exists yet. When the first one lands, mount it on a
group that is already gated, never by adding the check inside a handler; that
way a second route on the same group cannot forget it. `server.go` carries
this example:

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

`requireXHR` is not written yet; Auth's `RequireXHR` is the model.
`auth.DecisionFrom(c)` returns the decision the gate already made, so a
handler never needs a second round trip. The permission keys live in
`backend/internal/auth` as `auth.PermissionLive`, `auth.PermissionHistorical`,
and `auth.PermissionCommands`. Use the constants, not string literals.

## Backend

Go Fiber v3. This process does not issue sessions; Auth does. CORS allows
only `FRONTEND_URL`, with `GET`, `POST`, and `OPTIONS`.

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/health` | Liveness, `{"status":"ok"}`. Public: the container healthcheck and Logger's monitor poll it. |

Guest sign-in uses the product id `exo-gui`; Auth binds the daily key and the
resulting guest session to that product. Daily-key guests receive `live` and
`historical` but never `commands`. Auth enforces that inside `/v1/check`, and
the gate above is what makes Exo ask.

## Frontend

Vite and React, Tailwind 4 (imported in `src/index.css`, with the shared dark
palette as `@theme` tokens), and the shared components from `@biotron/style`:
`Brand`, `ThemeToggle`, `UserMenu`, and `AuthScreen`. The Vite config adds
`biotronFavicon()` from `@biotron/style/vite`.

The top bar leads with the 112px wordmark, then the machine selector (one
placeholder machine, `TestDummyExo`), a Live / Historical toggle in the
centre, and the theme toggle and user menu on the right. The main area shows
the chosen mode and machine and nothing else yet.

Sign-in is the shared `AuthScreen` with a GitHub button and a guest key
field. GitHub sign-in redirects to `{VITE_AUTH_URL}/auth/github/login` with
this origin as `redirect`. The guest field posts `app_id` and `key` to
`/auth/guest`. The session is checked with `/auth/me?app=exo-gui` on load,
on focus, and every ten minutes, so a guest session that expires at Eastern
midnight is noticed without a reload. A network error never signs anyone
out; only a 401 or 403 does. Logout sends `X-Requested-With`, which Auth
requires.

Scripts: `dev`, `build` (`tsc -b && vite build`), and `preview`.

## Database

Prisma 6.19.3, pinned exactly. `prisma/package.json` also overrides the
transitive dependency `deepmerge-ts` to 8.0.0; `@prisma/config` asks for
7.1.5. The `prisma-client-js` generator is a placeholder; no code uses it.
The backend has no database code at all yet, and the query layer for the
telemetry tables is still to be chosen.

`schema.prisma` declares no models. Three migrations exist: two created local
operator, session, and guest-key tables, and `20260829200000_drop_local_auth`
dropped them because Auth owns that. Applying all three leaves the `exo`
schema with no tables of its own. The telemetry schema lands with ingest.

Working with migrations in the devcontainer:

```bash
docker compose run --rm exo-migrate             # apply, as the container does
set -a && . apps/exo/.env && set +a             # Prisma reads DATABASE_URL from the environment
npm run --prefix apps/exo/prisma migrate        # prisma migrate dev: create and apply
npm run --prefix apps/exo/prisma studio
```

The Prisma image installs from its own lockfile and runs `npm run deploy` as
the image's `node` user.

## Scripts

`scripts/mock_telemetry.sh` stands in for the ESP32 WiFi coprocessor. It
needs only `bash`, `awk`, and `curl`. Each POST is one batch from one machine
in the agreed wire shape: a `machine_id` and a list of samples, each with a
microsecond `sampled_at`, a per-run `seq` counter, battery voltage and
current, left and right temperature, position, velocity, torque, and error,
and MCU and link status. Values are jittered around plausible baselines.

```bash
./scripts/mock_telemetry.sh
INGEST_URL=http://localhost:8081/api/telemetry RATE_HZ=5 ./scripts/mock_telemetry.sh
MACHINE_ID=exo-002 RATE_HZ=2 BATCH=20 COUNT=50 ./scripts/mock_telemetry.sh
```

| Variable | Default | Meaning |
|---|---|---|
| `MACHINE_ID` | `exo-001` | the batch's `machine_id` |
| `INGEST_URL` | `http://localhost:8080/api/telemetry` | where to POST |
| `RATE_HZ` | `1` | batches per second |
| `BATCH` | `10` | samples per batch |
| `COUNT` | `0` | stop after this many batches; `0` runs until Ctrl-C |

Two things to know. The API does not serve `/api/telemetry` yet; the ingest
route is still a comment in `backend/internal/server/server.go`, so today the
script only shows the wire shape and reports whatever status the server
returns. And its default port, 8080, is Auth's host port in this repository;
the Exo API is published on 8081, so pass `INGEST_URL` explicitly.

## CI

`.github/workflows/ci.yml` runs when `apps/exo/**` changes: the frontend job
builds `packages/style` and this app and runs Oxlint on `frontend/src`; the
backend job runs golangci-lint, `go vet`, and `go test`. The workflow has not
run yet.
