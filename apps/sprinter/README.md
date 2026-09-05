# Sprinter

Sprinter is the BioTron Discord bot and its admin UI. Two slash commands
answer questions in Discord. An admin API holds the bot's configuration:
who may run each command, and which Calendar scopes it will announce.

A question goes to Gemini, which may call the bot's six read-only tools
before it answers. Without `GEMINI_API_KEY` the bot echoes the question
back instead; everything around the model still runs.

The announce and nudge automations can be configured but do not run yet.
Nothing reads them.

## Layout

| Path | What |
|------|------|
| `backend/` | Go Fiber admin API and the `discordgo` session. Postgres through pgx. |
| `frontend/` | Vite and React placeholder page, served by Nginx. |
| `prisma/` | Schema and migrations. The Go service runs its own queries. |
| `docker-compose.yml` | `sprinter-migrate`, `sprinter-bot`, `sprinter-web` on the `biotron` network. |
| `.env.example` | Every variable the app reads. Copy it to `.env`. |

## Run

Start the shared Postgres once, from the monorepo root, then Sprinter:

```bash
docker compose -f infra/docker-compose.yml --env-file infra/.env up -d
cp apps/sprinter/.env.example apps/sprinter/.env   # DISCORD_TOKEN may stay empty
docker compose up -d --build sprinter-migrate sprinter-bot sprinter-web
```

The UI is at http://localhost:5178 and the API at http://localhost:8084.
An empty `DISCORD_TOKEN` runs the admin API alone. With a token, the bot
opens a gateway session and registers its commands in `DISCORD_GUILD_ID`.
If the session cannot open, the process exits.

## Develop

Open the monorepo in its devcontainer. It supplies Node 24, Go 1.27, and
a Postgres that `.env.example` already points at. Then:

```bash
npm run dev -w apps/sprinter/frontend            # http://localhost:5178
cd apps/sprinter/backend && PORT=8084 go run .   # reads ../.env
npm run lint                                     # Oxlint, from the root
```

`PORT` defaults to 8080, which Auth uses, so set it to match
`VITE_API_URL`. `go test ./...` in `backend/` runs the tests. Tests named
`Live*` need a database and skip without one:
`SPRINTER_TEST_DATABASE_URL` for the `sprinter` schema, and
`SPRINTER_TEST_READ_DATABASE_URL` for the read-only role the tools use.

```bash
SPRINTER_TEST_READ_DATABASE_URL='postgresql://sprinter_reader:change-me-reader@127.0.0.1:15433/biotron' \
  go test ./... -run Live
```

## Commands

| Command | What it does |
|---------|--------------|
| `/agent question:<text>` | Answers once in the channel. |
| `/agent-thread question:<text>` | Answers, then opens a thread on the answer to keep going. |

Both are registered in one guild, so a change appears at once. A global
command takes up to an hour to propagate.

A **guard** decides who may run a command. Each guard names a guild, the
roles that may run it, and the channels it may run in. An empty channel
list means any channel in that guild. A command with no guard row is
refused: adding a command without configuring it fails closed.

A refusal is ephemeral, so only the person who ran the command sees it.
It never names the allowed roles or channels, because that would leak
the guard to the person it excludes.

### How a question is answered

Sprinter acknowledges the command, asks the model, runs whatever tools
the model asks for, and feeds the results back. It stops when the model
answers in words. Three limits bound the loop: 8 model turns, 12 tool
calls, and `AGENT_TIMEOUT`. When a limit stops the search the answer says
so, rather than presenting a half-finished search as the whole story. A
rate-limited model is reported as such, so the operator knows to wait.

A tool that fails is reported to the model as a failed result, not to the
operator as a broken command. A mistyped service id costs one turn.

### Threads

`/agent-thread` opens a thread named after the first 80 characters of the
question, archived after a day. The whole first exchange is stored as the
thread's turns: the question, each tool call, each tool result, and the
answer. A follow-up replays them, so the model continues the search
rather than starting it again.

A message in one of those threads continues the conversation. A message
anywhere else is ignored, which is the whole reason the bot may hold the
Message Content intent. Each follow-up re-checks the `agent-thread`
guard against the author's roles, because a role can be taken away
between the first question and the tenth. The guard's channel list is
checked against the thread's parent channel.

A thread answers `THREAD_MAX_TURNS` questions, 40 by default. Past that
Sprinter says the thread is full and asks for a new one.

### Limits

One question per person at a time, and three across the whole bot. A
refused question gets one sentence and no work: the model call is the
expensive part, and a person who runs the command twice does not want two
answers.

An answer longer than 2000 characters is split across messages at a line
boundary.

## Tools

