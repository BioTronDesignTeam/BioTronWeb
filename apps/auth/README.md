# Auth

Central GitHub OAuth and per-app permissions for the BioTron tools. Operators
sign in with GitHub; membership of the `BioTronDesignTeam` org is checked at
login. Each product defines its own named permission catalog. There is no
request queue: an operator asks for access in a meeting or on Discord, and a
manager grants it from the Org tab. My access shows each operator what they
hold today.

Products can opt in to daily guest keys. Each opted-in product gets its own
key, and every key rotates at Eastern midnight. Managers and superusers reveal
or copy them from the Keys tab. A guest session expires with the key and is
bound to the product that issued it. Exo is the only product with daily keys
enabled.

Other tools sign in through this API. GitHub login accepts `?redirect=<origin>`
when that origin is `FRONTEND_URL` or one of `CORS_ORIGINS`.

This app was the `OAuthManager` repository. The old name survives where
renaming would break something: the Go module path, the Compose project name
`biotron-oauth-manager`, the network alias `oauth-manager` that other APIs
call, and the service name Auth reports to Logger.

## Layout

| Path | What |
|------|------|
| `backend/` | Go Fiber API. Queries Postgres through pgx; caches grant sets in Redis. |
| `frontend/` | Vite and React UI. Its container serves the built files with Nginx. |
| `prisma/` | Schema and migrations. Migrations only; the Go service runs its own queries. |
| `docker-compose.yml` | `auth-migrate`, `auth-api`, and `auth-web` on the shared `biotron` network. |
| `.env.example` | Every variable Compose, the backend, the frontend, and Prisma read. Copy it to `.env`. |

## Quick start

The shared Postgres and Redis live in `infra/`. Start them once, from the
monorepo root:

```bash
cp infra/.env.example infra/.env
# Replace the Postgres password, then:
docker compose -f infra/docker-compose.yml --env-file infra/.env up -d
```

That creates the external `biotron` network every app joins. Then start Auth:

```bash
cp apps/auth/.env.example apps/auth/.env
# Use the same Postgres password. Fill in the GitHub OAuth settings.
docker compose up -d --build auth-migrate auth-api auth-web
```

The same `docker compose up -d --build` works alone from `apps/auth/`. The
root compose file includes this app's file and reads `apps/auth/.env` for it.
`auth-api` waits for `auth-migrate` to finish; `auth-web` waits for
`auth-api`.

- UI: http://localhost:5173
- API: http://localhost:8080/health

Register a GitHub OAuth App under BioTronDesignTeam with callback
`http://localhost:8080/auth/github/callback`. Put your GitHub numeric user id
in `SUPERUSER_GITHUB_IDS` so the first login can grant permissions.

The API port stays on **8080** because the GitHub OAuth callback is registered
against it. The other services take the ports beside it:

| Service | Frontend | API |
| --- | --- | --- |
| Auth | `5173` | `8080` |
| Exo | `5174` | `8081` |
| Logger | `5175` | `8082` |
| Calendar | `5176` | `8083` |
| Site | `5177` | n/a |
| Sprinter | `5178` | `8084` |

## Develop

Open the monorepo in its devcontainer. It has Node 24 and Go 1.27, and it
starts a Postgres 18 and a Redis 8 beside the workspace, named `postgres` and
`redis`, with user `biotron`, password `change-me`, and database `biotron`.
`.env.example` already points at them, with `?schema=oauth`. The post-create
step runs `npm ci` and `go work sync`, copies `.env` from the example where
none exists, and applies this app's migrations. After that:

```bash
npm run dev -w apps/auth/frontend    # http://localhost:5173
cd apps/auth/backend && go run .     # http://localhost:8080, reads ../.env
```

The root `package.json` owns the workspaces and the toolchain. From the root:

```bash
npm ci                                 # every frontend and packages/style
npm run build -w apps/auth/frontend    # tsc --noEmit, then vite build
npm run lint                           # Oxlint, type-aware, .oxlintrc.json
go work sync                           # after changing any go.mod
```

`frontend/package.json` lists only `@biotron/style`, `react`, and
`react-dom`. Vite, TypeScript, Tailwind, and the type packages come from the
root. There is one root `package-lock.json`; `prisma/` keeps its own because
it is not a workspace.

