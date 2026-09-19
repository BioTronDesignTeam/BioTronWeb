# BioTron platform: rules for agents

Read `README.md` first. It owns the layout, the toolchain versions, the run
commands, and the devcontainer. This file holds the decisions that the code
alone does not explain. `CLAUDE.md` is a symlink to this file.

Never write a real OAuth credential, ingestion token, guest key, or database
password into this file or any tracked file. Secrets live in each app's
untracked `.env`.

## Where things are

| Path | What | Old repository name |
|---|---|---|
| `apps/site` | Public team website | `Biotron-Site` |
| `apps/calendar` | Public calendars and feeds | `BiotronCalendar` |
| `apps/logger` | Status page and log warehouse | `Logger` |
| `apps/auth` | GitHub OAuth, sessions, permissions | `OAuthManager` |
| `apps/exo` | Exoskeleton operator UI and API | `exo-gui` |
| `apps/sprinter` | Discord bot and admin UI | `Sprinter` |
| `packages/style` | `@biotron/style` shared components | `BioTronStyle` |
| `go/logclient` | Go client that sends events to Logger | `Logger/client` |
| `infra` | Edge Nginx, cloudflared, Postgres, Redis | `Server` |

The old repositories are frozen. Code comments and app ids still use some old
names, such as `OAuthManager` and `exo-gui`. They mean the paths above.

## How to work here

- Stay inside the app the task names. Note other problems; do not fix them in
  the same change.
- Match the code around you. Build and lint tooling belongs in the root
  `package.json`, never in an app's own `package.json`.
- Commit each finished step with one clear reason. If change A needs change B,
  commit B first. Keep behavior and the tests that prove it in one commit.
- Leave every commit buildable. Do not amend, squash, or reorder existing
  commits unless asked.
- Each app has one untracked `.env` and one committed `.env.example`, both
  beside its Compose file. `VITE_*` values go into the public browser bundle;
  never put a secret there.
- Prisma owns every Postgres schema and migration. Go code queries with `pgx`.
  Do not use a Prisma client from Go.
- Run `./pr-precheck` inside the devcontainer before you open a pull request.

## Run and verify

- Use `./launch`. It wraps both compose files; `./launch --help` lists it all.
- Start `infra` first. It owns Postgres, Redis, and the `biotron` Docker
  network that every app joins. For local work: `./launch infra --local`.
- Do not start `cloudflared` for local work. It opens a public tunnel.
  `--local` leaves it out; a plain `./launch infra` starts it.
- Start the apps with `./launch web --build`, and leave one out with
  `--no-<app>`. For a single app, name its services to compose directly, for
  example `docker compose up -d --build calendar-migrate calendar-api calendar-web`.
- After a change: run the affected tests, rebuild only the affected
  containers, and check the real page in a browser. For a Three.js change on
  the site, check the exact scroll stop you changed.

## Ports

| Service | Frontend | API |
|---|---|---|
| Auth | `5173` | `8080` |
| Exo | `5174` | `8081` |
| Logger | `5175` | `8082` |
| Calendar | `5176` | `8083` |
| Site | `5177` | n/a |
| Sprinter | `5178` | `8084` |

- Auth's `8080` is fixed. The GitHub OAuth callback is registered against it.
- Move any other host port only by updating this table, the README, and every
  app that names the port, in one change.
- Only host ports follow this table. Every container listens on `8080`
  inside Docker. Services reach each other by network alias, such as
  `http://oauth-manager:8080`, `http://logger-api:8080`, and
  `http://calendar-api:8080`.
- The `18080`-`18084` proxy ports are retired. Do not bring them back.
- CORS allow-lists must name both the `localhost` and the `127.0.0.1` form of
  each port. Browsers treat them as different origins.

## Authentication

- Auth is the only service that creates a session. It sets one `HttpOnly`
  session cookie.
- The other backends keep no session. Each one forwards the request's `Cookie`
  header to Auth, and Auth answers whether that session holds the permission.
- On a deployment, set `COOKIE_DOMAIN` to the parent domain, such as
  `.biotron.ca`, so the browser sends the cookie to every app hostname. Leave
  it blank on localhost. Auth refuses to start with a cookie domain and
  `COOKIE_SECURE=false`.
