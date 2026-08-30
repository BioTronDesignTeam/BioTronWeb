# BiotronCalendar

Subscribe-able calendars for BioTron events (Google/Apple feeds).

## Layout

| Path | What |
|---|---|
| `frontend/` | Public React + TypeScript calendar UI |
| `backend/` | Go/Fiber feed and event API |
| `prisma/` | Calendar-owned Postgres schema and migrations |
| `.devcontainer/` | Node 22 + Go 1.23 development environment |
| `docker-compose.yml` | Calendar containers on the shared `biotron` network |

## Start the scaffold

Start the shared Postgres and Redis containers from `../Server`, copy
`.env.example` to `.env`, replace the example password, then run:

```bash
docker compose up --build
```

The initial backend exposes `GET /health`. Calendar persistence and feed models
remain intentionally uncommitted until that behavior is implemented.
