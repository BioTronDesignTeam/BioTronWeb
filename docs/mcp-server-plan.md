# New repository: MCP debugging server

Status: proposal. Nothing is built. Four decisions below are open.

This document plans a new sibling repository in the BioTron workspace: a Go MCP
server that gives agents read-only access to the platform for debugging. Claude
Code and Cursor connect to it directly. Sprinter, the agentic Discord bot,
connects to it as a client so people can ask questions from Discord.

---

## 1. Decisions still open

| # | Decision | Recommendation | Why it is yours |
|---|---|---|---|
| 1 | Repository name | `Probe` | House style is a plain noun. Hostname `mcp.<domain>`, container alias `probe-api`. |
| 2 | Host API port | `8085` | `AGENTS.md` reserves `8080`-`8084` and says the table moves only when all six repositories move together. Adding a seventh row edits a shared document. |
| 3 | Auth path | Cloudflare Access now, OAuthManager tokens next | Section 3. The cheap option and the correct option differ, and the gap is weeks of work. |
| 4 | Tool list | Draft from Logger and OAuthManager first | You know what you actually debug. |

---

## 2. What the service is

One Go binary. No frontend. It speaks MCP over Streamable HTTP on a public
hostname behind the existing Cloudflare Tunnel and Nginx edge.

Every tool is a read. Nothing writes to Postgres, nothing writes to Redis,
nothing changes permissions. That constraint is the whole point of the design
and Section 4 explains how to enforce it.

The repository owns no Postgres schema. It reads schemas that other
repositories own. This is new for the workspace and Section 5 covers the risk.

---

## 3. Authentication

### The problem

MCP's authorization spec expects the server to be an OAuth 2.1 resource server.
It validates bearer tokens issued by an authorization server and advertises that
server through protected-resource metadata.

OAuthManager is not an authorization server. It is only an OAuth client to
GitHub. It has no `/authorize`, no `/token`, no client registry, no PKCE for
downstream clients, and no metadata documents. What it issues is an opaque
`oauth_session` cookie. Every other service authorizes by forwarding that cookie
to `GET /v1/check?app=<id>&permission=<key>`.

That design assumes a browser. Neither caller has one. Claude Code can send
static headers. Sprinter is a daemon.

### The options

**A. Shared static bearer token.** Copy the `LOGGER_INGEST_TOKEN` pattern. One
secret in the environment, constant-time compare. About a day of work and no
change to OAuthManager. Every call is anonymous, revocation means rotating the
secret and updating every client, and one leaked string opens the whole log
warehouse to the internet.

**B. Cookie forwarding.** Accept `oauth_session` and proxy it to `/v1/check`.
Real per-user identity and no new concepts. It fails in practice: the cookie is
HttpOnly, so you extract it from devtools, paste it into config, and repeat every
seven days. Sprinter cannot use it at all.

**C. MCP tokens minted by OAuthManager.** Add an `mcp_tokens` table, store
`sha256(token)`, show the plaintext once, and teach `/v1/check` to accept
`Authorization: Bearer` alongside the cookie. Sprinter gets a token bound to a
service operator row. This reuses the existing `Allowed()` decision unchanged, so
grants, staff bypass and bans keep working. Cost is one real pull request against
OAuthManager: migration, handler, small UI.

**D. Make OAuthManager a full authorization server.** Spec-correct, and Claude
Code would do a browser login with no pasted secrets. It is also seven missing
components and a change of role for the service that gates every internal tool.
Weeks, not days.

**E. Cloudflare Access at the edge.** Access gates `mcp.<domain>` with GitHub SSO
for humans and service tokens for Sprinter, then passes a signed
`Cf-Access-Jwt-Assertion` header the Go code verifies. Authenticated identity
without writing an identity provider.

### The recommendation

**E now, C next, never D.**

Access closes an unauthenticated public endpoint immediately, which matters
because the hostname is public from day one. C then gives per-user identity and
individual revocation inside your own system.

Verify the free Cloudflare Zero Trust tier covers your user count before
committing to E.

### Constraints that hold regardless of choice

- Registering an `mcp` app with a permission key **needs a migration inside the
  OAuthManager repository**. No endpoint creates `permissions` rows, and a check
  against a missing permission answers `allowed: false`.