- Known trade-off, accepted for now: one cookie reaches every subdomain, so a
  flaw in one app can expose the session for all of them. A move to per-app
  sessions is its own project across four backends. Do not start it as side
  work.
- A backend that cannot reach Auth answers 503. It fails closed, never open.
- App ids and permissions: `exo-gui` has `live`, `historical`, and `commands`.
  `logger` has `view`. `calendar` has `write`. `sprinter` has an admin check.
- New tools register as apps in Auth. Do not build a separate login.
- Only Exo has a daily guest key. Guests may use Live and Historical, never
  Commands. Guest access does not grant Logger View.
- Day boundaries and guest-key rotation use `America/Toronto`.

## Theme and brand

- The palette is `#16033c`, `#160b6c`, `#aedbfc`, `#3050b0`, and `#ffffff`.
- Use `@biotron/style` for the login screen, theme toggle, user menu, logout
  control, wordmark, and favicon. The favicon is the BioTron hand, never the
  Vite default.
- Dark mode is graphite, not brand purple: page `#070b0e`, surfaces `#0e151b`
  and `#131c24`, white text, `#9fb0ba` secondary text. Purple `#160b6c` and
  blue `#3050b0` are highlight fills only. Accent text on graphite is
  `#7f9cf5`. Pale blue `#aedbfc` does not appear in dark mode. Use the
  `--biotron-*` tokens.
- The theme preference is the `biotron-theme` cookie. It is set on the parent
  domain so every tool shows the same theme. It holds no secret.
- The five tool frontends keep the full theme bootstrap script inline in
  `index.html`, before React loads. It stops a white flash on page load. Do
  not move it into React or into `@biotron/style`.
- Login screens stay short: the logo and the product name. Exo keeps its
  daily-guest-key option.

## Logging

- Logger owns the ingestion API. Every Go service sends events through
  `go/logclient`.
- Request events hold only method, path, status, and duration. Never log a
  query string, cookie, authorization header, credential, or token.
- Skip health probes. A 4xx is a warning, a 5xx is an error, and the rest is
  info.
- Logger writes its own lifecycle events straight to its store. It does not
  POST to itself.
- `LOGGER_INGEST_TOKEN` must match Logger's value. `LOGGER_URL` defaults to
  `http://logger-api:8080` and `LOG_LEVEL` to `info`. A missing or unreachable
  Logger must never stop a service from starting.

## Versions that look wrong but are deliberate

- **Prisma stays on `6.19.3`, exact.** Prisma 7 removes `url` from the schema
  datasource block. Adopting it needs a `prisma.config.ts` and a driver adapter
  in all five schema-owning apps. Treat it as a planned migration, not a bump.
- **`@types/node` stays on the 24.x line.** It matches the Node 24 runtime.
  Newer types would declare APIs that do not exist at runtime.
- **The Instagram icon is drawn locally** in
  `apps/site/frontend/src/components/InstagramIcon.tsx`. `lucide-react` 1.x
  removed every brand icon, so there is nothing to import.
- **Fiber v3 proxy trust** uses `TrustProxy` plus
  `TrustProxyConfig{Proxies: ...}` with an explicit list. Do not switch to
  `TrustProxyConfig.Private`. It also trusts `10/8` and `192.168/16`, which is
  wider than `TRUSTED_PROXIES` states.

## Edge security headers

- `infra/nginx` enforces a document-level CSP: `frame-ancestors 'none'`,
  `base-uri 'none'`, `object-src 'none'`, `form-action 'self'`.
- The resource-loading CSP ships as Report-Only on purpose. Enforcing it means
  keeping per-app script hashes for the inline theme bootstrap. Ask before you
  enforce it.
- HSTS is sent only when `X-Forwarded-Proto` is `https`, because the edge
  listens on plain HTTP behind cloudflared. This was tested against a local
  stub, never against the real Cloudflare edge.
- Do not tune, enable, or extend HSTS as side work. Browsers cache it for the
  full `max-age`, so a mistake locks visitors out. When someone picks it up:
  start with `max-age=300`, confirm with `curl -sI`, and never add `preload`.