### Environment

This app has one environment file, `apps/auth/.env`. Compose, the backend
(through `godotenv`), the frontend (Vite `envDir: '..'`), and Prisma all read
it. Do not create component-level environment files.

- Leave `COOKIE_DOMAIN` blank on localhost. Set it to the shared parent domain
  (for example `.biotron.ca`) when the products run on sibling subdomains, so
  one session cookie reaches every tool.
- `COOKIE_SECURE=true` is required once `OAUTH_CALLBACK_URL` is https or
  `COOKIE_DOMAIN` is set. The backend refuses to start otherwise.
- `CORS_ORIGINS` must list every product frontend in both its `localhost` and
  `127.0.0.1` form. Browsers treat those as different origins.
- `TRUSTED_PROXIES` must include the edge Nginx's network. Fiber reads
  `Cf-Connecting-Ip` only from a trusted proxy, and the per-IP rate limit
  keys on that address.
- `SESSION_TTL_HOURS` defaults to 168. `CACHE_TTL_SECONDS` defaults to 300.

### Logger

The API sends events to Logger as `oauth-manager`. `LOGGER_URL` defaults to
`http://logger-api:8080`. Set `LOGGER_INGEST_TOKEN` to the same shared secret
Logger uses; leaving it empty disables delivery without stopping Auth.
`LOG_LEVEL` sets the minimum level sent.

Three kinds of event go out:

- Lifecycle: `OAuthManager started` and `OAuthManager stopping`.
- Completed requests, with method, path, status, and duration. `/health` is
  never logged. Successful `OPTIONS`, `/v1/check`, and `/auth/me` requests are
  skipped because they are the busy paths. Query strings, cookies, and
  credentials are never included.
- Audit: `Permission granted`, `Permission revoked`, `Operator banned`,
  `Operator unbanned`, `Manager flag set`, and `Manager flag removed`. Each
  names the actor by id and login and the target by id; ban, unban, and
  manager changes add the target's login, and grants add the app and
  permission. Access is decided in meetings and on Discord, so these events
  are the only record of who changed what.

`backend/internal/eventlog` is a copy of `go/logclient`. It should import the
shared module and go away.

## Backend

Go Fiber v3 service. Postgres through pgx; Redis through go-redis. Prisma owns
the schema.

| Method | Path | Who | Notes |
|--------|------|-----|-------|
| GET | `/health` | anyone | liveness |
| GET | `/auth/github/login` | anyone | start OAuth; optional `?redirect=<allowed origin>` |
| GET | `/auth/github/callback` | anyone | finish OAuth; non-members bounce to `?auth=denied`, banned operators to `?auth=banned` |
| POST | `/auth/guest` | anyone | daily-key login with `app_id` and `key`; 20 per minute per IP |
| POST | `/auth/logout` | session | ends the session |
| GET | `/auth/me` | session | current operator; a guest must pass `?app=` matching its product |
| GET | `/auth/guest-keys` | staff | today's key for every opted-in product |
| GET | `/apps` | session | app catalog |
| GET | `/permissions` | session | permission catalog |
| POST | `/apps` | superuser | register an app |
| GET | `/me/grants` | session | my grants, plus `full_access` for staff |
| POST, DELETE | `/grants` | staff | grant or revoke one permission |
| GET | `/org/members` | staff | every operator who has signed in |
| GET | `/org/members/:id/grants` | staff | one member's grants |
| POST | `/org/members/:id/ban`, `/unban` | staff | a ban ends the member's sessions |
| PATCH | `/org/members/:id/manager` | superuser | set or clear the manager flag; ends the member's sessions |
| GET | `/v1/check?app=&permission=` | session | allow or deny; `action=` is accepted as an alias |

"Staff" means a manager or a superuser. Every mutating route requires the
`X-Requested-With` header, which blocks cross-site form posts. Nobody can ban
or change themselves, the guest account, or (unless a superuser) a superuser.

Sessions live in Postgres only. The cookie is `oauth_session`; it lasts
`SESSION_TTL_HOURS`. Org membership is checked at login and not again, so a
removed member keeps access until the cookie dies or staff ban them; see
`backend/internal/auth/DEFERRED.md`. Redis caches grant sets for
`CACHE_TTL_SECONDS` and is cleared on grant and revoke. Roles and bans are
read from Postgres on every request, so they are never stale. An hourly loop
prunes expired sessions and old daily keys.

