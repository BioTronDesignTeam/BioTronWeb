# backend

Go service for the exo telemetry pipeline. HTTP via **Fiber**, Postgres via
**pgx** (hand-written SQL); **Prisma owns the schema** (`../prisma`).

## Operator auth layer (working)

Operators sign in with **GitHub OAuth**, gated on **`BioTronDesignTeam`** org
membership (`read:org`). Sessions are server-side: a random token lives in a
cookie, and only its SHA-256 hash is stored in Postgres.

### Routes

| Method | Path                    | Purpose                                                                 |
|--------|-------------------------|-------------------------------------------------------------------------|
| GET    | `/health`               | Liveness — `{"status":"ok"}`                                            |
| GET    | `/auth/github/login`    | Redirect to GitHub authorize (`read:org` + CSRF `state` cookie)         |
| GET    | `/auth/github/callback` | Exchange code → verify org membership → create session → redirect to SPA |
| POST   | `/auth/logout`          | Delete the session and clear the cookie                                 |
| GET    | `/auth/me`              | Current operator JSON (protected by `RequireSession`)                   |

Non-members are redirected to `FRONTEND_URL/?auth=denied` with no session.

### Guest login (shared daily key)

An alternative to GitHub for when org OAuth isn't available. A random key is
generated per **Eastern** day (`America/Toronto`, DST-aware) and stored; guests
enter it to get a (shared) session with the same access as an operator. The
session expires when the key rotates (next Eastern midnight), so a leaked key is
only good for the rest of the day.

| Method | Path                | Purpose                                                          |
|--------|---------------------|------------------------------------------------------------------|
| GET    | `/admin/guest-key`  | Today's key — requires `X-Admin-Token: $ADMIN_TOKEN` (404 if unset) |
| POST   | `/auth/guest`       | `{"key":"…"}` → validates today's key → guest session (rate-limited) |

Fetch and share today's key:
```bash
curl -H "X-Admin-Token: $ADMIN_TOKEN" http://localhost:8080/admin/guest-key
# {"day":"2026-06-01","key":"ABCD-EFGH-JKLM"}
```
Set `ADMIN_TOKEN` to enable this; in prod the admin endpoint should also only be
reachable over Tailscale. Note: a shared key has no per-person accountability,
and guest sessions currently have full access.

### Layout

```
backend/
├── main.go                 # wire config → store → github → handlers → server
└── internal/
    ├── config/             # env-sourced Config
    ├── store/              # pgx pool + operators/sessions SQL
    ├── auth/               # github oauth, session cookies, handlers, middleware
    └── server/             # Fiber app: CORS, logging, routes (+ integration test)
```

### Configuration

See `.env.example`. Cookie attributes are env-driven because the deployment is
cross-origin in prod but same-site in dev:

- **Local dev** (`:5173` ↔ `:8080`, both `localhost` = same-site): `COOKIE_SAMESITE=Lax`, `COOKIE_SECURE=false` (defaults).
- **Production** (Netlify ↔ `api.` over HTTPS = cross-site): `COOKIE_SAMESITE=None`, `COOKIE_SECURE=true`.

### Register the GitHub OAuth App

1. Org settings → **Developer settings → OAuth Apps → New OAuth App**
   (`https://github.com/organizations/BioTronDesignTeam/settings/applications`).
2. **Homepage URL:** the SPA URL (dev: `http://localhost:5173`).
3. **Authorization callback URL:** must exactly equal `OAUTH_CALLBACK_URL`
   (dev: `http://localhost:8080/auth/github/callback`).
4. Copy the **Client ID** and a generated **Client Secret** into `backend/.env`
   as `GITHUB_CLIENT_ID` / `GITHUB_CLIENT_SECRET`. Never commit `.env`.

### Run (inside the dev container)

```bash
cd prisma  && npm run migrate   # apply operators/sessions (prisma migrate dev)
cd backend && go run .          # :8080 — curl localhost:8080/health
go test ./...                   # integration test (needs DATABASE_URL)
```

Module path: `github.com/BioTronDesignTeam/exo-gui/backend`
