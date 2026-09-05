# Calendar

Calendar is BioTron's public calendar. Anyone can browse the events and
subscribe to an iCalendar feed. An event belongs to one scope: the team
root, a project, or a subteam. Each scope has its own feed, and the All
feed carries every scope. An event is one-off or repeats weekly through
an inclusive end date. Times are `America/Toronto` wall clock, so a
weekly meeting keeps its hour across a daylight-saving change.

The public site's Calendar page reads the next few events from
`GET /v1/events/upcoming` and links here. Editing requires the
`calendar/write` permission from Auth; browsing requires nothing. This
app was the `BiotronCalendar` repository. The old name remains in the Go
module path and the Compose project name `biotron-calendar`.

## Layout

| Path | What |
|------|------|
| `backend/` | Go Fiber API. Postgres through pgx. No Redis. |
| `frontend/` | Vite and React UI, served by Nginx. |
| `prisma/` | Schema and migrations. The Go service runs its own queries. |
| `docker-compose.yml` | `calendar-migrate`, `calendar-api`, `calendar-web` on the `biotron` network. |
| `.env.example` | Every variable the app reads. Copy it to `.env`. |

## Run

Start the shared Postgres once, from the monorepo root, then Calendar:

```bash
docker compose -f infra/docker-compose.yml --env-file infra/.env up -d
cp apps/calendar/.env.example apps/calendar/.env
docker compose up -d --build calendar-migrate calendar-api calendar-web
```

The UI is at http://localhost:5176 and the API at http://localhost:8083.
To edit events, run Auth as well. The API reaches it at
`OAUTH_MANAGER_URL`, the network alias `oauth-manager` by default.

## Develop

Open the monorepo in its devcontainer. It supplies Node 24, Go 1.27, and
a Postgres that `.env.example` already points at. The post-create step
installs the workspaces and applies this app's migrations. Then:

```bash
npm run dev -w apps/calendar/frontend           # http://localhost:5176
cd apps/calendar/backend && PORT=8083 go run .  # reads ../.env
npm run lint                                    # Oxlint, from the root
```

`PORT` defaults to 8080, which Auth uses, so set it to match
`VITE_API_URL`. To edit natively, set `OAUTH_MANAGER_URL` to
`http://localhost:8080` in `.env`; the example names the Compose alias.
`go test ./...` in `backend/` runs the tests. Tests named `Live*` need
`CALENDAR_TEST_DATABASE_URL`, a throwaway Postgres with the `calendar`
schema migrated, and skip without it.

## Environment

One file, `apps/calendar/.env`, feeds Compose, the backend, the
frontend, and Prisma. Do not add environment files below it.

- `CORS_ORIGINS` adds origins that may read the public routes.
  `ADMIN_CORS_ORIGINS` adds origins that may make credentialed editor
  requests. `FRONTEND_URL` is in both lists; `SITE_URL` is in the public
  list only. List each origin as `localhost` and `127.0.0.1`.
- An empty `SITE_URL` logs a warning at start. The site's Calendar page
  then fails CORS in the browser, and the server log shows nothing.
- `DEFAULT_TIMEZONE` must be `America/Toronto`. The backend refuses to
  start with any other value.
- `MAX_RANGE_DAYS` caps the `/v1/events` window. Default 370, at most 730.
- `PUBLIC_BASE_URL` is the origin written into each feed's `URL` line.
- `LOGGER_INGEST_TOKEN` must match Logger's. Leave it empty to send nothing.

## API

| Method | Path | Who | Notes |
|--------|------|-----|-------|
| GET | `/health` | anyone | `503` when the database is down |
| GET | `/v1/auth/status` | anyone | `operator` and `can_write` for the cookie sent |
| GET | `/v1/scopes` | anyone | active scopes |
| GET | `/v1/events?from=&to=&scope_id=` | anyone | occurrences in a date window; default today to three months ahead |
| GET | `/v1/events/upcoming?limit=&days=` | anyone | the next occurrences; `limit` 1 to 20, default 5; `days` 1 to 90, default 42 |
| GET | `/v1/feeds/all.ics` | anyone | every scope |
| GET | `/v1/feeds/scopes/:id.ics` | anyone | one scope |
| GET | `/v1/admin/scopes`, `/v1/admin/events` | editor | everything, archived scopes and draft events included |
| POST | `/v1/admin/scopes` | editor | `kind`, `name`, `parent_id`; a project under the root, a subteam under a project |
| PATCH | `/v1/admin/scopes/:id` | editor | rename |
| POST | `/v1/admin/scopes/:id/archive`, `/restore` | editor | restore refuses while the parent is archived |
| DELETE | `/v1/admin/scopes/:id` | editor | only an empty leaf with no event history |
| POST | `/v1/admin/events` | editor | creates a draft |
| PATCH | `/v1/admin/events/:id` | editor | edits the series |
| POST | `/v1/admin/events/:id/publish`, `/cancel` | editor | a cancelled series can be published again |
| DELETE | `/v1/admin/events/:id` | editor | drafts only |
| PUT | `/v1/admin/events/:id/occurrences` | editor | changes or cancels one occurrence |
| DELETE | `/v1/admin/events/:id/occurrences?recurrence_id_local=&expected_sequence=` | editor | resets one occurrence to the series |

Editor means an operator whose Auth session holds `calendar/write`. On
every editor request the API forwards the cookie to Auth's `/auth/me`
and `/v1/check?app=calendar&permission=write`. It reads both so the log
can name the operator behind the change. No cookie answers `401`, a
session without the permission `403`, an unreachable Auth `503`. Every
mutating request must carry `X-Requested-With: XMLHttpRequest`, which
blocks cross-site form posts.