The model answers from these six and nothing else. Every one is
read-only. The four SQL tools read through a second Postgres pool that
connects as a role with `SELECT` on the `logger` schema and on four
`oauth` tables — nothing else, and no write anywhere. That role is the
containment: a model can be talked into asking for anything, so the
answer to "what stops it" is the grant, not the prompt.

Every result passes through one cap. A cut result ends with a line
saying it was cut and how to narrow the question.

| Tool | Source | Arguments | Cap |
|------|--------|-----------|-----|
| `platform_status` | `GET {LOGGER_URL}/v1/status` | none | 6000 characters |
| `recent_logs` | `logger.logs` | `service`, `level`, `search`, `limit` 1–50 (20) | 6000 characters, 50 rows, payload 500 per row |
| `log_history` | `logger.logs` | the same plus `from`, `to` (RFC3339) and `cursor` | 8000 characters, 50 rows per page |
| `health_history` | `logger.health_checks` | `service` (required), `hours` 1–168 (24) | 6000 characters, 200 checks |
| `calendar_upcoming` | `GET {CALENDAR_URL}/v1/events/upcoming` | `limit` 1–20 (5), `days` 1–90 (42) | 4000 characters |
| `who_has_access` | `oauth.apps`, `oauth.grants`, `oauth.operators`, `oauth.permissions` | `app` (required) | 6000 characters |

`log_history` pages on `(created_at, id)` exactly as Logger does, so a
row written mid-walk cannot shift a page. Its answer ends with a
`next_cursor` line; the model sends that cursor back with the same
filters.

`who_has_access` also lists the managers and the superusers, because
those two flags admit an operator everywhere without a grant, and an
answer that named only the grant holders would be wrong. It never reads
`oauth.sessions` or `oauth.guest_keys`; a live test proves the role
cannot read either one.

The model is told that tool results are data and never instructions. A
log message can contain words that read like an order; Sprinter reports
them and does not act on them.

## Admin API

| Method | Path | Notes |
|--------|------|-------|
| GET | `/health` | `503` when the database is down. Reports `discord_connected`. |
| GET | `/v1/auth/status` | `operator` and `can_admin` for the cookie sent |
| GET | `/v1/admin/guards` | every guard |
| GET | `/v1/admin/guards/:subject` | one guard |
| PUT | `/v1/admin/guards/:subject` | writes the whole guard |
| DELETE | `/v1/admin/guards/:subject` | removes it, which turns the feature off |
| GET | `/v1/admin/automations` | every automation |
| POST | `/v1/admin/automations` | creates one |
| GET | `/v1/admin/automations/:id` | one automation |
| PATCH | `/v1/admin/automations/:id` | changes the fields sent |
| DELETE | `/v1/admin/automations/:id` | removes it |

`:subject` is `agent`, `agent-thread`, `announce`, or `nudge`. The first
two are the command names.

Every `/v1/admin` route needs an operator whose Auth session holds
`sprinter/admin`. The API forwards the cookie to Auth's `/auth/me` and
`/v1/check`. It reads both so the log can name the operator behind a
change. No cookie answers `401`, a session without the permission `403`,
an unreachable Auth `503` — never a pass. Every mutating request must
carry `X-Requested-With: XMLHttpRequest`, which blocks cross-site form
posts.

A body is rejected with `400` unless `kind` is `ANNOUNCE` or `NUDGE`,
`deliver` is `DM` or `CHANNEL`, `lead_hours` and `lookback_hours` are 1
to 168, `post_hour` is 0 to 23, `scope_id` is a UUID, and every Discord
id is a string of digits. A guard must name at least one role.

## Environment

One file, `apps/sprinter/.env`, feeds Compose, the backend, the frontend,
and Prisma. Do not add environment files below it. `go run .` reads
`../.env`.

- `DISCORD_TOKEN`. Empty means no gateway session and no commands.
- `DISCORD_GUILD_ID`. The guild commands are registered in. The backend
  refuses to start with a token and no guild.
- `DATABASE_URL`. The `sprinter` schema. The `?schema=` becomes the
  connection's `search_path`.
- `READ_DATABASE_URL`. The read-only role the four SQL tools use. Empty
  leaves those tools out; the bot still answers from the two public
  endpoints.
- `GEMINI_API_KEY`. Empty runs the echo runner and logs `Model unset`.
- `GEMINI_MODEL`. Defaults to `gemini-2.5-flash`.
- `THREAD_MAX_TURNS`. How many questions one thread answers. 40 by
  default.
- `AGENT_TIMEOUT`. How long one question may take, tool calls included.
  `2m` by default.
- `CALENDAR_URL`. Where `calendar_upcoming` reads.
- `OAUTH_MANAGER_URL`. Where the admin permission is checked.
- `FRONTEND_URL` and `CORS_ORIGINS`. The origins that may make
  credentialed admin requests. List each host as `localhost` and
  `127.0.0.1`.