- `Allowed()` returns true for any superuser or manager on any app and any
  permission, before grants are consulted. Whatever key you define, every manager
  already has it.
- OAuthManager sessions live in Postgres, not Redis. Redis holds only
  `grants:<operator_id>` with a 300 second TTL. Any design that reads sessions
  from Redis does not work.
- `/v1/check` is uncached Postgres on every call.

### Implementation shape

Put the check behind an `Authorizer` interface, the way `Logger` already does in
`backend/internal/auth/oauth.go`. Moving from E to C is then one file.

Copy the fail-closed posture from `exo-gui/backend/internal/auth/middleware.go`.
A transport error, an unset `OAUTH_MANAGER_URL`, or an unexpected status all
return 503. Never a pass.

---

## 4. Read-only, enforced in the database

Do not rely on the Go code to avoid writes. Enforce it where it cannot be
bypassed.

- Create a dedicated Postgres role for this service. Grant `SELECT` only.
- `ALTER ROLE <role> SET default_transaction_read_only = on`.
- Give Redis its own ACL user limited to `+@read`.

A bug in a tool handler then cannot write, whatever the code does.

This is cheap to do now and very hard to retrofit convincingly later.

---

## 5. The coupling risk

Every existing repository owns its Postgres schema. This one owns none. It reads
`oauth`, `logger`, and whatever exo and calendar hold.

A migration in Logger can therefore break a tool here, and nothing in Logger's
test suite would catch it.

Three ways to handle it:

1. **Call the existing HTTP APIs.** Loose coupling and loud failures, but
   limited. Logger deliberately strips `Health.Detail` from every public
   response because it leaks internal hostnames and ports, and its
   `internal/store` package is unexported and not importable.
2. **Direct read-only SQL.** Full access, which is what makes a debugging tool
   worth having. Tight coupling to schemas you do not control.
3. **Both, split by intent.**

**Recommend 3.** Use the HTTP APIs where they already answer the question. Use
direct SQL for everything else, and mark those tools as best-effort in the
README so a broken tool after a migration surprises nobody.

### What the existing APIs already give you

From Logger, without touching its tables:

| Endpoint | Auth | Gives |
|---|---|---|
| `GET /v1/status` | none | Overall status, 24h/7d/90d uptime per application |
| `GET /v1/status/history` | none | Daily uptime buckets, up to 90 days |
| `GET /v1/apps` | `logger/view` | Applications, components, current state |
| `GET /v1/apps/:app/logs/recent` | `logger/view` | Redis tail, level and text filters |
| `GET /v1/apps/:app/logs/history` | `logger/view` | Postgres history, keyset paginated |

### What needs direct reads

- `health_checks.detail`, stripped from every Logger response by design.
- Anything in the `oauth` schema: sessions, grants, access requests, app catalog.
- Redis introspection beyond what Logger exposes. Key shapes are
  `logger:logs:<service>` (capped list) and `logger:health:<service>` (string,
  no TTL).

---

## 6. Transport

Streamable HTTP, not stdio, because the server is public.

The official Go SDK is stable at `v1.7.0` and covers it. Protocol revision
`2026-07-28` is stateless with per-request `_meta`, which suits a proxied
deployment: no session affinity to maintain behind Nginx.

---

## 7. Repository scaffold

All of this follows existing convention. None of it needs a decision.

### Layout

```
<Name>/
├── .devcontainer/
│   ├── Dockerfile
│   ├── devcontainer.json
│   └── post-create.sh
├── backend/
│   ├── .dockerignore
│   ├── .gitignore
│   ├── Dockerfile
│   ├── go.mod
│   ├── go.sum
│   ├── main.go
│   └── internal/
│       ├── auth/
│       ├── config/
│       ├── mcp/
│       └── store/
├── .env.example
├── .gitignore
├── README.md
└── docker-compose.yml
```

Keep the `backend/` subdirectory even without a frontend. Every sibling has it,
compose builds from `./backend`, and it leaves room to grow.

Module path is `github.com/BioTronDesignTeam/<Name>/backend`, Go 1.27. `main.go`
sits directly in `backend/`. There is no `cmd/` directory anywhere in this
workspace; do not introduce one.

