# Logger

Logger is the platform's public status page and its log warehouse. The
status page needs no account. It shows one coarse state per application
and per component, `operational`, `degraded`, `down`, or `unknown`, with
uptime over 24 hours, 7 days, and 90 days. The log explorer behind it
needs Auth's `logger/view` permission.

## Layout

| Path | What |
|------|------|
| `backend/` | Go Fiber API: ingest, queries, the health monitor, the public status routes. Postgres through pgx; Redis for the recent tail and the latest health. |
| `frontend/` | Vite and React status page and log explorer, served by Nginx. |
| `prisma/` | Schema and migrations. The Go service runs its own queries. |
| `docker-compose.yml` | `logger-migrate`, `logger-api`, `logger-web` on the `biotron` network. |
| `.env.example` | Every variable the app reads. Copy it to `.env`. |

The Go client other services use to send events is `go/logclient`, not a
folder here.

## Run

Start the shared Postgres and Redis once, from the monorepo root, then Logger:

```bash
docker compose -f infra/docker-compose.yml --env-file infra/.env up -d
cp apps/logger/.env.example apps/logger/.env    # set a long random LOGGER_INGEST_TOKEN
docker compose up -d --build logger-migrate logger-api logger-web
```

The status page is at http://localhost:5175 and renders without a sign-in.
The API listens on http://127.0.0.1:8082; the Nginx inside `logger-web`
proxies `/api/` to it, as the edge does in staging and production.

## Develop

Open the monorepo in its devcontainer. It supplies Node 24, Go 1.27, and a
Postgres and Redis that `.env.example` already points at. The post-create
step installs the workspaces and applies this app's migrations. Then:

```bash
npm run dev -w apps/logger/frontend    # http://localhost:5175, proxies /api to :8082
cd apps/logger/backend && set -a && . ../.env && set +a && PORT=8082 go run .
npm run lint                           # Oxlint, from the root
```

The backend reads the process environment only, never `.env`, so export the
file first. Set `OAUTH_MANAGER_URL` to `http://localhost:8080`, where the
Auth API runs in the devcontainer.

`AUTH_DISABLED=true` makes every log line readable without a session on a
service the edge publishes to the internet, so it takes two deliberate
steps. The backend refuses to start unless `BIOTRON_ENV=development`
accompanies it, and neither variable appears in `docker-compose.yml` or
`.env.example`, so a stray value in a shell reaches nothing. Opt in from
`apps/logger`:

```bash
docker compose -f docker-compose.yml -f docker-compose.dev.yml up
```

## Environment

One file, `apps/logger/.env`, feeds Compose, the backend, the frontend, and
Prisma. Do not add environment files below it.

- `LOGGER_INGEST_TOKEN` is required. Every sender uses the same token.
- `HEALTH_INTERVAL` (15s) is the probe cadence; `HEALTH_TIMEOUT` (3s) bounds
  one probe. A reading older than three intervals shows as `unknown`.
  `HEALTH_HISTORY_INTERVAL` (5m) is the heartbeat: Postgres takes a row on
  a state change or when a heartbeat is due, and a gap longer than three
  heartbeats counts as time nobody was watching.
- `STATUS_CACHE_TTL` (30s) and `STATUS_RATE_LIMIT` (60 per minute per
  address) guard the public routes, which query ninety days for anyone.
- `INGEST_RATE_LIMIT` (600 per minute per sender address) bounds what a
  leaked token can write. It runs before the token check, so a wrong-token
  flood pays too.
- `TRUSTED_PROXIES` must include the edge Nginx network
  (`127.0.0.1,::1,172.16.0.0/12` in Compose). Every per-address limit keys
  on `Cf-Connecting-Ip`, which Fiber believes only from a listed peer.
  Widen the list past the edge and a client can pick its own bucket.

## API

| Method | Path | Who | Notes |
|--------|------|-----|-------|
| GET | `/health` | anyone | pings Postgres and Redis; `503` when either fails |
| GET | `/v1/status` | anyone | overall, per-application, and per-component state with 24h, 7d, and 90d uptime |
| GET | `/v1/status/history?days=90` | anyone | one bucket per `America/Toronto` day, per application and per component; `days` is clamped to 1..90 |
| GET | `/v1/session` | anyone | always `200` with `{authenticated, allowed}`; `Cache-Control: no-store` |
| POST | `/v1/logs` | ingest token | stores one event for a catalog component id; `201` with the row |
| GET | `/v1/apps` | `logger/view` | state per component with health detail and `checked_at` |
| GET | `/v1/apps/:app/logs/recent` | `logger/view` | the Redis tail; `levels`, `q`, `limit` (1 to 200, default 100) |
| GET | `/v1/apps/:app/logs/history` | `logger/view` | Postgres; adds RFC3339 `from` and `to`, and `cursor` from `next_cursor` |

`/v1/status` and `/v1/status/history` send `Cache-Control: public,
max-age=30` and share one rate limit with `/v1/session`. A health detail
string holds the hostname, the port, and the raw dial error of a probe.
Together those map the internal network, so the public routes drop it.

`/v1/session` never answers `401` or `403`: the page has to tell "signed
out" from "signed in without `logger/view`", and a status code collapses
the two. When Auth is unreachable it reports a signed-out visitor. The
`logger/view` routes answer `401` with no session, `403` without the
permission, and `503` when Auth is unreachable.

