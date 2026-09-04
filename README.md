# Logger

BioTron's public platform status page, structured log warehouse, and
authenticated log explorer.

## What it does

Logger has two layers. The **public status page** needs no account at all;
the **log explorer** behind it needs OAuthManager's `logger/view` permission.

Public, no authentication:

- Polls every application component from inside the `biotron` Docker network.
- Keeps the latest health state per component in Redis.
- Records health transitions and periodic snapshots in Postgres.
- Publishes overall, per-application, and per-component status, uptime over the
  last 24 hours, 7 days, and 90 days, and a 90-day daily history bar.
- Exposes only a coarse `operational` / `degraded` / `down` / `unknown` state.
  Health detail strings and catalog health URLs stay internal, because they
  embed hostnames, ports, and raw dial errors.

Behind `logger/view`:

- Accepts structured `debug`, `info`, `warning`, and `error` events.
- Keeps a capped recent tail per service in Redis while Postgres remains the
  durable log warehouse.
- Provides recent and historical application log views with level, text, and
  time filters.

## How uptime is computed

`health_checks` rows are sparse and unevenly spaced: the monitor writes a row on
a state change or once per `HEALTH_HISTORY_INTERVAL`, not on every poll. Uptime
is therefore **time-weighted**, never `count(ok)/count(*)`, which would
over-weight flapping periods where transitions cluster densely.

Each row's state holds from its `checked_at` until the next row's, the last row
holds until the end of the window, and the first segment is clipped to the
window start. A stretch longer than `3 × HEALTH_HISTORY_INTERVAL` means nobody
was watching — usually Logger itself was down — so it is counted as **unknown**
and excluded from both the numerator and the denominator rather than silently
inventing uptime. A window with no data reports `null`, not `100`, and a window
containing any downtime is clamped to `99.99` so a green `100%` is never a lie.

Daily buckets are `America/Toronto` calendar days, matching the rest of the
platform. The maths lives in `backend/internal/uptime` and is unit-tested
without a database.

## Layout

| Path | What |
|---|---|
| `frontend/` | React + TypeScript public status page and log explorer |
| `backend/` | Go/Fiber ingest, query, health-monitor, cache, and authorization API |
| `client/` | Reusable Go logging client with source-side `LOG_LEVEL` filtering |
| `prisma/` | Logger-owned Postgres schema and migrations |
| `.devcontainer/` | Node 22 + Go 1.23 development environment |
| `docker-compose.yml` | Logger containers on the shared `biotron` network |

## Run locally

Start the shared Postgres and Redis containers from `../Server`, then:

```bash
cp .env.example .env
docker compose up --build
```

The backend, frontend, Prisma, and Compose all use the single root `.env`; do
not create component-level environment files.

Set a long random `LOGGER_INGEST_TOKEN` in `.env`. OAuthManager is required by
default. `AUTH_DISABLED=true` is available for isolated local UI work only and
must never be used in staging or production.

The status page is available on `http://localhost:5175` and renders fully
without signing in; the API is bound to `http://127.0.0.1:8082`.

## API

| Method | Route | Authentication | Purpose |
|---|---|---|---|
| `GET` | `/health` | public | Postgres + Redis readiness |
| `GET` | `/v1/status` | public | Overall, per-application, and per-component state with 24h/7d/90d uptime |
| `GET` | `/v1/status/history?days=90` | public | One uptime bucket per `America/Toronto` day, oldest first |
| `GET` | `/v1/session` | public | Always HTTP 200 `{authenticated, allowed}` |
| `POST` | `/v1/logs` | ingestion bearer token | Store one structured event |
| `GET` | `/v1/apps` | OAuthManager `logger/view` | Application/component status with health detail |
| `GET` | `/v1/apps/:app/logs/recent` | OAuthManager `logger/view` | Redis-backed recent tail |
| `GET` | `/v1/apps/:app/logs/history` | OAuthManager `logger/view` | Postgres historical query |

`/v1/session` never answers 401 or 403. The page has to tell "signed out" from
"signed in without `logger/view`", and a status code collapses those into one,
which used to bounce a permission-less user into an endless sign-in loop.
`authenticated` reports a valid OAuthManager session, `allowed` reports whether
it holds `logger` / `view`.

The public routes are rate limited (`STATUS_RATE_LIMIT`, 60 requests per minute
per address by default), send `Cache-Control: public, max-age=30`, and share one
in-process cache of the 90-day sample set (`STATUS_CACHE_TTL`, 30 seconds), so
an unauthenticated burst cannot become a burst of time-series queries. `days` is
clamped to 1..90 and never echoed back.

Recent and historical routes accept `levels=debug,info`, `q=search text`, and
`limit=1..200`. History additionally accepts RFC3339 `from`, `to`, and the
opaque `cursor` returned as `next_cursor`.

Log ingestion example:

```bash
curl -X POST http://127.0.0.1:8082/v1/logs \
  -H "Authorization: Bearer $LOGGER_INGEST_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "service": "exo-api",
    "level": "info",
    "message": "telemetry batch stored",
    "payload": {"samples": 256}
  }'
```

Service ids must match a component in
`backend/internal/catalog/default.json`. Set `LOGGER_CATALOG_JSON` to a complete
replacement catalog when an environment needs different applications or health
URLs.

## Application logging

Go services import `github.com/BioTronDesignTeam/Logger/client`. Each container
sets:

- `LOGGER_URL=http://logger-api:8080`
- `LOGGER_INGEST_TOKEN=<shared secret>`
- `LOGGER_SERVICE=<catalog component id>`
- `LOG_LEVEL=info` in production or `debug` while tracing

The client keeps all four methods in source and suppresses events below
`LOG_LEVEL` before making a network request. See `client/README.md` for usage.

OAuthManager and Exo emit lifecycle and safe completed-request metadata under
the catalog services `oauth-manager` and `exo-api`. Logger writes its own
startup and shutdown events directly as `logger-api`; bypassing its HTTP
ingestion route prevents recursive self-logging.

## Storage behavior

Postgres is authoritative. After inserting a durable row, the backend updates
the capped Redis tail. If Redis is temporarily unavailable, ingestion still
succeeds and recent reads fall back to Postgres, avoiding duplicate rows caused
by client retries.

Health is polled every `HEALTH_INTERVAL` (15 seconds by default). Redis is
updated on every poll; Postgres records state changes plus a heartbeat at
`HEALTH_HISTORY_INTERVAL` (five minutes by default), preventing unbounded
per-poll history growth.

## Production authentication

Logger forwards the browser's `oauth_session` cookie to OAuthManager's
`/v1/check?app=logger&permission=view` endpoint. HTTP 401 means unauthenticated
and HTTP 403 means authenticated but unpermitted; the two are never collapsed.
OAuthManager must therefore:

1. register a `logger` app with a `view` permission; and
2. issue its session cookie for the shared base domain so it is sent to trusted
   application subdomains.

For staging, the cookie domain is `biotron-dev.com`. Production uses its own
base domain. The cookie remains `Secure`, `HttpOnly`, and `SameSite=Lax`.
