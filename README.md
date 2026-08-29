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
| `docker-compose.yml` | Postgres + Redis + migrate + api + web |

## Quick start

```bash
cp backend/.env.example backend/.env
# fill GITHUB_CLIENT_ID / SECRET and SUPERUSER_GITHUB_IDS

docker compose up --build
```

- UI: http://localhost:5173  
- API: http://localhost:18080/health  

Edit code on the host — frontend hot-reloads via Vite; backend rebuilds via [air](https://github.com/air-verse/air) when `.go` files change. No separate dev profile.

Register a GitHub OAuth App under BioTronDesignTeam with callback
`http://localhost:18080/auth/github/callback`. Put your GitHub numeric user id in
`SUPERUSER_GITHUB_IDS` so the first login can approve requests.

Host ports on this box: API **18080** (8080 is already used), Postgres **5434**, Redis **6379**, UI **5173**.
