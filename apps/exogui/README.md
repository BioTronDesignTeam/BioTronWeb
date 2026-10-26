# Exo

Exo is the operator UI and the telemetry API for the exoskeleton. Today the
API serves `/health`, `/v1/auth/status`, and the gated prefixes. The live, historical, and
command routes are a plan, not code. The machine list in the UI is a
placeholder. The hardware side, an STM32 that reads the sensors and an ESP32
that posts telemetry, is not in this repository.

This app was the `exo-gui` repository. `exo-gui` is still its app id in Auth,
its Go module path, and its Compose project name `biotron-exo-gui`. It reports
its events as `exogui-api`. The code is MIT licensed; see `LICENSE`.

## Layout

| Path | What |
|------|------|
| `backend/` | Go `net/http` API. `/health` and the permission gate. No database code yet. |
| `frontend/` | Vite and React UI, served by Nginx. |
| `docker-compose.yml` | `exogui-api` and `exogui-web` on the `biotron` network. |
| `.env.example` | Every variable the app reads. Copy it to `.env`. |

## Run

Exo needs a running Auth, which needs the shared Postgres from `infra/`.
Start them first, from the monorepo root (see `apps/auth/README.md`), then Exo:

```bash
docker compose -f infra/docker-compose.yml --env-file infra/.env up -d
docker compose up -d --build auth-migrate auth-api auth-web
cp apps/exogui/.env.example apps/exogui/.env
docker compose up -d --build exogui-api exogui-web
```

The UI is at http://localhost:5174 and the API at http://localhost:8081. The
browser signs in against Auth on port 8080. The API reaches Auth at
`OAUTH_MANAGER_URL` over the `biotron` network.

## Develop

Open the monorepo in its devcontainer. It supplies Node 24 and Go 1.27, and
its post-create step installs the workspaces. Then:

```bash
npm run dev -w apps/exogui/frontend          # http://localhost:5174
cd apps/exogui/backend && PORT=8081 go run . # http://localhost:8081, reads ../.env
npm run lint                                 # Oxlint, from the root
```

The backend defaults to port 8080, which Auth uses, so pass `PORT=8081`.

## Environment

One file, `apps/exogui/.env`, feeds Compose, the backend, and the frontend. Do not add environment files below it. The backend reads `PORT`,
`FRONTEND_URL`, and `OAUTH_MANAGER_URL` in
`backend/internal/config`. The shared `go/logclient` reads `LOG_LEVEL`
and the optional Grafana and PostHog variables listed in
`go/logclient/README.md`.

- `FRONTEND_URL` is the one CORS origin.
- `OAUTH_MANAGER_URL` unset or unreachable: every gated route answers `503`.

`VITE_API_URL` and `VITE_AUTH_URL` are compiled into the frontend bundle;
Compose passes them to `exogui-web` as build arguments. A production build fails
if either is unset. Nothing in the UI calls the API yet.

## Authorization

Auth owns identity and permissions. Exo's app id is `exo-gui`. It declares
three permissions: `live`, `historical`, and `commands`. A daily guest key
opens `live` and `historical` on Exo and never `commands`. Auth enforces that;
Exo holds it by asking on every request.

The gate is `go/authclient`. Each permission has its own route prefix, and
`Require(permission)` wraps the whole prefix, so a route added under it
cannot forget the check:

| Prefix | Permission | Who passes |
|---|---|---|
| `/v1/live/` | `live` | granted members, managers, superusers, daily guests |
| `/v1/historical/` | `historical` | granted members, managers, superusers, daily guests |
| `/v1/commands/` | `commands` | granted members, managers, superusers; never a guest |

A signed-in member with no Exo GUI permission is refused by all three.
Each request asks `GET {OAUTH_MANAGER_URL}/v1/session?app=exo-gui&permission=<key>`
with the caller's cookie:

| Situation | Exo answers |
|---|---|
| Mutating request without `X-Requested-With: XMLHttpRequest` | `403`, Auth not asked |
| No cookie, or Auth refuses the session | `401` |
| Signed in without the permission | `403` with `{"error":"permission required","permission":"<key>"}` |
| Auth unreachable, unexpected status, or `OAUTH_MANAGER_URL` unset | `503` |
| Signed in with the permission | the handler runs |

