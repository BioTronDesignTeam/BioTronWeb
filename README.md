# BioTron platform

One repository for the BioTron web platform: six products, the shared
component library, the shared Go client, and the edge that serves them. Every
piece arrived with its full history from the repository it came from. This is
where the platform is developed now; the separate repositories are frozen.

## Layout

| Path | What | Was |
|---|---|---|
| `apps/site` | Public team website (React, Three.js) | `Biotron-Site` |
| `apps/calendar` | Public calendars and feeds (Go API, React UI, Prisma) | `BiotronCalendar` |
| `apps/logger` | Status page, log warehouse, health monitor | `Logger` |
| `apps/auth` | GitHub OAuth, sessions, per-tool permissions | `OAuthManager` |
| `apps/exo` | Exoskeleton operator UI and telemetry API | `exo-gui` |
| `apps/sprinter` | Discord bot and admin UI | `Sprinter` |
| `packages/style` | `@biotron/style`: brand, theme, shared controls | `BioTronStyle` |
| `go/logclient` | Go client every service uses to send events to Logger | `Logger/client` |
| `infra` | Edge Nginx, cloudflared, Postgres, Redis, deployment manifests | `Server` |

Each app keeps its own `README.md`, `.env.example`, `docker-compose.yml`, and
`prisma/` where it always was.

## The three files that make it one repo

- **`package.json`** declares the npm workspaces: `packages/*` and
  `apps/*/frontend`. One `package-lock.json`. Every frontend depends on
  `"@biotron/style": "*"`, which npm links to `packages/style`.
- **`go.work`** lists every backend and `go/logclient`. Each backend keeps its
  own `go.mod`, so an image compiles only its own code, but locally everything
  resolves together.
- **`docker-compose.yml`** includes `infra` and every app. `docker compose up -d`
  here starts the platform; `docker compose up -d --build web` inside an app
  folder still works alone.

Ports do not change: Auth 5173/8080, Exo 5174/8081, Logger 5175/8082,
Calendar 5176/8083, Site 5177, Sprinter 5178/8084.

## One baseline

Every app builds and runs on the same versions, and each version is declared
in exactly one place so it cannot drift:

| Tool | Version | Declared in |
|---|---|---|
| Node | 24 | every Dockerfile, the devcontainer, `.github/workflows` |
| TypeScript | 7.0 | root `package.json` |
| Linter | Oxlint, type-aware | root `package.json` and `.oxlintrc.json` |
| Vite, React plugin | 8.2, 6.1 | root `package.json` |
| React, React DOM | 19.2 | each app's `package.json`, same range |
| Tailwind | 4.3 | root `package.json`; an app opts in by importing it |
| Type packages | React 19.2, Node 24 | root `package.json` |
| Prisma | 6.19.3 | each `apps/*/prisma/package.json`, exact |
| Go | 1.27.0 | every `go.mod`, `go.work`, every backend Dockerfile |

An app's own `package.json` lists only what that app alone needs, such as
`three` for the site. Build and lint tooling never goes there.

## Working in it

```bash
npm ci                                   # every frontend and the library, once
npm run build                            # builds packages/style, then each app
npm run build -w apps/logger/frontend    # one app
npm run lint                             # Oxlint, type-aware, one configuration for all frontends
go work sync                             # after changing any go.mod
docker compose up -d                     # the whole platform (needs each app's .env)
```

Frontend images build from the repository root so they can see
`packages/style`; each app's compose file already sets `context: ../..`.

## The devcontainer

Open the repository in its devcontainer and everything is there: Node 24, Go
1.27, a Postgres, and a Redis. `.devcontainer/docker-compose.yml` starts the
two databases beside the workspace container, with the service names and
credentials every app's `.env.example` already uses, and the post-create
step installs every workspace, copies each app's `.env` from its example
where none exists, and applies every app's migrations. After that any app
runs natively against the sidecars, for example:

```bash
cd apps/logger/backend && set -a && . ../.env && set +a && PORT=8082 go run .
npm run dev -w apps/logger/frontend
```

Two things to know. The sidecar Postgres and Redis are their own definitions,
not a reference to `infra/docker-compose.yml`; they can drift from it, and a
commit brings them back when they do. And the workspace is bind-mounted from
the host, so every `node_modules` is masked by a container-side volume; the
container installs its own binaries and never touches the host's.

## How the repository was assembled

1. `git subtree add` for each repository, from its checked-out branch, so every
   commit is preserved and the originals were only read.
2. Root files added once: devcontainer, lint and TypeScript bases, CI
   workflows.
3. `Logger/client` moved to `go/logclient` with a workspace module path.
4. Frontends switched to the workspace-linked library; per-app lockfiles
   removed; one root install.
5. Frontend Dockerfiles rewritten for the root build context; compose files
   updated; the root compose validated with `docker compose config`.
6. `go work sync`, then every Go module built, vetted, and tested.

Verified at assembly: every frontend builds from the workspace, the root lint
runs, every Go module builds under `go.work`, and the root compose
configuration parses with the includes. Not yet done: no image has been built
and no container started from here, because the containers on the host still
run from the separate repositories on the same ports.

## What is still to do

- Go module paths still carry their old names
  (`github.com/BioTronDesignTeam/Logger/backend` and so on). Renaming them
  to `github.com/BioTronDesignTeam/biotron/apps/<app>/backend` is a
  find-and-replace plus `go work sync`.
- `apps/auth/backend/internal/eventlog` is a copy of the Logger client. It
  should import `go/logclient` instead and be deleted.
- The OAuthManager permission-check client that Logger, Exo, and Calendar each
  hand-roll belongs in `go/authcheck`. It is not extracted here.
- The CI workflows under `.github/workflows` are written but have never run.
- The React hook and effect findings Oxlint reports are warnings; each app
  should clear its own and the rules then return to errors.
