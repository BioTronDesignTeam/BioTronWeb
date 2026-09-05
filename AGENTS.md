# BioTron Tools Project Memory

This file records durable decisions from the August 2026 multi-repository local
integration session. Apply it to work in the repositories beneath this folder.
Never store actual OAuth credentials, ingestion tokens, guest keys, or database
passwords here.

## Repository scope and git policy

- The product group contains `exo-gui`, `Logger`, `OAuthManager`, and the shared
  React component library `BioTronStyle`.
- Keep experimental changes to `exo-gui`, `Logger`, and `OAuthManager` local;
  commits are wanted, but do not push those repositories unless the user later
  explicitly asks.
- `BioTronStyle` may be pushed when necessary to make it consumable by the
  product repositories.
- Preserve unrelated user changes and use separate, focused commits per repo.

## Shared visual language

- BioTron's palette is `#16033c`, `#160b6c`, `#aedbfc`, `#3050b0`, and
  `#ffffff`.
- Use `BioTronStyle` for shared brand assets and common surfaces: the login
  screen, theme toggle, account/user menu, logout affordance, and favicon.
- Favicons use the BioTron hand asset, never the default Vite lightning bolt.
- Dark mode follows the public site, not the brand purple: a graphite page
  (`#070b0e`), graphite surfaces (`#0e151b`, `#131c24`), white text with
  `#9fb0ba` secondary text, and purple `#160b6c` or blue `#3050b0` only as
  highlight fills. Accent text on graphite is the lifted blue `#7f9cf5`. Pale
  blue `#aedbfc` does not appear in dark mode. BioTronStyle 0.3.0 carries these
  as `--biotron-*` tokens; the four tools pin it and use those tokens or the
  same literals. Light mode is unchanged.
- Login screens should be concise and consistent. Keep the BioTron logo and
  product name; omit generic “BioTron Tools” copy and redundant explanatory
  lines. Exo keeps its clean daily-guest-key login option.

## Theme behavior

- Theme preference uses the shared `biotron-theme` cookie. It works across
  localhost ports and, on deployment, across sibling `*.biotron.ca`
  subdomains. Legacy per-origin `darkMode` local-storage state is migrated.
- Every frontend must retain the complete synchronous theme bootstrap directly
  in `index.html`, before React/TypeScript loads. It applies the `dark` class,
  `color-scheme`, and page background immediately to prevent a white flash.
- Do not move the bootstrap exclusively into React or the shared package.

## Authentication and permissions

- OAuthManager is the central GitHub OAuth/session and per-product permission
  service. Sessions must support the shared deployment cookie domain.
- Host ports are standardised across the workspace. Frontends occupy a
  contiguous `5173`-`5178` block and APIs a contiguous `8080`-`8084` block:

  | Service | Frontend host port | API host port |
  | --- | --- | --- |
  | OAuthManager | `5173` | `8080` |
  | Exo (`exo-gui`) | `5174` | `8081` |
  | Logger | `5175` | `8082` |
  | BiotronCalendar | `5176` | `8083` |
  | Biotron-Site | `5177` | n/a |
  | Sprinter | `5178` | `8084` |

- OAuthManager's API port `8080` is fixed because the GitHub OAuth callback is
  registered against it. Every other host port may be moved only by updating
  this table and all six repositories together.
- Only host-published ports follow this table. Every container still listens on
  `8080` internally, and service-to-service URLs use Docker network aliases
  (`http://oauth-manager:8080`, `http://logger-api:8080`,
  `http://calendar-api:8080`), which never change with the host map.
- The `18080` compatibility proxy is retired. No repository references `18080`,
  `18081`, `18082`, `18083`, or `18084` any more; do not reintroduce them.
- CORS allow-lists must name both the `localhost` and the `127.0.0.1` form of
  every permitted port, because browsers treat them as different origins.
  OAuthManager's `CORS_ORIGINS` covers `5174`-`5178` in both forms.
