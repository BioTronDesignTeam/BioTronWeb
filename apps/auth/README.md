# Auth

Auth signs operators in with GitHub and records what each one may do in each
BioTron tool. Login requires membership of the `BioTronDesignTeam` GitHub
organization. Each tool defines its own permissions. There is no request
queue: an operator asks a manager in person or on Discord, and the manager
grants the permission in the Org tab.

A tool can opt in to a daily guest key. The key rotates at Eastern midnight.
Staff read it in the Keys tab, and can rotate it early there. A guest session
ends with the key, whether at midnight or by an early rotation, and works in
that tool only. Exo is the only tool with a guest key.

This app was the `OAuthManager` repository. The old name remains where a
change would break something: the Go module path, the Compose project name
`biotron-oauth-manager`, the network alias `oauth-manager`, and the service
name Auth reports its events under.

## Layout

| Path | What |
|------|------|
| `backend/` | Go `net/http` API. Postgres through pgx and sqlc. Grants are cached in memory. |
| `frontend/` | Vite and React UI, served by Nginx. |
| `prisma/` | Schema and migrations. The Go service runs its own queries. |
| `docker-compose.yml` | `auth-migrate`, `auth-api`, `auth-web` on the `biotron` network. |
| `.env.example` | Every variable the app reads. Copy it to `.env`. |

## Run

Start the shared Postgres once, from the monorepo root, then Auth:

```bash
docker compose -f infra/docker-compose.yml --env-file infra/.env up -d
cp apps/auth/.env.example apps/auth/.env    # set the GitHub App values
docker compose up -d --build auth-migrate auth-api auth-web
```

The UI is at http://localhost:5173 and the API at http://localhost:8080.
The API port must stay 8080: the GitHub App's callback is registered as
`http://localhost:8080/auth/github/callback`. Put your GitHub user id in
`SUPERUSER_GITHUB_IDS` so that the first login can grant permissions.

## Develop

Open the monorepo in its devcontainer. It supplies Node 24, Go 1.27, and a
Postgres that `.env.example` already points at. The post-create
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
  `127.0.0.1`. Browsers treat them as different origins. These origins get
  credentialed CORS only for what the tools call: `GET /auth/me`,
  `GET /v1/check`, `POST /auth/logout`, and `POST /auth/guest`. Every other
  route, and every staff action, answers cross-origin only to
  `FRONTEND_URL`, Auth's own frontend. A tool that needs another route of
  Auth adds it to `siblingRoutes` in `backend/internal/server/cors.go`.
- `TRUSTED_PROXIES` must include the edge Nginx network. The per-IP rate
  limit keys on `Cf-Connecting-Ip`, which the API reads only from a trusted
  proxy.
- `SESSION_SIGNING_KEY` signs the session and sign-in cookies. Generate one
  per environment with `openssl rand -base64 48` and keep it secret: anyone
  holding it can sign in as anyone. The backend refuses a key shorter than 32
  characters or one that looks like a placeholder.

## The GitHub App

Sign-in is the user authorization of a GitHub App, not an OAuth App. The
user token only names the operator (`GET /user`) and is then dropped: it is
never stored or logged. Membership of the organization is asked with the
App's own installation token, which holds Members: read and nothing else, so
operators are never asked to grant an organization scope. The backend refuses
to start without the App's four values.

Create one App per environment (local, staging, production), so that a key
or secret from one never works in another:

1. In the `BioTronDesignTeam` organization: Settings, Developer settings,
   GitHub Apps, New GitHub App. Name it, for example, `BioTron Auth (staging)`.
2. Homepage URL: the Auth frontend, such as `https://auth.uwbiotron.dev`.
3. Callback URL, exactly what `OAUTH_CALLBACK_URL` says:
   - local: `http://localhost:8080/auth/github/callback`
   - staging: `https://auth.uwbiotron.dev/api/auth/github/callback`
   - production: `https://auth.uwbiotron.ca/api/auth/github/callback`
4. Leave "Expire user authorization tokens" on, and "Request user
   authorization (OAuth) during installation" and "Enable Device Flow" off.
5. Webhook: untick Active. Auth needs no events.
6. Permissions: Organization permissions, Members: Read-only. Nothing else;
   reading `/user` needs no permission.
7. Where can this App be installed: Only on this account.
8. Create it. Note the App ID (a number) and the Client ID (`Iv...`).
   Generate a client secret and a private key (a `.pem` download).
9. Install App, on `BioTronDesignTeam`. An owner must approve the install,
   and must approve again whenever the App's permissions change.