## Frontend

Vite and React, Tailwind 4, and the shared components from `@biotron/style`:
`Brand`, `ThemeToggle`, `UserMenu`, and `AuthScreen`. The header leads with
the 112px wordmark; the "Auth" label beside it hides at phone widths.

What each operator sees:

- **My access**: a read-only "Your permissions" view. Held permissions are
  green chips with a check; the rest are outlined. Members who are not staff
  see this and no tab bar at all. Staff see "Full access".
- **Keys** (staff): each opted-in product's daily key, hidden until revealed,
  with a Copy button.
- **Org** (staff): every operator who has signed in. Selecting one shows
  their grants and a grant form. Ban and the manager flag each ask for
  confirmation in a native `<dialog>` that says what will happen; Unban is
  one click. The manager flag is superuser-only. A member who is already
  staff shows "Full access" and no grant form.

The sign-in screen shows a notice for `?auth=denied` and `?auth=banned`.

`VITE_API_BASE` (default `http://localhost:8080`) is the only frontend
variable. It is compiled into the bundle; Compose passes it as a build
argument to `auth-web`. Scripts: `dev`, `build` (`tsc --noEmit && vite build`),
and `preview`.

## Database

Prisma 6.19.3, pinned exactly. `prisma/package.json` also overrides the
transitive dependency `deepmerge-ts` to 8.0.0; `@prisma/config` asks for
7.1.5. The `prisma-client-js` generator is a placeholder; no code uses it.
The Go service queries through pgx in `backend/internal/store`.

Tables: `operators`, `sessions`, `guest_keys`, `apps`, `permissions`, and
`grants`. Six migrations build them; the last,
`20260905120000_drop_access_requests`, removed the request queue.

Apps and their permissions are rows, not schema. Migrations seed the catalog,
so `auth-migrate` (which runs `prisma migrate deploy`) is what makes a fresh
database usable. Skipping migrations, for example by running
`prisma db push`, leaves the tables empty, and every non-staff permission
check answers `allowed: false`.

| App id | Permission keys | Seeded by |
|---|---|---|
| `exo-gui` | `live`, `historical`, `commands` | `20260830030000_product_permissions` |
| `logger` | `view` | `20260830030000_product_permissions` |
| `calendar` | `write` | `20260729240000_managers_permission_catalog` |

Only `exo-gui` has `daily_key_enabled`. A guest session created from Exo's
daily key is allowed `exo-gui/live` and `exo-gui/historical` and nothing
else: never `exo-gui/commands`, `logger/view`, or `calendar/write`.

Working with migrations in the devcontainer:

```bash
docker compose run --rm auth-migrate            # apply, as the container does
set -a && . apps/auth/.env && set +a            # Prisma reads DATABASE_URL from the environment
npm run --prefix apps/auth/prisma migrate       # prisma migrate dev: create and apply
npm run --prefix apps/auth/prisma studio
```

The Prisma image installs from its own lockfile and runs `npm run deploy` as
the image's `node` user.

## Permission check contract

Products ask `GET /v1/check?app=<id>&permission=<key>` with the caller's
`oauth_session` cookie forwarded. The status code, not the body, separates
the two failure modes:

| Situation | Response |
|---|---|
| No cookie, unknown, expired, or banned operator's session | `401` |
| `app` or `permission` missing | `400` |
| Valid session | `200 {"allowed": true}` or `200 {"allowed": false}` |

The route never answers `403`, because `RequireSession` is the only
middleware in front of it and a permission refusal is a `200` with
`allowed: false`. Callers should still map a `403` to "authenticated but not
permitted" rather than "signed out", so this service can grow a stricter
middleware later without pushing anyone into a sign-in loop.

Banning an operator deletes their sessions immediately, and sessions are
never cached in Redis, so a ban takes effect on the very next check.

## CI

`.github/workflows/ci.yml` runs when `apps/auth/**` changes: the frontend job
builds `packages/style` and this app and runs Oxlint on `frontend/src`; the
backend job runs golangci-lint, `go vet`, and `go test`. The workflow has not
run yet.
