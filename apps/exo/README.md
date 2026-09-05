# Exo

Exo is the operator UI and the telemetry API for the exoskeleton. Today the
API serves `/health` and the permission gate. The live, historical, and
command routes are a plan, not code. The machine list in the UI is a
placeholder. The hardware side, an STM32 that reads the sensors and an ESP32
that posts telemetry, is not in this repository.

This app was the `exo-gui` repository. `exo-gui` is still its app id in Auth,
its Go module path, and its Compose project name `biotron-exo-gui`. It reports
to Logger as `exo-api`. The code is MIT licensed; see `LICENSE`.

## Layout

| Path | What |
|------|------|
| `backend/` | Go Fiber API. `/health` and the permission gate. No database code yet. |
| `frontend/` | Vite and React UI, served by Nginx. |
| `prisma/` | Schema and migrations. No models yet. |
| `scripts/` | `mock_telemetry.sh`, a stand-in for the ESP32. See Scripts. |
| `docker-compose.yml` | `exo-migrate`, `exo-api`, `exo-web` on the `biotron` network. |
| `.env.example` | Every variable the app reads. Copy it to `.env`. |

## Run

Exo needs the shared Postgres and Redis from `infra/` and a running Auth.
Start them first, from the monorepo root (see `apps/auth/README.md`), then Exo:

```bash
docker compose -f infra/docker-compose.yml --env-file infra/.env up -d
docker compose up -d --build auth-migrate auth-api auth-web
cp apps/exo/.env.example apps/exo/.env    # use the same Postgres password
docker compose up -d --build exo-migrate exo-api exo-web
```

The UI is at http://localhost:5174 and the API at http://localhost:8081. The
browser signs in against Auth on port 8080. The API reaches Auth at
`OAUTH_MANAGER_URL` over the `biotron` network.

## Develop

Open the monorepo in its devcontainer. It supplies Node 24, Go 1.27, and a
Postgres and Redis that `.env.example` already points at. The post-create
step installs the workspaces and applies this app's migrations. Then:

```bash
npm run dev -w apps/exo/frontend             # http://localhost:5174
cd apps/exo/backend && PORT=8081 go run .    # http://localhost:8081, reads ../.env
npm run lint                                 # Oxlint, from the root
```

The backend defaults to port 8080, which Auth uses, so pass `PORT=8081`.

## Environment

One file, `apps/exo/.env`, feeds Compose, the backend, the frontend, and
Prisma. Do not add environment files below it. The Go code reads seven
variables: `PORT`, `FRONTEND_URL`, `TRUSTED_PROXIES`, and `OAUTH_MANAGER_URL`
in `backend/internal/config`; `LOGGER_URL`, `LOGGER_INGEST_TOKEN`, and
`LOG_LEVEL` in `backend/internal/eventlog`.

- `FRONTEND_URL` is the one CORS origin.
- `TRUSTED_PROXIES` must include the edge Nginx network. Fiber reads
  `Cf-Connecting-Ip` only from a trusted proxy.
- `OAUTH_MANAGER_URL` unset or unreachable: every gated route answers `503`.
- `LOGGER_INGEST_TOKEN` must match Logger's. Leave it empty to send nothing.
- `DATABASE_URL` reaches `exo-migrate` and `exo-api`. Only Prisma reads it.

`VITE_API_URL` and `VITE_AUTH_URL` are compiled into the frontend bundle;
Compose passes them to `exo-web` as build arguments. A production build fails
if either is unset. Nothing in the UI calls the API yet.

## Authorization

Auth owns identity and permissions. Exo's app id is `exo-gui`. It declares
three permissions: `live`, `historical`, and `commands`. A daily guest key
opens `live` and `historical` on Exo and never `commands`. Auth enforces that
inside `/v1/check`; Exo enforces it by asking.

`backend/internal/auth` is the one gate. `Require(permission)` is middleware
for a route group. It forwards the caller's cookie to
`GET {OAUTH_MANAGER_URL}/v1/check?app=exo-gui&permission=<key>` and maps the
answer to a status:

