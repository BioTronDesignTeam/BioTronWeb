# OAuthManager

Central GitHub OAuth + per-app permissions for BioTron tools. Operators sign in
with GitHub (org membership); each product defines its own named capability
catalog. Privileged users approve or deny access requests in the UI.

Products can opt into independently generated daily guest keys that rotate at
Eastern midnight. Managers and superusers reveal or copy them from the Keys
tab. Guest sessions expire with the key and are restricted to the product that
issued them. Exo GUI is currently the only product with daily keys enabled.

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
# Use the same Postgres password and fill GitHub OAuth settings in this file.

docker compose up --build
```

- UI: http://localhost:5173  
- API: http://localhost:18080/health  

For live development, reopen the repository in its devcontainer and run the
Vite and Go processes directly. The root Compose file builds deployment-shaped
containers and does not create another Postgres or Redis instance.

The repository has one environment file at its root. Compose, the backend, the
frontend, and Prisma all use values from that file; do not create
component-level environment files.

Register a GitHub OAuth App under BioTronDesignTeam with callback
`http://localhost:18080/auth/github/callback`. Put your GitHub numeric user id in
`SUPERUSER_GITHUB_IDS` so the first login can approve requests.

Default host ports: API **18080** and UI **5173**. Shared Postgres and Redis are
owned by `Server`.