- Exo's app id is `exo-gui` with permissions `live`, `historical`, and
  `commands`. Migrated legacy `view` access means Live + Historical; legacy
  `write` means Commands.
- Logger's app id is `logger` and its single permission is `view`.
- Only Exo currently has a daily product guest key. Managers and superusers can
  reveal/copy it in OAuthManager's Keys tab. Exo daily guests may use Live and
  Historical, never Commands. Guest access does not grant Logger View.

## Structured logging

- Logger owns the structured ingestion API and the shared service catalog.
  Catalog ids in current scope are `oauth-manager`, `exo-api`, and
  `logger-api`.
- OAuthManager and Exo emit lifecycle events and completed HTTP request events.
  Request payloads contain only method, path, status, and duration; never log
  query strings, cookies, authorization headers, credentials, or tokens.
- Health probes are excluded to prevent log spam. HTTP 4xx events are warnings,
  5xx events are errors, and successful events are info.
- Logger records its own lifecycle directly in its store rather than POSTing to
  itself, which avoids recursive self-logging.
- `LOGGER_INGEST_TOKEN` is a shared secret and must match Logger in each product
  environment. `LOGGER_URL` defaults to `http://logger-api:8080`; `LOG_LEVEL`
  defaults to `info`. Missing ingestion configuration must not prevent a
  product from starting.

## Local integration expectations

- The repositories share the external Docker network named `biotron`, plus the
  shared Postgres and Redis containers.
- After cross-repository changes, run each affected test suite, rebuild only the
  affected containers, verify real browser/API behavior, and query Logger's
  local store when confirming end-to-end log delivery.

## Toolchain versions and the deliberate exceptions

Every repository runs the current stable toolchain: Go 1.27, Fiber v3, React
19.2.8, Vite 8.2.2, and TypeScript 7.0.2. Three deviations are deliberate, and
each one will look like an oversight to anyone tidying up later. Leave them
alone unless the reason has actually gone away.

**exo-gui stays on TypeScript 6.** It is the only repository with ESLint
configured, and `typescript-eslint` has no TypeScript 7 release: its peer range
stops below 7, and forcing past that does not merely warn, it throws
`typescript-eslint does not support TS 7.0` at lint time. TypeScript 7 itself
type-checks exo-gui with zero errors, so the compiler is not the blocker — the
linter is. A working `npm run lint` is worth more than matching version numbers,
so exo-gui stays on the newest TypeScript the linter parses. Revisit when
typescript-eslint ships TypeScript 7 support.

**`@types/node` stays on the 24.x line, not 26.** The devcontainers run Node 22
and the host runs Node 24. Installing the 26.x types would declare APIs that do
not exist at runtime, which is a silent type-safety regression wearing an
upgrade's clothes.

**Prisma stays on 6.19.3.** npm's `latest` tag currently points at an 8.x release
candidate, which is not stable. Prisma 7 is a real breaking change: it removes
the `url` property from the schema's datasource block, so adopting it requires a
`prisma.config.ts` and a driver adapter passed to `PrismaClient` in all five
repositories that own a schema. That is a coordinated migration, not an
incidental bump.

Two smaller notes worth keeping. Fiber v3 renamed the proxy-trust configuration
from `EnableTrustedProxyCheck` plus `TrustedProxies` to `TrustProxy` plus
`TrustProxyConfig{Proxies: ...}`; the semantics are identical and the old names
no longer compile, so the rename cannot silently disable per-IP rate limiting.
Prefer the explicit CIDR list over `TrustProxyConfig.Private`, which also trusts
`10/8` and `192.168/16` and is therefore wider than what `TRUSTED_PROXIES`
states. And `lucide-react` 1.x removed every brand icon, so the site's Instagram
mark is drawn locally in `Biotron-Site/frontend/src/components/InstagramIcon.tsx`
rather than imported; do not "restore" it to a lucide import, because there is
nothing to import.

## Edge security headers: CSP and HSTS