## The status page

The page is one column: a banner with the overall state, then one **System
status** card with one row per application. A collapsed row is a name, an
"i" that opens the description, a 90-day figure, and a 90-day strip. The
**N components** button unfolds the components in place of the strip, each
with its own figure and strip. The strip is a button: point at a day, or
focus it and press the arrow keys, and a popover names the day and what
happened on it.

**View history** leads to `/history`, the days that had incidents by
month, derived from the same daily series; Logger keeps no incident
record. With `logger/view`, an unfolded row gains an **Open logs** link to
`/applications/<id>`: health detail per component, and the recent tail or
the Postgres history with level, text, and time filters. An operator
without the permission sees a notice that says to ask a manager.

## The catalog

`backend/internal/catalog/default.json` lists each application and its
components. Every component names how it is probed:

| `check` | What the monitor does | `health_url` |
|---|---|---|
| `http` (default) | `GET health_url`; 2xx or 3xx is healthy | required |
| `postgres` | pings the shared database through Logger's own pool | refused |
| `redis` | pings the shared cache through Logger's own client | refused |

The catalog ends with an **Infrastructure** application: the database, the
cache, and the edge proxy at `http://edge-proxy:8080/_edge/health`. When
several applications go red at once, that row says whether the cause is
shared. The database and cache are pinged, not fetched, because a URL for
them would carry credentials. The devcontainer runs no edge, so there that
row reads down; `LOGGER_CATALOG_JSON` can name a catalog without it.

## How uptime is computed

`health_checks` rows are sparse: one on a state change or once per
heartbeat, not one per poll. Uptime is therefore time-weighted, never
`count(ok)/count(*)`, which would over-weight flapping periods. Each row's
state holds until the next row's time; the last row holds until the end of
the window; the first segment is clipped to the window start.

A stretch longer than three heartbeats means nobody was watching, usually
because Logger itself was down. It counts as unknown and leaves both the
numerator and the denominator. A window with no data reports `null`, not
`100`. A window with any downtime is clamped to `99.99`, so a green 100% is
never a lie. An application adds up its components' durations rather than
averaging their percentages. Days are `America/Toronto` calendar days. The
maths is in `backend/internal/uptime` and is tested without a database.
The history query collapses runs of identical rows in SQL and marks each
returned row `continuous`, so a collapsed run is never read as a silence.

## Application logging

Auth, Exo, and Calendar send events through the shared Go client in
`go/logclient`, reporting as `oauth-manager`, `exo-api`, and
`calendar-api`. The client reads `LOGGER_URL` (default
`http://logger-api:8080`), `LOGGER_INGEST_TOKEN`, and `LOG_LEVEL`
(default `info`), and sends nothing when the token is empty.

Logger writes its own events straight into the store as `logger-api`.
Posting them to `/v1/logs` would make the request log record the logging,
one more event for every event.

| Message | Level | Payload |
|---------|-------|---------|
| `Logger API started` | info | `port` |
| `Read authentication disabled` | warning | — |
| `HTTP request completed` | info; warning on 4xx; error on 5xx | `method`, `path`, `status`, `duration_ms`, and `error` when the request failed |
| `Component down` | warning | `component`, and `detail` from the probe |
| `Component recovered` | info | `component`, `detail`, `down_for_s` |
| `Authorization service unavailable` | error | `error` from the call to Auth |
| `Logger API stopping` | info | — |

The request log stays quiet on `/v1/status`, `/v1/status/history`,
`/v1/session`, and `/v1/logs`. The first three are what the status page
polls; the last carries every event the platform sends, and a row for the
request that carried a row says nothing. All four still speak when they
fail, so a `400` on `/v1/logs` names the service sending what Logger
cannot store. Requests refused with `429`, and ingest refused with `401`,
record nothing: both come from the internet, and one row per attempt would
let an attacker write into the audit trail at the rate limit.

The monitor reports a component only when its state flips, so a poll that
finds nothing changed is silent, and a component already healthy at
startup says nothing at all.

A store failure is printed on stdout and nowhere else, because the event
reporting it would go to the store that just refused one. `docker logs
logger-api` is where a dead database, a dead cache, and a dropped event
appear.

## Storage

Postgres is authoritative. After inserting a row, the backend pushes it onto
a Redis list per service capped at `REDIS_TAIL_SIZE` (500). A failed push
is logged and the insert stands, so a client retry cannot duplicate the
row. Recent reads come from Redis and fall back to Postgres when the tail
is empty. The monitor writes the latest health to Redis on every poll.

Prisma 6.19.3, pinned exactly. Two tables, `logs` and `health_checks`, in
the schema named by `?schema=` in `DATABASE_URL` (`logger`). To create a
migration in the devcontainer, source `apps/logger/.env` and run
`npm run --prefix apps/logger/prisma migrate`. The `logger-migrate` service
applies the migrations as the image's `node` user.

## Production authentication

Logger forwards the browser's cookie to Auth's
`GET /v1/check?app=logger&permission=view`. Auth must hold a `logger` tool
with a `view` permission, which its migrations seed, and must issue its
cookie for the parent domain. The edge serves Logger at
`logs.<BASE_DOMAIN>`, so on staging `COOKIE_DOMAIN` is `biotron-dev.com`.
Production uses its own domain.