## Deployment

- Path: Cloudflare edge, then Cloudflare Tunnel, then the `cloudflared`
  container, then Nginx, then the app containers. One Postgres and one Redis
  per environment.
- Cloudflare Tunnel is the only public ingress. Tailscale SSH is the only
  admin path. Never publish SSH, Nginx, app, Postgres, or Redis ports.
- Two Docker networks: `biotron-edge` holds only `cloudflared` and Nginx.
  `biotron` holds Nginx, the apps, Postgres, and Redis. Nginx is the only
  bridge.
- One app per hostname: `www`, `calendar`, `auth`, `exogui`, `status`
  (Logger), and `sprinter`. `infra/nginx` is the source of truth. Within a
  hostname, `/` goes to the frontend and `/api/` to the backend.
- Staging uses `*.biotron-dev.com`. Production uses the same layout under its
  own domain. Domains, secrets, and data differ; routing and containers do not.
- Build `linux/amd64` and `linux/arm64` images once in CI and pin them by
  digest. Test that digest in staging, then promote the same digest. Never
  rebuild a candidate after staging approves it.

## Calendar

- One calendar system and one source of truth. The site's calendar view and
  the calendar app read the same API. Do not copy event data or business logic
  into the site.
- The site shows a small view with links to open the full calendar and to
  subscribe. The calendar app owns full browsing and subscription. Avoid an
  iframe.
- Subscription URLs stay stable and do not depend on either frontend.
- Feeds are exact: one for the team, one per project, one per subteam, and an
  explicit All Events feed. A child feed never inherits parent events, so
  several subscriptions never duplicate an event.
- One scope tree: the team root, projects under it, subteams under projects.
  A scope can be hard-deleted only when it is an empty leaf with no event
  history. Archiving keeps records and feed contents. An archived parent hides
  its children from the active public JSON views.
- A weekly series always has an inclusive end date. There is no pause and
  resume; create a new series for the next term. Edit or cancel one occurrence
  by its original local start time, and keep cancellation records so
  subscribers reconcile.
- Store and expand recurrence as `America/Toronto` wall-clock time, so a
  meeting keeps its local hour across daylight-saving changes.
- PostgreSQL is the source of truth. Do not add a Redis cache until measured
  load justifies it.
- Keep every URL environment-driven. Public reads need no login. Every editor
  mutation needs a server-verified `calendar/write` permission from Auth.

## Public site

- Every page and its title and description live in
  `apps/site/frontend/src/data/seo.ts`. The build writes an HTML file, the
  sitemap, `robots.txt`, and `llms.txt` from that list, and Nginx answers 404
  for any path with no file. A new route that is missing from the list returns
  404 in production. See `docs/site-seo.md`.
- A deployment must set `VITE_SITE_URL` to the public address before the build.
- Top navigation: Projects, Sponsors, Calendar, and the join call to action.
  `/projects` lists current projects first and past projects below.
  `/past-projects` redirects to `/projects#past`.
- Use black and graphite surfaces. Purple `#160b6c` is the main highlight and
  blue `#3050b0` the second. Do not use pale blue `#aedbfc` in the site UI.
- The hero reads exactly two lines: `Welcome to`, then `Biotron.`
- The hero's resting offset belongs on `.flythrough__hero-content`. GSAP
  animates `.flythrough__hero`. Putting both transforms on one element makes
  the page jump on the first pixel of scroll.
- The workshop has no random scatter of cubes, cylinders, or toruses. Do not
  restore it. Use the native browser cursor.
- The desktop layout is the approved reference. Scope mobile fixes to phone
  and touch breakpoints. Phones below 640px, and short touch viewports, get the
  stacked homepage instead of the flythrough.
- Mobile keeps a 20px minimum gutter, respects safe-area insets, and keeps
  44px touch targets. The mobile menu locks page scroll while open and closes
  on Escape and on every choice.
- Site copy is active, concrete, and short. Do not use em dashes in site copy.
- Facebook is retired and must not appear. Contact is `biotron@uwaterloo.ca`
  and Instagram `@uwaterloo_biotron`.