Then set, in `apps/auth/.env` locally and in the environment's secrets on a
server:

| Variable | Value |
|---|---|
| `GITHUB_APP_ID` | The App ID, a number. |
| `GITHUB_APP_CLIENT_ID` | The Client ID. |
| `GITHUB_APP_CLIENT_SECRET` | The client secret. |
| `GITHUB_APP_PRIVATE_KEY` | The whole `.pem`, in double quotes across its lines, or on one line with each line break written as `\n`. |

A sign-in that GitHub refuses is logged as `GitHub sign-in failed` with its
stage. `membership` with a status 302 or 422 means the installation lacks
Members: read; a 404 on the installation means the App is not installed on
the organization.

## API

| Method | Path | Who | Notes |
|--------|------|-----|-------|
| GET | `/health` | anyone | liveness |
| GET | `/auth/github/login` | anyone | starts OAuth; `?redirect=` must be an allowed origin |
| GET | `/auth/github/callback` | anyone | non-members return with `?auth=denied`, banned operators with `?auth=banned` |
| POST | `/auth/guest` | anyone | guest login with `app_id` and `key`; 20 per minute per IP |
| POST | `/auth/logout` | session | removes the session cookie |
| GET | `/auth/me` | session | current operator; a guest must pass its `?app=` |
| GET | `/auth/guest-keys` | staff | today's key for each opted-in tool |
| POST | `/auth/guest-keys/:app/rotate` | staff | replaces today's key of one tool, which signs out its guests |
| GET | `/apps`, `/permissions` | member | the catalog |
| POST | `/apps` | superuser | registers a tool |
| GET | `/me/grants` | member | my grants; `full_access` is true for staff |
| POST, DELETE | `/grants` | staff | grants or revokes one permission |
| GET | `/org/members` | staff | every operator who has signed in |
| GET | `/org/members/:id/grants` | staff | one member's grants |
| POST | `/org/members/:id/ban`, `/unban` | staff | a ban refuses the member at once |
| PATCH | `/org/members/:id/manager` | superuser | sets the manager flag, effective at once |
| GET | `/v1/check?app=&permission=` | session | allow or deny |

Member means a signed-in operator who is not a guest; a guest gets `403`.
Staff means a manager or a superuser. Every mutating route requires the
`X-Requested-With` header, which blocks cross-site form posts. Nobody can ban
or change themself or the guest account. Only a superuser can ban a superuser.

A superuser is whoever `SUPERUSER_GITHUB_IDS` lists, read on every request:
remove an id and redeploy, and that operator's next request is no longer a
superuser's. The `is_superuser` column only mirrors the list for other
readers of the table, such as Sprinter; Auth sets it from the list when it
starts and at each sign-in.

### Sessions

