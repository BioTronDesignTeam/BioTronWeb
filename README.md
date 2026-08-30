# Logger

BioTron's internal application status page, structured log warehouse, and log
explorer.

## What it does

- Polls every application component from inside the `biotron` Docker network.
- Keeps the latest health state per component in Redis.
- Records health transitions and periodic snapshots in Postgres.
- Accepts structured `debug`, `info`, `warning`, and `error` events.
- Keeps a capped recent tail per service in Redis while Postgres remains the
  durable log warehouse.
- Provides recent and historical application log views with level, text, and
  time filters.
- Checks OAuthManager's `logger/read` permission before serving operational
  data.

## Layout

| Path | What |
|---|---|
| `frontend/` | React + TypeScript status and log portal |
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

The portal is available on `http://localhost:5175`; the API is bound to
`http://127.0.0.1:18081`.

## API

| Method | Route | Authentication | Purpose |
|---|---|---|---|
| `GET` | `/health` | none | Postgres + Redis readiness |
| `POST` | `/v1/logs` | ingestion bearer token | Store one structured event |
| `GET` | `/v1/session` | OAuthManager `logger/read` | Portal access check |
| `GET` | `/v1/apps` | OAuthManager `logger/read` | Application/component status |
| `GET` | `/v1/apps/:app/logs/recent` | OAuthManager `logger/read` | Redis-backed recent tail |
| `GET` | `/v1/apps/:app/logs/history` | OAuthManager `logger/read` | Postgres historical query |

Recent and historical routes accept `levels=debug,info`, `q=search text`, and
`limit=1..200`. History additionally accepts RFC3339 `from`, `to`, and the
opaque `cursor` returned as `next_cursor`.

Log ingestion example:

```bash
curl -X POST http://127.0.0.1:18081/v1/logs \
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
`/v1/check?app=logger&permission=read` endpoint. OAuthManager must therefore:

1. register a `logger` app with a `read` permission; and
2. issue its session cookie for the shared base domain so it is sent to trusted
   application subdomains.

For staging, the cookie domain is `biotron-dev.com`. Production uses its own
base domain. The cookie remains `Secure`, `HttpOnly`, and `SameSite=Lax`.