| Situation | Exo answers |
|---|---|
| No cookie, or Auth answers `401` | `401` |
| Auth answers `{"allowed": false}` or `403` | `403` |
| Auth unreachable, unexpected status, or `OAUTH_MANAGER_URL` unset | `503` |
| `{"allowed": true}` | the handler runs |

A request with no cookie never reaches Auth. `401` and `403` stay distinct,
so a signed-in operator is never sent back to a sign-in button that already
worked. A handler reads the decision with `auth.DecisionFrom(c)`.

No route uses the gate yet. `server.go` shows the intended wiring in a
comment: mount each route on a gated group, never check inside a handler, so
a second route on the group cannot forget it. Use the constants in
`backend/internal/auth`, not string literals.

## Backend

Go Fiber v3. Exo issues no sessions; Auth does. CORS allows `FRONTEND_URL`
only, with `GET`, `POST`, and `OPTIONS`. One route exists: `GET /health`
answers `{"status":"ok"}` to anyone. Logger's monitor polls it.

Exo reports to Logger as `exo-api`: start and stop, and each completed
request as method, path, status, and duration. `/health` is never sent.
`backend/internal/eventlog` is a copy of `go/logclient`; it should import
the shared module instead.

## Frontend

Sign-in is the shared `AuthScreen` with a GitHub button and a guest key
field. GitHub sign-in redirects to `{VITE_AUTH_URL}/auth/github/login` with
this origin as `redirect`. The guest field posts `app_id` and `key` to
`/auth/guest`. A return with `?auth=denied` shows the non-member notice.

`AuthProvider` checks the session with `/auth/me?app=exo-gui` on load, when
the tab regains focus or becomes visible, and every ten minutes, so a guest
session that ends at Eastern midnight is noticed without a reload. Only a
`401` or `403` signs an operator out; a network error never drops a signed-in
session. Logout sends `X-Requested-With`, which Auth requires.

The top bar leads with the wordmark from `@biotron/style`, then the machine
selector, the Live / Historical toggle in the centre, and the theme toggle
and user menu on the right. The machine list is one placeholder,
`TestDummyExo`, hard-coded in `App.tsx`. The main area names the chosen mode
and machine and nothing else.

## Database

Prisma 6.19.3, pinned exactly. `prisma/package.json` overrides `deepmerge-ts`
to `8.0.0`; `@prisma/config` asks for `7.1.5`. `schema.prisma` declares no
models. Three migrations exist: two created operator, session, and guest-key
tables, and `20260829200000_drop_local_auth` dropped them because Auth owns
that. After all three, the `exo` schema holds no tables. The telemetry tables
land with ingest, and the Go query layer is still to be chosen.

To create a migration in the devcontainer, source `apps/exo/.env` and run
`npm run --prefix apps/exo/prisma migrate`. The `exo-migrate` service applies
the migrations as the image's `node` user.

## Scripts

`scripts/mock_telemetry.sh` stands in for the ESP32. It needs `bash`, `awk`,
`curl`, and GNU `date`; macOS `date` cannot print microseconds. Each POST is
one batch from one machine: a `machine_id` and samples 10 ms apart, each with
a microsecond `sampled_at`, a per-run `seq`, battery voltage and current,
temperature, position, velocity, torque, and error for `left` and `right`,
and `mcu_status` and `link_status`.

| Variable | Default | Meaning |
|---|---|---|
| `MACHINE_ID` | `exo-001` | the batch's `machine_id` |
| `INGEST_URL` | `http://localhost:8080/api/telemetry` | where to POST |
| `RATE_HZ` | `1` | batches per second |
| `BATCH` | `10` | samples per batch |
| `COUNT` | `0` | stop after this many batches; `0` runs until Ctrl-C |

Two notes. The API does not serve `/api/telemetry`; the script shows the
wire shape and prints whatever status the server returns. And the default
port, 8080, is Auth's host port here; the Exo API is on 8081, so run it as
`INGEST_URL=http://localhost:8081/api/telemetry ./scripts/mock_telemetry.sh`.