### Devcontainer

Reuse the existing three-file pattern. The Dockerfile is byte-identical across
Sprinter, BiotronCalendar and Logger, and it supplies the `node` user the
devcontainer expects. Keep it unchanged.

Trim `devcontainer.json`: drop the Prisma and Tailwind extensions, keep
`golang.go` and `ms-azuretools.vscode-docker`, forward one port.

Cut `post-create.sh` to the `go mod download` block.

### Backend Dockerfile

Copy Logger's exactly.

- Build stage `golang:1.27-bookworm`, `WORKDIR /src`, copy `go.mod go.sum` first
  for layer caching.
- `CGO_ENABLED=0 go build -trimpath -ldflags="-s -w"`.
- Runtime stage `debian:bookworm-slim` with `ca-certificates` only.
- Non-root user: `useradd --system --uid 10001 app`.
- `EXPOSE 8080`.

### Compose

- `name: biotron-<lowercase>`.
- One service. No `migrate` container, because the repository owns no schema.
- Listens on `8080` internally. Publishes `127.0.0.1:${API_PORT:-8085}:8080`.
- Joins the external `biotron` network with a DNS alias.
- `restart: unless-stopped`.
- No `healthcheck:` block. This house does not use them.

### Edge

Adding `mcp.<domain>` is a copy-paste `server` block in
`Server/nginx/templates/default.conf.template`. The existing
`proxy_read_timeout 3600s` already suits long-lived MCP streams.

### Environment

One `.env.example` at the repository root. One untracked `.env` beside it. No
component-level environment files.

Sections in order: Backend, PostgreSQL, Redis, Host port overrides.

Add a comment saying this repository reads other repositories' schemas and owns
none. Otherwise someone will assume a missing `prisma/` folder.

### Gitignores

Local, never a root mega-file.

- Root: the `.env` block only.
- `backend/`: `bin/`, `*.test`, `*.out`.

### No Makefile, no CI, no lint config

No sibling repository has them. Adding them is the one thing that would make this
repository look foreign. Document verification in the README instead:

```bash
docker run --rm -v "$PWD/backend:/src" -w /src golang:1.27-bookworm go test ./...
docker run --rm -v "$PWD/backend:/src" -w /src golang:1.27-bookworm go vet ./...
```

Tests are colocated `_test.go` files. Prefix any test needing a live dependency
with `Live`, matching BiotronCalendar.

---

## 8. Suggested build order

1. Scaffold the repository. Health endpoint only. Confirm it builds, runs on the
   `biotron` network, and answers on `127.0.0.1:8085`.
2. Add the MCP server with two or three read tools backed by Logger's public
   `/v1/status` endpoints. No auth yet, local only.
3. Add the `Authorizer` interface with a static token implementation. Keep it
   local.
4. Add the `mcp.<domain>` edge block and put Cloudflare Access in front. Test
   with a Cloudflare service token before pointing any client at it.
5. Create the read-only Postgres role and Redis ACL user. Move the direct-SQL
   tools onto them.
6. Point Claude Code at it. Iterate on the tool list against real debugging.
7. Wire Sprinter as a client.
8. Land the OAuthManager migration and `mcp_tokens` work. Swap the `Authorizer`.

Steps 1 through 6 deliver a useful tool. Steps 7 and 8 are separate pieces of
work and should not block it.

---

## 9. Cross-repository changes this implies

Track these, because they touch repositories other than the new one.

- `AGENTS.md`: add the new service to the host port table.
- `Server/nginx/templates/default.conf.template`: add the `mcp` server block.
- `Server`: create the read-only Postgres role and Redis ACL user.
- `Logger`: add a catalog entry so the new service can emit its own health and
  logs.
- `OAuthManager`: migration adding the `mcp` app and its permission key, plus
  the `mcp_tokens` work when step 8 arrives.
- `Sprinter`: MCP client, in step 7.

---

## 10. Open questions worth answering before step 2

- Which failures do you actually debug most? That decides the first tool list.
- Should the tools read exo telemetry, or only platform data?
- Does Sprinter call the MCP server as itself, or on behalf of the Discord user
  who asked? The second needs identity mapping that does not exist yet.
