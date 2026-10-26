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
| `apps/logger` | Public status page and health monitor | `Logger` |
| `apps/auth` | GitHub OAuth, sessions, per-tool permissions | `OAuthManager` |
| `apps/exo` | Exoskeleton operator UI and telemetry API | `exo-gui` |
| `apps/sprinter` | Discord bot and admin UI | `Sprinter` |
| `packages/style` | `@biotron/style`: brand, theme, shared controls | `BioTronStyle` |
| `go/logclient` | Go client every service uses to send events to Grafana and PostHog | `Logger/client` |
| `go/authclient` | Go client and middleware the gated backends use to ask Auth about a session | new |
| `infra` | Edge Nginx, cloudflared, Postgres, the migrate image, deployment manifests | `Server` |

Each app keeps its own `README.md`, `.env.example`, `docker-compose.yml`, and
`prisma/` where it always was.

## The three files that make it one repo

- **`package.json`** declares the npm workspaces: `packages/*` and
  `apps/*/frontend`. One `package-lock.json`. Every frontend depends on
  `"@biotron/style": "*"`, which npm links to `packages/style`.
- **`go.work`** lists every backend and the shared modules under `go/`. Each
  backend keeps its own `go.mod`, so an image compiles only its own code, but locally everything
  resolves together.
- **`docker-compose.yml`** is one instance of the platform: it includes
  `infra/docker-compose.yml` (Postgres) and every app, so an
  instance's data and apps are one Compose project. `./launch --instance
  staging` runs a second copy beside it with its own network and volumes. The
  edge and Infisical are separate projects. `docker compose up -d --build
  logger-web` builds one service, and each app's compose file still works
  alone from its folder once the `biotron` network exists.

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
| Prisma | 6.19.3 | `infra/migrate/package.json`, exact |
| Go | 1.27.3 | every `go.mod`, `go.work`, every backend Dockerfile |
| golangci-lint, sqlc, actionlint, TruffleHog | 2.15.0, 1.31.1, 1.7.12, 3.97.4 | `.devcontainer/Dockerfile` `FROM` lines; CI reads them there |

An app's own `package.json` lists only what that app alone needs, such as
`three` for the site. Build and lint tooling never goes there.

## Working in it

```bash
npm ci                                   # every frontend and the library, once
npm run build                            # builds packages/style, then each app
npm run build -w apps/logger/frontend    # one app
npm run lint                             # Oxlint, type-aware, one configuration for all frontends
go work sync                             # after changing any go.mod
docker compose up -d                     # the whole platform (injected configuration)
```

### Starting and stopping

`./launch` wraps the data, app, edge, and Infisical compose files, so nobody
has to remember which one takes which flags. It only runs `docker compose`,
plus creating the external `biotron` network when it is missing. It never
edits an `.env` or removes a volume.

```bash
./launch infra                       # Postgres
./launch web --build                 # every app
./launch web --no-sprinter --build   # every app but one; --no-<app> repeats
./launch all --build                 # infra, then web
./launch edge --no-tunnel --build    # the edge Nginx alone; a host drops --no-tunnel
./launch stop                        # stop the apps and data services, keep the data
./launch status
```

Add `--dry-run` to any of them to print the docker commands and run nothing.
Use `--build` after a pull and after any `VITE_*` change, because those values
go into the built frontend. Migrations need no step of their own: each app's
`*-migrate` container runs the one migrate image (`infra/migrate`) on that
app's schema, applies what is pending, and exits before its API starts.

App `.env` files are optional local inputs. `./launch` selects the ones that
exist and checks required configuration only for selected apps. A deployed
host has no checkout and no `.env` files: `deploy/biotron-deploy` fetches an
instance's settings from Infisical and runs `./launch all --instance <name>
--runtime --pull`, which ignores repository `.env` files and pulls the
commit's images. App inputs use prefixes (`AUTH_DATABASE_URL`,
`CALENDAR_DATABASE_URL`, etc.) to prevent collisions in one injected process.
Container variable names stay unchanged. `jq` is required for launch
validation. See [Deploying staging and production](deploy/README.md).

`./launch secrets` starts the independent Infisical stack using bootstrap
credentials outside the checkout, and `./launch edge` the shared Nginx and
tunnel. `./launch stop secrets` and `./launch stop edge` stop them
explicitly; `all` and the default `stop` cover one instance's data services
and apps only.

Frontend images build from the repository root so they can see
`packages/style`, and the API images build from the root so they can see
the shared modules under `go/`; each app's compose file already sets
`context: ../..` for them.

### Before opening a pull request

Run `./pr-precheck` inside the devcontainer. It mirrors
`.github/workflows/ci.yml`, `sqlc.yml`, and `workflows.yml`: build and lint
affected frontends, then run golangci-lint, `go vet`, `go test`, and (where
the backend has a `sqlc.yaml`) `sqlc diff` for affected backends, and lint
every workflow with actionlint. Shared tooling changes expand the checks to
their consumers. Changes to the precheck, devcontainer, CI workflow, or
`.golangci.yml` check every app.

```bash
./pr-precheck --plan             # inspect the selected checks
./pr-precheck                    # check this branch and local edits
./pr-precheck --base main        # choose a different local comparison ref
./pr-precheck --all              # check every app and the shared Go client
```

The default comparison is the merge base with local `origin/main`, falling
back to `main`. The script includes staged, unstaged, and untracked files
that Git does not ignore. It never fetches; update your comparison ref
yourself when needed. Frontend checks start with `npm ci --ignore-scripts`
for all workspaces so the lockfile is checked and other apps retain their
dependencies. Independent checks continue after failures, and any failure
makes the command exit nonzero.

Security scans always run. TruffleHog checks new commits and separate
snapshots of staged and working changes, reporting verified findings and
verification errors without printing credentials. Trivy checks a snapshot
of the current Git-visible files for fixable HIGH/CRITICAL vulnerabilities
and misconfigurations, using `.trivyignore.yaml`. Ignored `.env` files,
dependencies, and build output stay out of the snapshots; tracked files
are included even if an ignore rule matches them. Symlink targets are
scanned as text without following them.

Rebuild an existing devcontainer to install the native scanner and linter
binaries. The checks need network access for dependencies, vulnerability
databases, and credential verification, but no Docker daemon. They do not
start services or run migrations. Database integration tests remain opt-in
through their existing test-database environment variables.

### Code quality checks in GitHub

| Workflow or file | What it does |
|---|---|
| `.github/workflows/ci.yml` | Builds, lints, and tests what a change touches; TruffleHog and Trivy on every change |
| `.github/workflows/sqlc.yml` | `sqlc diff` for every `apps/*/backend/sqlc.yaml`; passes when none exist |
| `.github/workflows/workflows.yml` | actionlint, with ShellCheck, on every workflow |
| `.github/workflows/codeql.yml` | CodeQL for Go, TypeScript, and the workflows, on pull requests, `main`, and each Monday |
| `.github/dependabot.yml` | Weekly grouped updates for Go, npm (root and Prisma), actions, Dockerfiles, and compose files |
| `.golangci.yml` | One golangci-lint config for every Go module; it lists each enabled linter and why |

Every action is pinned by commit SHA with its release in a comment, and
Dependabot moves both. Every job reads the repository only, and no checkout
keeps the token. Generate sqlc code with the devcontainer's `sqlc`: the
header of each generated file names the release, and `sqlc diff` fails on
any other one. Keep `database:` out of a `sqlc.yaml`, so sqlc reads the
schema from files and needs no Postgres in CI.

Repository settings the workflows depend on: CodeQL's default setup must stay
off (the workflow is the advanced setup), and Dependabot security updates
are a separate switch from this file.

### Environment settings

For local development, keep an optional `.env` beside each app's Compose file,
copied from its example. Production credentials live in Infisical, while
public deployment configuration stays in a sourced shell configuration file.
`VITE_*` URLs go into the public browser bundle; never put secrets there.
Backend integration URLs such as `OAUTH_MANAGER_URL` use Docker aliases
instead, so they remain separate even when they reach the same service.
Database URLs also remain app-specific because each app owns a different
Postgres schema; Sprinter's read-only connection has different permissions.

Calendar shares `VITE_API_URL` between browser requests and feed metadata.
`PUBLIC_BASE_URL` is optional: remove it when it matches `VITE_API_URL`, or
retain it for a separate subscriber address. CORS lists contain extra origins
only; the configured frontend (and Calendar's public site) is already included.

## The devcontainer

Open the repository in its devcontainer and everything is there: Node 24, Go
1.27, and a Postgres. `.devcontainer/docker-compose.yml` starts the database
beside the workspace container, with the service names and
credentials every app's `.env.example` already uses, and the post-create
step installs every workspace, copies each app's `.env` from its example
where none exists, and applies every app's migrations. After that any app
runs natively against the sidecar, for example:

```bash
cd apps/logger/backend && set -a && . ../.env && set +a && PORT=8082 go run .
npm run dev -w apps/logger/frontend
```

Two things to know. The sidecar Postgres is its own definition, not a
reference to `infra/docker-compose.yml`; it can drift from it, and a commit
brings it back when it does. And the workspace is bind-mounted from
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
configuration parses with the includes. Since then, every image has been built
from here and every container on the host runs from this repository, on the
same ports as before.

## What is still to do

- Go module paths still carry their old names
  (`github.com/BioTronDesignTeam/Logger/backend` and so on). Renaming them
  to `github.com/BioTronDesignTeam/BioTronWeb/apps/<app>/backend` is a
  find-and-replace plus `go work sync`.
- The React hook and effect findings Oxlint reports are warnings; each app
  should clear its own and the rules then return to errors.