The cookie is `oauth_session`: a JWT signed with HMAC-SHA256 and
`SESSION_SIGNING_KEY`, with its own issuer and audience, and it lasts
`SESSION_TTL_HOURS` (a guest's ends at the next Eastern midnight). There is
no session table: the signature is the check. The cookie names only the
operator's GitHub id, and for a guest its tool, the day, and an HMAC of the
key it signed in with. Roles, bans, grants, and the daily keys are read
from Postgres through an in-memory cache of at most 30 seconds, and every
grant, revoke, ban, and manager change clears that operator from the cache,
and every key rotation that tool's keys, so it applies to the very next
request. The sign-in state cookie,
`oauth_state`, is signed the same way for another audience and also carries
the origin to return to and the PKCE verifier, whose S256 challenge goes to
GitHub, so a stolen code is useless without this browser's cookie. Its path
is the folder of `OAUTH_CALLBACK_URL` (`/api/auth/github` behind the edge,
`/auth/github` locally), so it is sent only to the login and the callback.

What that means in practice:

- A ban refuses the operator's next request on every device. An unban lets
  their unexpired cookies in again.
- A guest session is checked on every request against its tool's current
  key. Rotating the key in the Keys tab, or turning the tool's daily key off
  in `apps.daily_key_enabled`, refuses every guest of that tool at their next
  request (a change by hand in psql within 30 seconds).
- Sign-out removes the cookie from that browser only. A copy of the cookie
  stays valid until it expires; ban the operator to end it everywhere.
- Changing `SESSION_SIGNING_KEY` signs everyone out.
- Membership of the organization is asked again, with the App's
  installation token, at most every 15 minutes per operator. Someone removed
  from `BioTronDesignTeam` is refused with `401` within 15 minutes, on
  `/v1/check` too. When GitHub cannot answer and the last answer is older
  than that, the request fails closed with `503`. The check goes by login, so
  an operator who renames their GitHub account signs in again. A sign-in
  records its own answer. Guests have no GitHub identity and are not checked.

Sessions from before signed cookies were opaque tokens in a `sessions`
table. Those cookies no longer validate, so everyone signs in once more
after the upgrade, and `20261025130000_drop_sessions` dropped the table.

### Permission check

Tools call `GET /v1/check?app=<id>&permission=<key>` with the operator's
cookie. The status code carries the verdict:

| Situation | Response |
|---|---|
| No cookie, or an unsigned, expired, or banned session, or an operator no longer in the organization | `401` |
| GitHub cannot confirm the membership, and no answer of the last 15 minutes is kept | `503` |
| `app` or `permission` missing | `400` |
| Valid session | `200` with `{"allowed": true}` or `{"allowed": false}` |

The route never answers `403`. Callers must still treat a `403` as "signed
in but not permitted", so that a stricter middleware can be added later
without sending anyone into a sign-in loop. A ban applies to the next
check, which refuses.

### Events

Auth reports to Grafana and PostHog as `oauth-manager`, through
`go/logclient`. Every completed request is one event. `/health` is never
logged, and `/v1/check` and `/auth/me` are logged only when they fail,
because every page polls them. A request made with a session names its
operator as `actor`.

| Message | Level | Payload |
|---|---|---|
| `OAuthManager started` | info | `port` |
| `OAuthManager stopping` | info | — |
| `HTTP request completed` | info, warning on 4xx, error on 5xx | `method`, `path`, `status`, `duration_ms`, `actor`, `error` |
| `Operator signed in` | info | `operator_id`, `operator_login`, `new_operator` |
| `Operator signed out` | info | `operator_id`, `operator_login` |
| `Sign-in refused` | warning | `reason` (`not a member` or `banned`), `login` when known |
| `GitHub sign-in failed` | error | `stage` (`exchange`, `user`, `membership`), `error` |
| `Guest signed in` | info | `app_id` |
| `Guest key refused` | warning | `app_id` |
| `Permission granted`, `Permission revoked` | info | `target_id`, `app`, `permission` |
| `Operator banned`, `Operator unbanned` | info | `target_id`, `target_login` |
| `Manager flag set`, `Manager flag removed` | info | `target_id`, `target_login` |
| `Tool registered` | info | `app_id` |
| `Guest key rotated` | info | `app_id` |
| `Stale guest keys pruned` | info | `count` |
| `Guest key prune failed` | error | `error` |

The grant, revoke, ban, unban, manager, tool, and key rotation rows are the
audit trail.
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
  Rotate opens a confirmation dialog, then replaces today's key and signs out
  every guest who used the old one.
- **Org** (staff). Every operator who has signed in. Select one to see and
  change their grants. Ban and the manager flag open a confirmation dialog
  that says what will happen. The manager flag is superuser-only. A staff
  member shows "Full access" and no grant form.

`VITE_API_BASE` is the only frontend variable. It is compiled into the
bundle; Compose passes it to `auth-web` as a build argument.

## Database

Prisma 6.19.3, pinned exactly. Five tables: `operators`, `guest_keys`,
`apps`, `permissions`, and `grants`. Tools and their permissions are rows
that the migrations seed, so `auth-migrate` is what makes a fresh database
usable. Without it, every permission check for a
non-staff operator answers `false`.

| Tool | Permissions |
|---|---|
| `exo-gui` | `live`, `historical`, `commands` |
| `calendar` | `write` |
| `sprinter` | `admin` |

A guest session from Exo's key holds `exo-gui/live` and `exo-gui/historical`
and nothing else.

To create a migration in the devcontainer, source `apps/auth/.env` and run
`infra/migrate/migrate auth dev --name <name>` from the repository root.
The `auth-migrate` service applies the migrations with the shared migrate
image, `infra/migrate`.

### Queries

The backend's SQL lives in `backend/internal/store/queries/queries.sql`.
sqlc 1.31.1 turns it into Go in the same folder, reading the schema straight
from the Prisma migrations, so a migration and a query cannot disagree
without the generate step failing. After changing a query or adding a
migration, regenerate and commit the result:

```bash
cd apps/auth/backend
docker run --rm -v "$PWD/..:/src" -w /src/backend sqlc/sqlc:1.31.1 generate
```

`internal/store/store_live_test.go` runs every query against a real
database. Set `AUTH_TEST_DATABASE_URL` to a throwaway database with the
migrations applied, such as the devcontainer's with `?schema=oauth`, and run
`go test ./internal/store/ -run Live -v`. Without it the tests skip.
