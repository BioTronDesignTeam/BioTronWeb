# BiotronCalendar

BioTron's public calendar, editor, and independent iCalendar subscription feeds.
The application is the single source of truth used by both the full Calendar UI
and the lightweight upcoming-events view on the BioTron public site.

## Architecture

- `frontend/` is React, TypeScript, Tailwind, and shared `BioTronStyle` UI.
- `backend/` is the Go/Fiber public API, permission-enforced editor API, recurrence engine, and feed generator.
- `prisma/` owns the `calendar` schema and its PostgreSQL migrations.
- `.devcontainer/` provides Node 22, Go 1.23, Prisma tooling, and forwarded UI/API ports.
- `docker-compose.yml` runs the migration, API, and static frontend containers on the shared `biotron` network.

PostgreSQL is the source of truth. Redis is intentionally not part of the
Calendar path: the expected read volume does not justify invalidation risk or
operational complexity yet. HTTP feed responses still provide `ETag`,
`Last-Modified`, and short cache-control directives.

## Calendar model

Events belong to exactly one active scope: the whole team, one project, or one
subteam. Subscriptions are exact and independent. The All Events feed is the
only feed that aggregates scopes.

A series is either a one-off event or a weekly series with an inclusive end
date. A changed or cancelled meeting is stored as an occurrence override keyed
to its original Toronto wall-clock start. Editors can change one occurrence,
cancel one occurrence, or reset it to the series schedule. Version 1 does not
implement "this and future" edits or pause/resume; a new term gets a new series.

Projects and subteams can be renamed, archived, restored, or deleted. An
archived ancestor hides all descendants and their public JSON events while
retaining records and the contents of stable subscription feeds. Hard deletion
succeeds only for an empty leaf scope with no event history.

## Local setup

Start the shared PostgreSQL container and make sure the external Docker network
named `biotron` exists. Copy `.env.example` to `.env`, replace the example
database password, and run:

```bash
docker compose up --build
```

The default local URLs are:

- Calendar UI: `http://localhost:5176`
- Calendar API: `http://localhost:8083`
- OAuthManager: `http://localhost:8080`
- BioTron site: `http://localhost:5177`

All browser-facing and service URLs remain environment-driven for deployment.
The repository uses one root `.env`; do not create component-level environment
files. `CORS_ORIGINS` adds aliases that may call public read routes;
`ADMIN_CORS_ORIGINS` is deliberately separate and adds aliases allowed to make
credentialed editor requests. The configured Calendar frontend is included in
both lists, while the public site is read-only.

## Authentication

Public scopes, occurrences, and feeds do not require authentication. The editor
uses OAuthManager app id `calendar` and permission `write`. Every admin endpoint
checks the shared session on the server. State-changing requests also require
`X-Requested-With: XMLHttpRequest`.

Access requests remain in OAuthManager. The Calendar UI intentionally has no
request-access workflow.

## Public API

- `GET /health`
- `GET /v1/auth/status`
- `GET /v1/scopes`
- `GET /v1/events?from=YYYY-MM-DD&to=YYYY-MM-DD&scope_id=...`
- `GET /v1/feeds/all.ics`
- `GET /v1/feeds/scopes/:scopeID.ics`

Administrative routes live under `/v1/admin` and cover scope lifecycle, event
draft/publish/cancel, and occurrence overrides.

## Verification

```bash
docker run --rm -v "$PWD/backend:/src" -w /src golang:1.23-bookworm go test ./...
npm --prefix frontend run build
npm --prefix prisma run format
npm --prefix prisma run validate
```
