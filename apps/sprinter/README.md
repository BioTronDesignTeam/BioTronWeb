# Sprinter

Discord bot and admin UI for BioTron (calendar announcements, lead nudges, and more).

## Layout

| Path | What |
|---|---|
| `frontend/` | React + TypeScript admin UI |
| `backend/` | Go service using `discordgo`, plus the admin API |
| `prisma/` | Sprinter-owned Postgres schema and migrations |
| `.devcontainer/` | Node 22 + Go 1.27 development environment |
| `docker-compose.yml` | Sprinter containers on the shared `biotron` network |

Start the shared Postgres and Redis containers from `../Server`, copy
`.env.example` to `.env`, set the Postgres password and optionally a Discord bot
token, then run `docker compose up --build`.

The bot, frontend, Prisma, and Compose all use the single root `.env`; do not
create component-level environment files.

Without a Discord token, the scaffold still starts its admin API and reports
`discord_connected: false` from `GET /health`.

Default host ports are `5178` for the admin UI and `8084` for the admin API.
They sit at the end of the shared BioTron block (OAuthManager `5173`/`8080`,
Exo `5174`/`8081`, Logger `5175`/`8082`, Calendar `5176`/`8083`, the public
site `5177`).