The edge Nginx in `Server/` sets a document-level Content-Security-Policy that
is enforced (`frame-ancestors 'none'`, `base-uri 'none'`, `object-src 'none'`,
`form-action 'self'`) plus a separate resource-loading CSP that ships as
`Content-Security-Policy-Report-Only`. `frame-ancestors` has to be in the
enforced policy because Report-Only does not honour that directive.

Leave the resource-loading CSP in Report-Only. It is deliberately low priority:
the only inline script in any frontend is the synchronous dark-mode bootstrap
that this file already requires to stay in `index.html`, so the realistic
exposure a stricter `script-src` would close is small. Enforcing it would mean
maintaining per-app script hashes at a shared edge that serves five apps, and
every `index.html` edit would invalidate them. Do not promote it to enforcing
without asking.

HSTS is emitted only when `$http_x_forwarded_proto` is `https`, because the edge
listens on plain HTTP behind cloudflared and its own `$scheme` therefore always
reads `http`. This has been verified locally against a stub but never against
the real Cloudflare edge, so it is unconfirmed whether the header reaches the
origin in production. If it does not arrive, HSTS silently never ships: there is
no error and no log line, only absent protection.

**Do not tune, enable, or extend HSTS as incidental work.** It is deferred by
choice until someone deliberately picks it up. HSTS is hard to reverse, since
browsers cache it for the full `max-age` and a certificate problem then locks
visitors out with no way to reach them. When it is picked up: deploy, confirm
with `curl -sI https://<domain> | grep -i strict-transport-security`, start with
a short `max-age` such as 300 seconds, and only raise it once the deployment is
proven healthy. Do not add `preload`, which is effectively permanent.

## Biotron public-site refactor

- Before changing `Biotron-Site`, read
  `.cursor/rules/biotron-site-refactor-memory.mdc`. It records the current
  information architecture, visual language, homepage/Three.js composition,
  copy voice, contact channels, preview setup, and verification workflow.
- Continue on `la/general-improvements`, keep the site preview at
  `http://localhost:5177`, make focused local commits, and never push this
  repository unless the user explicitly replaces the existing no-push rule.

## Calendar architecture

- Use the hybrid calendar architecture: the BioTron public site presents a
  lightweight calendar view with clear actions to open the full calendar and
  subscribe, while the dedicated calendar application owns the complete
  browsing and subscription experience.
- Maintain one calendar system and one source of truth. Both presentations must
  consume the same event API/data source; do not duplicate event data or
  business logic between repositories.
- Keep calendar subscription URLs stable and independent of either frontend.
- Keep the public-site view intentionally small or reuse shared calendar UI to
  avoid maintaining two divergent calendar implementations. Avoid an iframe
  unless its delivery advantage clearly outweighs the styling, accessibility,
  navigation, and responsive-design costs.
- Calendar subscriptions are independent exact feeds: one teamwide feed, one
  per project, one per subteam, plus an explicit All Events feed. Parent events
  are never inherited into child feeds, so subscribing to several exact feeds
  does not create duplicates.
- Model one scope hierarchy with a single team root, projects beneath the team,
  and subteams beneath projects. Projects and subteams can be added, renamed,
  archived, restored, and hard-deleted only when they are empty leaf scopes
  with no event history. Archiving preserves records and stable feed contents;
  an archived ancestor hides its descendants from active public JSON views.
- Weekly recurring series always have an inclusive end date. Do not add a
  pause/resume abstraction; create a new series when the next term's schedule
  is known. Support editing/cancelling one occurrence by its original scheduled
  local start, and retain cancellation records for subscriber reconciliation.
- Store and expand recurrence as `America/Toronto` wall-clock time so meetings
  remain at the same local hour through daylight-saving changes.
- PostgreSQL is the Calendar source of truth. Do not introduce Redis caching
  until measured load justifies the invalidation and operational complexity.
- Keep Calendar UI, API, OAuth, and BioTron site URLs environment-driven until
  production domains are chosen. Public reads stay unauthenticated; all editor
  mutations require server-verified OAuthManager `calendar/write` permission.