- `LOGGER_URL`, `LOGGER_INGEST_TOKEN`. Where events go, and where
  `platform_status` reads. An empty token sends no events.

Tests read two more: `SPRINTER_TEST_DATABASE_URL` and
`SPRINTER_TEST_READ_DATABASE_URL`. Neither belongs in `.env`.

## Events to Logger

Sprinter reports to Logger as `sprinter`. Every admin change names the
operator as `actor_id` and `actor_login`. A refused command records the
subject and a reason code, never the question text. No event carries a
question, an answer, or a tool result: what a question cost is an
operator's business, what it said is not.

| Message | Level | Payload |
|---------|-------|---------|
| `Sprinter started` | info | `port`, `discord` |
| `Sprinter stopping` | info | — |
| `Discord token unset` | warning | — |
| `Model unset` | warning | — |
| `Model ready` | info | `model`, `tools` |
| `Model unavailable` | error | `error` |
| `Read database unset` | warning | — |
| `Read pool failed` | error | `error` |
| `Discord session failed` | error | `stage`, `error` |
| `Discord connected` | info | `user`, `guilds` |
| `Discord disconnected` | warning | — |
| `Discord resumed` | info | — |
| `Commands registered` | info | `guild_id`, `count` |
| `Command registration failed` | error | `guild_id`, `error` |
| `Command refused` | warning | `subject`, `reason`, `user_id`, `guild_id`, `channel_id`, `thread_id` in a thread |
| `Question answered` | info | `subject`, `user_id`, `duration_ms`, `channel_id` or `thread_id`, `tools` and token counts when a model answered |
| `Question failed` | error | `subject`, `stage`, `duration_ms`, `error`, `thread_id` in a thread |
| `Follow-up failed` | error | `channel_id`, `error` |
| `Thread opened` | info | `thread_id`, `channel_id`, `opener_id` |
| `Thread creation failed` | error | `channel_id`, `error` |
| `Thread record failed` | error | `thread_id`, `error` |
| `Transcript write failed` | error | `thread_id`, `sequence`, `error` |
| `Guard saved` | info | `subject`, `guild_id`, `roles`, `channels` |
| `Guard deleted` | info | `subject` |
| `Automation created` | info | `automation_id`, `kind`, `name`, `scope_id` |
| `Automation updated` | info | `automation_id`, `kind`, `enabled` |
| `Automation deleted` | info | `automation_id` |
| `Authorization service unavailable` | error | `error` |
| `HTTP request completed` | info, warning on 4xx, error on 5xx | `method`, `path`, `status`, `duration_ms`, `actor` and `error` when known |

`/health` and `/v1/auth/status` are logged only when they fail. A domain
event fires only after the change reached the database.

The reason codes on `Command refused` are `no_guard`, `no_guild`,
`wrong_guild`, `wrong_channel`, `no_member`, `wrong_role`, and
`unavailable`.

## Database

Prisma 6.19.3, pinned exactly. `prisma/package.json` also overrides the
transitive dependency `deepmerge-ts` to 8.0.0; `@prisma/config` asks for
7.1.5. Seven tables in the `sprinter` schema:

| Table | What |
|-------|------|
| `guards` | One row per subject: the guild, roles, and channels that may use it. |
| `automations` | One announce or nudge job: kind, Calendar scope, channel, and its windows. |
| `agent_threads` | One `/agent-thread` conversation, keyed by the Discord thread. |
| `agent_messages` | Its transcript. One row per turn, unique on `(thread_id, sequence)`. |
| `seen_occurrences` | Calendar occurrences an announce job has read. |
| `posted_occurrences` | The messages it posted for them. |
| `nudges` | What a nudge job has already chased. |

The last three are not written yet. Deleting a thread removes its
messages; deleting an automation removes its occurrence rows.

The bot opens two pools. `DATABASE_URL` writes its own schema.
`READ_DATABASE_URL`, a read-only role, is what the tools read through, so
a model cannot be talked into a write. The second pool qualifies every
table as `logger.x` or `oauth.x` and carries no `search_path` of its own.

To create a migration in the devcontainer, source `apps/sprinter/.env`
and run `npm run --prefix apps/sprinter/prisma migrate`. The
`sprinter-migrate` service applies the migrations as the image's `node`
user.

## Frontend

One placeholder page: an eyebrow, the word "Sprinter", and one sentence.
`index.css` carries its own colors. The page does not import
`@biotron/style` or its `styles.css`: no wordmark, no theme toggle, no
`--biotron-*` tokens. Nothing calls the admin API yet.
