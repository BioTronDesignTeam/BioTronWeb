# Sprinter

Sprinter is the BioTron Discord bot and its admin UI. It is meant to announce
Calendar events in Discord, nudge leads, and hold the bot's configuration.
None of that is built. Today the bot opens a Discord session when it has a
token, answers `GET /health`, and does nothing else.

## Layout

| Path | What |
|------|------|
| `backend/` | Go Fiber API and the `discordgo` session. One route. |
| `frontend/` | Vite and React placeholder page, served by Nginx. |
| `prisma/` | A datasource and no models. No migrations yet. |
| `docker-compose.yml` | `sprinter-migrate`, `sprinter-bot`, `sprinter-web` on the `biotron` network. |
| `.env.example` | Every variable Compose reads. Copy it to `.env`. |

## Run

Start the shared Postgres once, from the monorepo root, then Sprinter:

```bash
docker compose -f infra/docker-compose.yml --env-file infra/.env up -d
cp apps/sprinter/.env.example apps/sprinter/.env    # DISCORD_TOKEN may stay empty
docker compose up -d --build sprinter-migrate sprinter-bot sprinter-web
```

The UI is at http://localhost:5178 and the API at http://localhost:8084.
`GET /health` answers `{"discord_connected":false,"service":"sprinter","status":"ok"}`.
With `DISCORD_TOKEN` set, the bot opens a gateway session and reports
`discord_connected: true`. If the session cannot open, the process exits.
The session has no handlers: the bot reads and sends nothing yet.

## Develop

Open the monorepo in its devcontainer. It supplies Node 24, Go 1.27, and a
Postgres that `.env.example` already points at. Then:

```bash
npm run dev -w apps/sprinter/frontend    # http://localhost:5178
cd apps/sprinter/backend && go run .     # http://localhost:8080
npm run lint                             # Oxlint, from the root
```

## Environment

One file, `apps/sprinter/.env`, feeds Compose, Vite, and Prisma. `go run .`
reads no file; export what it needs in the shell. The code reads:

- `DISCORD_TOKEN`. Empty means no Discord session.
- `PORT`. Compose sets `8080`; `go run .` defaults to it.
- `DATABASE_URL`. Prisma's connection, in the `sprinter` schema.

Nothing reads `CALENDAR_URL`, `OAUTH_MANAGER_URL`, or `VITE_API_URL` yet.

## Database

Prisma 6.19.3, pinned exactly. `prisma/package.json` also overrides the
transitive dependency `deepmerge-ts` to 8.0.0; `@prisma/config` asks for
7.1.5. `schema.prisma` holds the datasource and no models. There is no
`migrations/` folder, so `sprinter-migrate` finds nothing to apply and exits.
The `sprinter` schema holds only Prisma's own `_prisma_migrations` table.

To create the first migration in the devcontainer, source `apps/sprinter/.env`
and run `npm run --prefix apps/sprinter/prisma migrate`.

## Frontend

One placeholder page: an eyebrow, the word "Sprinter", and one sentence.
`index.css` carries its own colors. The page does not import `@biotron/style`
or its `styles.css`: no wordmark, no theme toggle, no `--biotron-*` tokens.