Every write to an existing series carries `expected_sequence`. The
`sequence` rises on each edit, publish, cancel, and occurrence change; a
stale value answers `409`, so two editors cannot overwrite each other. A
published series cannot move to another scope. An archived scope takes
no new or published events. Only a published weekly series can have
occurrence changes. A change stores only the fields that differ from the
series, keyed by the occurrence's original start, so a field left alone
follows later series-wide edits. Cancelling a series cancels every
occurrence, changed ones included. A new term gets a new series; there
is no "this and future" edit.

### Logging

Calendar reports to Logger as `calendar-api`. Every admin event names the
operator as `actor_id` and `actor_login`, so a change can be traced to the
person who made it.

| Message | Level | Payload |
|---------|-------|---------|
| `BiotronCalendar started` | info | `port` |
| `BiotronCalendar stopping` | info | — |
| `Configuration warning` | warning | `warning` |
| `HTTP request completed` | info, warning on 4xx, error on 5xx | `method`, `path`, `status`, `duration_ms`, `actor` and `error` when known |
| `Authorization service unavailable` | error | `error` |
| `Scope created` | info | `scope_id`, `kind`, `name`, `parent_id` |
| `Scope renamed` | info | `scope_id`, `name` |
| `Scope archived` | info | `scope_id` |
| `Scope restored` | info | `scope_id` |
| `Scope deleted` | info | `scope_id` |
| `Event created` | info | `series_id`, `scope_id`, `title` |
| `Event updated` | info | `series_id`, `sequence` |
| `Event published` | info | `series_id`, `scope_id`, `title` |
| `Event cancelled` | info | `series_id` |
| `Event deleted` | info | `series_id` |
| `Occurrence changed` | info | `series_id`, `recurrence_id_local`, `cancelled` |
| `Occurrence reset` | info | `series_id`, `recurrence_id_local` |

`/health` and successful preflights are skipped. A domain event fires only
after the change reached the database.

### Occurrences

`/v1/events` and `/v1/events/upcoming` return the same objects, sorted
by start. The site's Calendar page reads `series_id`,
`recurrence_id_local`, `scope_path`, `title`, `description`, `location`,
`starts_at`, `ends_at`, and `all_day`, and shows an unavailable state if
any is missing. `starts_at` and `ends_at` carry the Toronto offset.
`scope_path` qualifies a scope by its ancestors, root omitted, so two
subteams both named Software stay apart. An occurrence is upcoming until
it ends: a meeting in progress is listed, one that ended this morning is
not. `limit` and `days` are clamped, never rejected.

### Feeds

A feed is the whole calendar, never a window: a subscriber's app
reconciles against the whole document, so a dropped past event would
vanish from their calendar. Drafts are left out. A published series is a
`VEVENT` with `STATUS:CONFIRMED`; a cancelled one stays with
`STATUS:CANCELLED`. A weekly series is one `RRULE:FREQ=WEEKLY;UNTIL=`. A
changed or cancelled occurrence is a second `VEVENT` with a
`RECURRENCE-ID`. Times are `TZID=America/Toronto` wall clock with a
`VTIMEZONE` block, so the feed and the subscriber's app agree on
daylight saving. Feed URLs use the scope id, so a rename never breaks a
subscription. An archived scope leaves the public JSON, but its feed
keeps its events.

The feeds and `/v1/events/upcoming` send `ETag`, `Last-Modified`, and
`Cache-Control: public, max-age=60, stale-while-revalidate=300`, and
answer a matching `If-None-Match` with `304`. Only the feeds honour
`If-Modified-Since`: the upcoming window moves with the clock.

## Frontend

The header leads with the wordmark from `@biotron/style`; the "Calendar"
label hides on phones. Log in sends the browser to Auth's GitHub login
and back. An editor gets a Manage button.

- **Month grid** on tablets and up, **Agenda** on phones: the same weeks
  as a grid with today circled, or as a list of days with events.
- **Filter**. The döner icon opens a checkbox tree. Ticking a project
  ticks its subteams; a "General" row selects the project's own events
  alone. No selection means everything.
- **Subscribe**. One row per feed with Copy URL, a `webcal:` link, and
  the address itself, because `webcal:` has no handler on Android or most
  Linux desktops. `?subscribe=1` opens this panel on load.
- **Event details**. Scope path, series and occurrence chips, time in
  ET, location, description, and link. An editor also gets Edit series
  and, on a weekly series, Edit and Cancel this occurrence.
- **Manage** (editors). Events: Edit, Publish, Cancel series, Delete a
  draft. Projects & subteams: Rename, Archive, Restore, Delete, Add.
  Destructive actions confirm in a dialog.

`VITE_API_URL`, `VITE_AUTH_URL`, and `VITE_SITE_URL` are compiled into
the bundle; Compose passes them to `calendar-web` as build arguments.
Nothing uses `VITE_SITE_URL` yet.

## Database

Prisma 6.19.3, pinned exactly. Three tables in the `calendar` schema:
`calendar_scopes`, `event_series`, and `event_overrides`. The one
migration, `20260901010000_calendar_domain`, creates them and seeds the
team root `BioTron` (slug `teamwide`) with a fixed id. A partial unique
index allows one `TEAM` row; a check makes every other scope carry a
parent. `event_series.uid` is `<uuid>@biotron.ca` and is the feed `UID`.
`starts_at_local` and `ends_at_local` are `timestamp` without zone,
because they are wall clock. Deleting a series removes its overrides; a
scope with series cannot be deleted. The Go store sets `search_path`
from the `?schema=` in `DATABASE_URL`.

To create a migration in the devcontainer, source `apps/calendar/.env`
and run `npm run --prefix apps/calendar/prisma migrate`. The
`calendar-migrate` service applies the migrations as the image's `node`
user.
