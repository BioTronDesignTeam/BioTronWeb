# Auth

Auth signs operators in with GitHub and records what each one may do in each
BioTron tool. Login requires membership of the `BioTronDesignTeam` GitHub
organization. Each tool defines its own permissions. There is no request
queue: an operator asks a manager in person or on Discord, and the manager
grants the permission in the Org tab.

A tool can opt in to a daily guest key. The key rotates at Eastern midnight.
Staff read it in the Keys tab. A guest session ends with the key and works in
that tool only. Exo is the only tool with a guest key.

This app was the `OAuthManager` repository. The old name remains where a
change would break something: the Go module path, the Compose project name
`biotron-oauth-manager`, the network alias `oauth-manager`, and the service
name Auth reports to Logger.

## Layout

| Path | What |
|------|------|
| `backend/` | Go Fiber API. Postgres through pgx. Redis caches grant sets. |
| `frontend/` | Vite and React UI, served by Nginx. |
| `prisma/` | Schema and migrations. The Go service runs its own queries. |
| `docker-compose.yml` | `auth-migrate`, `auth-api`, `auth-web` on the `biotron` network. |
| `.env.example` | Every variable the app reads. Copy it to `.env`. |

## Run

Start the shared Postgres and Redis once, from the monorepo root, then Auth:

```bash
docker compose -f infra/docker-compose.yml --env-file infra/.env up -d
cp apps/auth/.env.example apps/auth/.env    # set the GitHub OAuth values
docker compose up -d --build auth-migrate auth-api auth-web
```

The UI is at http://localhost:5173 and the API at http://localhost:8080.
The API port must stay 8080: the GitHub OAuth callback is registered as
`http://localhost:8080/auth/github/callback`. Put your GitHub user id in
`SUPERUSER_GITHUB_IDS` so that the first login can grant permissions.

## Develop

Open the monorepo in its devcontainer. It supplies Node 24, Go 1.27, and a
Postgres and Redis that `.env.example` already points at. The post-create
step installs the workspaces and applies this app's migrations. Then:

```bash
npm run dev -w apps/auth/frontend    # http://localhost:5173
cd apps/auth/backend && go run .     # http://localhost:8080, reads ../.env
npm run lint                         # Oxlint, from the root
```

The root `package.json` owns Vite, TypeScript, Tailwind, and the type
packages. `frontend/package.json` lists only what Auth alone needs.

## Environment

One file, `apps/auth/.env`, feeds Compose, the backend, the frontend, and
Prisma. Do not add environment files below it.

- Leave `COOKIE_DOMAIN` empty on localhost. Set it to the parent domain when
  the tools run on sibling subdomains, so that one cookie reaches them all.
- `COOKIE_SECURE=true` is required when the callback URL is https or
  `COOKIE_DOMAIN` is set. The backend refuses to start without it.
- `CORS_ORIGINS` must list each tool frontend as both `localhost` and
  `127.0.0.1`. Browsers treat them as different origins.
- `TRUSTED_PROXIES` must include the edge Nginx network. The per-IP rate
  limit keys on `Cf-Connecting-Ip`, which Fiber reads only from a trusted
  proxy.
- `LOGGER_INGEST_TOKEN` must match Logger's. Leave it empty to send nothing.

## API

| Method | Path | Who | Notes |
|--------|------|-----|-------|
| GET | `/health` | anyone | liveness |
| GET | `/auth/github/login` | anyone | starts OAuth; `?redirect=` must be an allowed origin |
| GET | `/auth/github/callback` | anyone | non-members return with `?auth=denied`, banned operators with `?auth=banned` |
| POST | `/auth/guest` | anyone | guest login with `app_id` and `key`; 20 per minute per IP |
| POST | `/auth/logout` | session | ends the session |
| GET | `/auth/me` | session | current operator; a guest must pass its `?app=` |
| GET | `/auth/guest-keys` | staff | today's key for each opted-in tool |
| GET | `/apps`, `/permissions` | session | the catalog |
| POST | `/apps` | superuser | registers a tool |
| GET | `/me/grants` | session | my grants; `full_access` is true for staff |
| POST, DELETE | `/grants` | staff | grants or revokes one permission |
| GET | `/org/members` | staff | every operator who has signed in |
| GET | `/org/members/:id/grants` | staff | one member's grants |
| POST | `/org/members/:id/ban`, `/unban` | staff | a ban ends the member's sessions |
| PATCH | `/org/members/:id/manager` | superuser | sets the manager flag; ends the member's sessions |
| GET | `/v1/check?app=&permission=` | session | allow or deny |

