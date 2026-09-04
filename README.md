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
- API: http://localhost:8080/health  

For live development, reopen the repository in its devcontainer and run the
Vite and Go processes directly. The root Compose file builds deployment-shaped
containers and does not create another Postgres or Redis instance.

The repository has one environment file at its root. Compose, the backend, the
frontend, and Prisma all use values from that file; do not create
component-level environment files. Leave `COOKIE_DOMAIN` blank for localhost;
set it to the shared parent domain (for example `.biotron.ca`) when products
are deployed on sibling subdomains so the OAuth session reaches each tool.

The API sends lifecycle and completed-request events to Logger as
`oauth-manager`. Set `LOGGER_INGEST_TOKEN` to the same shared secret used by
Logger; leaving it empty disables structured delivery without preventing Auth
from starting. Request metadata never includes query strings, cookies, or
credentials.

Register a GitHub OAuth App under BioTronDesignTeam with callback
`http://localhost:8080/auth/github/callback`. Put your GitHub numeric user id in
`SUPERUSER_GITHUB_IDS` so the first login can approve requests.

Default host ports: API **8080** and UI **5173**. The GitHub OAuth callback is
registered against `8080`, so this API port is fixed while the other BioTron
services take the ports beside it.

| Service | Frontend | API |
| --- | --- | --- |
| OAuthManager | `5173` | `8080` |
| Exo (`exo-gui`) | `5174` | `8081` |
| Logger | `5175` | `8082` |
| BiotronCalendar | `5176` | `8083` |
| Biotron site | `5177` | n/a |
| Sprinter | `5178` | `8084` |

`CORS_ORIGINS` must list every product frontend in both its `localhost` and
`127.0.0.1` form, because browsers treat those as different origins.

Shared Postgres and Redis are owned by `Server`.
