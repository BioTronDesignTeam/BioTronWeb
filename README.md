# BioTron platform

One repository for the BioTron web platform: six products, the shared
component library, the shared Go client, and the edge that serves them. Every
piece arrived with its full history; the separate repositories it came from
are untouched and remain the source of truth until the team decides to switch.

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
| `docs` | Brand assets, plans, decision records | the workspace folder |

Each app keeps its own `README.md`, `.env.example`, `docker-compose.yml`, and
`prisma/` where it always was. `AGENTS.md` and `.cursor/rules` sit at the root,
under version control for the first time.

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

## Working in it

```bash
npm ci                                   # every frontend and the library, once
npm run build                            # builds packages/style, then each app
npm run build -w apps/logger/frontend    # one app
npm run lint                             # one configuration for all frontends
go work sync                             # after changing any go.mod
docker compose up -d                     # the whole platform (needs each app's .env)
```

Frontend images build from the repository root so they can see
`packages/style`; each app's compose file already sets `context: ../..`.

## How this preview was built

1. `git subtree add` for each repository, from its checked-out branch, so every
   commit is preserved and the originals were only read.
2. Root files added once: rules, devcontainer, lint and TypeScript bases, CI
   workflows, brand assets.
3. `Logger/client` moved to `go/logclient` with a workspace module path.
4. Frontends switched to the workspace-linked library; per-app lockfiles
   removed; one root install.
5. Frontend Dockerfiles rewritten for the root build context; compose files
   updated; the root compose validated with `docker compose config`.
6. `go work sync`, then every Go module built, vetted, and tested.

Verified in this preview: every frontend builds from the workspace, the root
lint runs, every Go module builds under `go.work`, and the root compose
configuration parses with the includes. Not verified: no image was built and no
container was started from here, because the separate repositories are still
running on the same ports.

## What is deliberately left for the real migration

- Go module paths still carry their old names
  (`github.com/BioTronDesignTeam/Logger/backend` and so on). Renaming them
  to `github.com/BioTronDesignTeam/biotron/apps/<app>/backend` is a
  find-and-replace plus `go work sync`, done when the repo becomes real.
- `apps/auth/backend/internal/eventlog` is a copy of the Logger client. It
  should import `go/logclient` instead and be deleted.
- The OAuthManager permission-check client that Logger, Exo, and Calendar each
  hand-roll belongs in `go/authcheck`. It is not extracted here.
- The CI workflows under `.github/workflows` are written but have never run.
- The twelve React hook lint findings are warnings; each app should clear its
  own and the rules then return to errors.