A request with no cookie never reaches Auth. `401` and `403` stay distinct,
so a signed-in operator is never sent back to a sign-in button that already
worked. A handler reads the operator with `authclient.OperatorFrom(r)`, and
the request event names them as `actor`.

`GET /v1/auth/status` tells the UI who is signed in and what they hold:
`{"operator": {...}, "can_live": true, "can_historical": true,
"can_commands": false, "permissions": {...}}`, with `operator` left out when
no one is signed in.

No data or command route exists yet. Mount each in the mux for its prefix in
`server.go`. Use the constants in `backend/internal/auth`, not string
literals.

## Backend

Go's standard `net/http`, with no framework. Exo issues no sessions; Auth
does. CORS allows `FRONTEND_URL` only, with `GET`, `POST`, and `OPTIONS`.
`GET /health` answers `{"status":"ok"}` to anyone. Status's
monitor polls it. Routes match exactly: `/health/` and `/HEALTH` are `404`.

Exo reports to Grafana and PostHog as `exogui-api`. `/health` is never sent.

| Message | Level | Payload |
|---|---|---|
| Exo API started | Info | `port` |
| HTTP request completed | Info; Warning on 4xx; Error on 5xx | `method`, `path`, `status`, `duration_ms`, `actor` once the gate let them through, and `error` when it failed |
| Authorization service unavailable | Error | `permission`, `error` |
| Authorization not configured | Warning | |
| Exo API stopping | Info | |

## Frontend

The session, sign-in screen, and account menu come from `@biotron/style`.
The sign-in card is titled "Exo GUI" and offers Sign in with GitHub and the
Daily guest key. GitHub sign-in redirects to
`{VITE_AUTH_URL}/auth/github/login` with this origin as `redirect`. The
guest field posts `app_id` and `key` to `/auth/guest`. A return with
`?auth=denied` or `?auth=banned` shows the matching notice, and the query
is removed from the address.

`SessionProvider` checks the session with `/auth/me?app=exo-gui`, then asks
`/v1/check` for `live`, `historical`, and `commands`. It re-checks on load,
when the tab regains focus or becomes visible, and every ten minutes, so a
guest session that ends at Eastern midnight is noticed without a reload, and
then says the session expired. Only a `401` or `403` signs an operator out;
a network error never drops a signed-in session, and an Auth outage shows a
retrying "can't reach sign-in" screen instead of the sign-in card. Sign out
sends `X-Requested-With`, which Auth requires.

A signed-in member with none of the three permissions sees Access denied,
not the app. Guests hold `live` and `historical`, so they get in.

The top bar leads with the wordmark from `@biotron/style` and, on wide
screens, the "Exo GUI" label, then the machine
selector, the Live / Historical toggle in the centre, and the theme toggle
and account menu on the right. The machine list is one placeholder,
`TestDummyExo`, hard-coded in `App.tsx`. The main area names the chosen mode
and machine and nothing else.

## Database

Exo has none yet. Its Prisma schema held only a local copy of Auth's
tables, which a later migration dropped, so the `exo-migrate` image and
`DATABASE_URL` went too. A deployment migrated before then keeps an empty
`exo` schema with Prisma's `_prisma_migrations` table; it is harmless, and
`DROP SCHEMA exo CASCADE` removes it. The telemetry tables bring back a
schema and a migrate service when ingest lands.

## Telemetry wire shape

The planned ingest takes one POST per batch from one machine. Nothing serves
it yet. A batch is a `machine_id` and its samples, about 10 ms apart. Each
sample has a microsecond `sampled_at`, a per-run `seq`, the battery, both
legs, and two status strings:

```json
{
  "machine_id": "exo-001",
  "samples": [
    {
      "sampled_at": 1767225600000000,
      "seq": 0,
      "battery": { "voltage": 48.1, "current": 3.0 },
      "left": { "temp": 41.0, "position": 1.2, "velocity": 0.0, "torque": 2.0, "error": 0 },
      "right": { "temp": 41.0, "position": 1.2, "velocity": 0.0, "torque": 2.0, "error": 0 },
      "mcu_status": "ok",
      "link_status": "ok"
    }
  ]
}
```