Staff means a manager or a superuser. Every mutating route requires the
`X-Requested-With` header, which blocks cross-site form posts. Nobody can ban
or change themself or the guest account. Only a superuser can ban a superuser.

Sessions live in Postgres. The cookie is `oauth_session` and lasts
`SESSION_TTL_HOURS`. Membership is checked at login only, so a removed member
keeps access until the cookie ends or staff ban them. Roles and bans are read
on every request and are never stale.

### Permission check

Tools call `GET /v1/check?app=<id>&permission=<key>` with the operator's
cookie. The status code carries the verdict:

| Situation | Response |
|---|---|
| No cookie, or an unknown, expired, or banned session | `401` |
| `app` or `permission` missing | `400` |
| Valid session | `200` with `{"allowed": true}` or `{"allowed": false}` |

The route never answers `403`. Callers must still treat a `403` as "signed
in but not permitted", so that a stricter middleware can be added later
without sending anyone into a sign-in loop. A ban deletes the operator's
sessions at once, so the next check refuses.

### Events to Logger

Auth reports to Logger as `oauth-manager`. Every completed request is one
event. `/health` is never logged, and `/v1/check` and `/auth/me` are logged
only when they fail, because every page polls them. A request made with a
session names its operator as `actor`.

| Message | Level | Payload |
|---|---|---|
| `OAuthManager started` | info | `port` |
| `OAuthManager stopping` | info | — |
| `GitHub OAuth not configured` | warning | — |
| `HTTP request completed` | info, warning on 4xx, error on 5xx | `method`, `path`, `status`, `duration_ms`, `actor`, `error` |
| `Operator signed in` | info | `operator_id`, `operator_login`, `new_operator` |
| `Operator signed out` | info | `operator_id`, `operator_login` |
| `Sign-in refused` | warning | `reason` (`not a member` or `banned`), `login` when known |
| `GitHub sign-in failed` | error | `stage` (`exchange`, `membership`, `user`), `error` |
| `Guest signed in` | info | `app_id` |
| `Guest key refused` | warning | `app_id` |
| `Permission granted`, `Permission revoked` | info | `target_id`, `app`, `permission` |
| `Operator banned`, `Operator unbanned` | info | `target_id`, `target_login` |
| `Manager flag set`, `Manager flag removed` | info | `target_id`, `target_login` |
| `Tool registered` | info | `app_id` |
| `Session delete failed` | error | `error` |
| `Expired sessions pruned`, `Stale guest keys pruned` | info | `count` |
| `Session prune failed`, `Guest key prune failed` | error | `error` |

The grant, revoke, ban, unban, manager, and tool rows are the audit trail.
Each also names the actor as `actor_id` and `actor_login`. These events are
the only record of who changed what. The hourly prune reports only what it
deleted, so an idle hour sends nothing.

Query strings, cookies, tokens, guest keys, and the OAuth code and state are
never sent.

## Frontend

The header leads with the wordmark from `@biotron/style`; the "Auth" label
hides on phones. Each operator sees:

- **My access**. A read-only list of permissions per tool. Held ones are
  green chips with a check. A member who is not staff sees only this panel,
  with no tab bar.
- **Keys** (staff). Each opted-in tool's daily key, hidden until revealed.
- **Org** (staff). Every operator who has signed in. Select one to see and
  change their grants. Ban and the manager flag open a confirmation dialog
  that says what will happen. The manager flag is superuser-only. A staff
  member shows "Full access" and no grant form.

`VITE_API_BASE` is the only frontend variable. It is compiled into the
bundle; Compose passes it to `auth-web` as a build argument.

## Database

Prisma 6.19.3, pinned exactly. Six tables: `operators`, `sessions`,
`guest_keys`, `apps`, `permissions`, and `grants`. Tools and their
permissions are rows that the migrations seed, so `auth-migrate` is what
makes a fresh database usable. Without it, every permission check for a
non-staff operator answers `false`.

| Tool | Permissions |
|---|---|
| `exo-gui` | `live`, `historical`, `commands` |
| `logger` | `view` |
| `calendar` | `write` |
| `sprinter` | `admin` |

A guest session from Exo's key holds `exo-gui/live` and `exo-gui/historical`
and nothing else.

To create a migration in the devcontainer, source `apps/auth/.env` and run
`npm run --prefix apps/auth/prisma migrate`. The `auth-migrate` service
applies the migrations as the image's `node` user.
