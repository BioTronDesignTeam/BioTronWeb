# OAuthManager

Central GitHub OAuth + per-app permissions for BioTron tools. Operators sign in
with GitHub (org membership); tool access is granted via `read` / `write` /
`admin` rows. Privileged users approve or deny access requests in the UI.

Guests can sign in with a daily rotating key (Eastern midnight). Staff copy
today's key from the Org tab. Guest sessions expire when the key rotates.

Other tools (exo-gui, …) sign in through this API. GitHub login accepts
`?redirect=<origin>` when that origin is in `FRONTEND_URL` or `CORS_ORIGINS`.

## Layout

| Path | What |
|------|------|
| `prisma/` | Schema + migrations |
| `backend/` | Go Fiber API (air live-reload in Compose) |
| `frontend/` | Vite + React UI (HMR in Compose) |
| `docker-compose.yml` | Migration, API, and web containers on the shared `biotron` network |

## Quick start

```bash
cp ../Server/.env.example ../Server/.env
# Replace the shared Postgres password, then start infrastructure once.
(cd ../Server && docker compose up -d)

cp .env.example .env
cp backend/.env.example backend/.env
# Use the same Postgres password and fill GitHub OAuth settings.

docker compose up --build
```

- UI: http://localhost:5173  
- API: http://localhost:18080/health  

For live development, reopen the repository in its devcontainer and run the
Vite and Go processes directly. The root Compose file builds deployment-shaped
containers and does not create another Postgres or Redis instance.

Register a GitHub OAuth App under BioTronDesignTeam with callback
`http://localhost:18080/auth/github/callback`. Put your GitHub numeric user id in
`SUPERUSER_GITHUB_IDS` so the first login can approve requests.

Default host ports: API **18080** and UI **5173**. Shared Postgres and Redis are
owned by `Server`.
