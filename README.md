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

An occurrence override records only the fields that differ from the series, so
a field left alone keeps following later series-wide edits. Resetting an
occurrence deletes its override row outright rather than storing an empty one:
a retained row would keep blocking series-level start, timezone, all-day, and
recurrence-end changes with nothing left in the editor to remove. Saving an
occurrence whose fields all match the series again has the same effect as a
reset. Rows written by earlier releases as empty-patch tombstones are treated
everywhere as if they were absent.

Cancelling a series cancels every one of its instances, including occurrences
an editor had previously changed, so a subscriber's calendar removes all of
them.

Recurrence is stored and stepped as `America/Toronto` wall clock, so a meeting
keeps its local hour across a daylight-saving change. The two local times a
year that are ambiguous or nonexistent follow RFC 5545 section 3.3.5, the same
rule a subscriber's calendar client applies to the `TZID` values in the feed:
the repeated hour at fall-back resolves to the first of its two instants, and
an hour skipped by spring-forward is read with the offset in force before the
gap, so 02:30 on 8 March 2026 means 03:30 EDT.

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

`SITE_URL` defaults to `http://localhost:5177` so that running the API outside
compose does not silently drop the site from the public CORS list — a failure
that shows up only as a blocked request in the visitor's browser. Startup logs
a warning naming the consequence whenever a public read origin is missing.

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
- `GET /v1/events/upcoming?limit=5&days=42`
- `GET /v1/feeds/all.ics`
- `GET /v1/feeds/scopes/:scopeID.ics`

Administrative routes live under `/v1/admin` and cover scope lifecycle, event
draft/publish/cancel, and occurrence overrides.

### `GET /v1/events/upcoming`

The single source of truth for "what is on next", so the public site, the
Sprinter bot, and anything after them do not each re-derive it. It returns the
same occurrence objects as `GET /v1/events`:

```json
[
  {
    "series_id": "…", "uid": "…@biotron.ca",
    "scope_id": "…", "scope_name": "Controls and Signals", "scope_kind": "SUBTEAM",
    "title": "Controls sync", "description": "", "location": "E5 2004", "url": "",
    "starts_at": "2026-09-08T18:00:00-04:00", "ends_at": "2026-09-08T19:00:00-04:00",
    "all_day": false, "timezone": "America/Toronto",
    "recurrence_id_local": "2026-09-08T18:00:00",
    "recurring": true, "modified": false,
    "series_sequence": 3, "override_sequence": 1
  }
]
```

`override_sequence` is omitted when it is zero. Occurrences come from every
active public scope, ordered by `starts_at` ascending, already truncated.

- `limit` — how many occurrences to return, 1 to 20, default 5.
- `days` — how far ahead to look, 1 to 90, default 42.

Both are clamped rather than rejected, so a caller asking for more than the API
will give gets the maximum instead of an error to handle. The window starts at
the moment of the request, not at the start of today, so a meeting that
finished this morning is not upcoming; a meeting already in progress is.

Like the feeds, the response carries `ETag`, `Last-Modified`, and
`Cache-Control: public, max-age=60, stale-while-revalidate=300`, and answers a
conditional request with `304`. It matches on `If-None-Match` only: its window
slides with the clock, so an unchanged `Last-Modified` must never be allowed to
serve a body that should have changed.

## Verification

```bash
docker run --rm -v "$PWD/backend:/src" -w /src golang:1.23-bookworm go test ./...
docker run --rm -v "$PWD/backend:/src" -w /src golang:1.23-bookworm go vet ./...
npm --prefix frontend run build
npm --prefix prisma run format
npm --prefix prisma run validate
```
