# Infra

Deployment and host operations for the BioTron platform. Each host runs one
instance of the platform, `staging` or `prod`, behind its own edge, with its
own Infisical:

```text
Cloudflare -> cloudflared -> Nginx -> the instance's apps -> the instance's Postgres
```

Tailscale SSH is the separate administrative path. None of the edge or
application services require a publicly reachable host port. Which host runs
what, and how a host is set up, deployed, migrated, and rotated, is in
`deploy/README.md`.

## Layout

| Path | What |
|------|------|
| `docker-compose.yml` | Postgres of one instance, and `postgres-roles`, which brings its roles up to date; the root `docker-compose.yml` includes it |
| `postgres/init/` | The roles: one per app (`05-app-roles.sh`) and Sprinter's read-only one (`10-sprinter-reader.sh`) |
| `edge/docker-compose.yml` | Nginx and cloudflared, the host's `biotron-edge` project |
| `infisical/` | The `biotron-secrets` project: the host's Infisical with its own Postgres and Redis |
| `no-host-ports.yml` | Drops every host port from a named instance |
| `.env.example`, `edge/.env.example` | Local data-service settings; the edge's sites and tunnel token |
| `migrate/` | The one migrate image: Prisma applies each app's `apps/<app>/prisma/migrations` to its own schema, then `migrate/grants/<app>.sql` |
| `nginx/` | The versioned edge image, per-instance hostname routing, security headers, proxy headers, and health endpoint |
| `tests/` | Compose, launch, deploy, edge, and database regression checks; see "Tests" below |

## Instances and networks

An instance is one Compose project holding its Postgres and six apps:
`biotron` on a laptop, `biotron-<instance>` from
`./launch --instance <instance>`. Each has its own network,
`biotron-<instance>` (`biotron` on a laptop), its own volumes, and, when named,
no host ports.

- Inside an instance's network the services keep their plain aliases:
  `postgres`, `oauth-manager`, `status-api`, `calendar-api`, `biotron-web`,
  and so on. Apps reach each other and their database by
  these names, and cannot see any other instance.
- Every web and API service also carries an instance-qualified alias, such as
  `staging-oauth-manager`, for the edge.
- The edge project has a private network, `EDGE_SUBNET` (default
  `10.213.255.0/29`), holding only cloudflared and Nginx. `./launch edge` also
  attaches Nginx to the network of every instance named in `EDGE_SITES`,
  where it answers to `edge-proxy`. Nginx is the only container on more than
  one network.

`./launch` creates the external networks when they are missing, so the edge
and the instances can start in any order. Each gets a fixed subnet,
`10.213.<n>.0/24`: `n` is 0 on a laptop, 1 for `staging`, 2 for `prod`, and
derived from the name for any other instance. Nginx always holds `.2` on it;
Docker hands the other containers addresses from the upper half. A network
made before the subnets were fixed is refused with the command that replaces
it: stop its containers and the edge, `docker network rm` it, and launch
again.

## Hostname routing

`EDGE_SITES` in `/etc/biotron/edge.env` lists the instances this edge serves,
as `<instance>=<domain>[:noindex]` pairs. On the staging host it is
`staging=uwbiotron.dev:noindex`, on the production host `prod=uwbiotron.ca`;
one edge can list several instances.

When Nginx starts, `nginx/docker-entrypoint.d/40-biotron-sites.sh` renders
`nginx/sites/site.conf.template` once per instance. For `prod=uwbiotron.ca`:

| Hostname | Frontend | `/api/` backend |
|----------|----------|------------------|
| `uwbiotron.ca` | `prod-biotron-web:80` | none |
| `calendar.uwbiotron.ca` | `prod-calendar-web:80` | `prod-calendar-api:8080` |
| `auth.uwbiotron.ca` | `prod-oauth-web:80` | `prod-oauth-manager:8080` |
| `exogui.uwbiotron.ca` | `prod-exogui-web:80` | `prod-exogui-api:8080` |
| `sprinter.uwbiotron.ca` | `prod-sprinter-web:80` | `prod-sprinter:8080` |

`www` redirects to the apex. Unknown hostnames return `404`. `status.` is not
served here: Cloudflare redirects it to the public status page, outside this
repository. Upstreams are resolved per request, so an instance that is down
answers `502` and the others are unaffected. `/api/` is removed before forwarding so an external
request for `/api/health` reaches the Go service as `/health`. Redirects stay
relative (`absolute_redirect off`), so `/api` answers `Location: /api/`.

`:noindex` marks an instance that search engines should leave alone, such as
staging: every response from its hostnames, errors included, carries
`X-Robots-Tag: noindex, nofollow`, and its `robots.txt` disallows everything.
An unmarked instance sends no such header and serves the site's own
`robots.txt`. This hides nothing from people; see "Search engines and access"
in `deploy/README.md`.

Frontend production builds use relative API bases such as `/api`. Auth's
GitHub App callback is therefore
`https://auth.<domain>/api/auth/github/callback`, one GitHub App per
instance; see `apps/auth/README.md`.

## Security headers

Set once in `nginx/nginx.conf`, for every hostname:

- A Content-Security-Policy, enforced, in two headers: a document-level one
  (`frame-ancestors`, `base-uri`, `object-src`, `form-action`) and the
  per-instance resource-loading one in `nginx/sites/csp.template`, whose
  `script-src` allows the theme bootstrap of each `index.html` by its hash.
  Browsers report violations to `/_edge/csp-report`, which Nginx logs as
  `"type":"csp-report"` lines without query strings
  (`nginx/snippets/csp-report.conf`).
- `Strict-Transport-Security: max-age=300`, only when cloudflared says the
  request arrived over HTTPS, with no `includeSubDomains` or preload.
- `X-Content-Type-Options`, `Referrer-Policy`, `X-Frame-Options`, and
  `Permissions-Policy`.

## Client addresses

cloudflared sends the visitor's address in `Cf-Connecting-Ip`. Nginx believes
it only on a connection from the edge network (`set_real_ip_from
EDGE_SUBNET`), so a container on an instance network that calls `edge-proxy`
cannot choose its own. Nginx then sets `Cf-Connecting-Ip` itself on every
request it forwards (`nginx/snippets/proxy-headers.conf`). Auth and Status
read it only from a peer in `TRUSTED_PROXIES`, which defaults to loopback and
the edge's address on the instance's network, `10.213.<n>.2`; `./launch`
passes that address to Compose as `BIOTRON_EDGE_ADDRESS`. Any other caller,
such as another container or the host, is keyed by its own address.

## Cloudflare Tunnel

One remotely managed tunnel per host, not per instance: the edge is the
host's. Add every hostname of every instance in `EDGE_SITES`, plus each
`www`, to it. Every public hostname uses the same origin service:

```text
http://nginx:8080
```

Nginx selects the instance and application from the original HTTP `Host`
header. The tunnel token lives in `/etc/biotron/edge.env` as
`CLOUDFLARE_TUNNEL_TOKEN`; Compose hands it to cloudflared as the read-only
secret file `/run/secrets/cloudflare_tunnel_token`. Rotating it requires
recreating cloudflared. Never run two edges with one tunnel's token: Cloudflare
spreads requests across every connector of a tunnel.

## Local use

```bash
./launch infra                       # Postgres and its roles, reading infra/.env
./launch web --build                 # the apps
cp infra/edge/.env.example infra/edge/.env
./launch edge --no-tunnel --build    # Nginx alone, serving EDGE_SITES=local=localhost
docker compose -p biotron-edge exec nginx wget -qO- --header 'Host: calendar.localhost' http://127.0.0.1:8080/
```

Never start the tunnel on a laptop.

## Verification

```bash
sudo docker compose -p biotron-edge ps
sudo docker compose -p biotron-edge exec nginx nginx -t
sudo docker compose -p biotron-edge exec nginx wget -qSO- --header 'Host: uwbiotron.dev' http://127.0.0.1:8080/
sudo docker compose -p biotron-edge logs --tail 100 nginx cloudflared
sudo docker compose -p biotron-prod ps
```

The Nginx health response at `/_edge/health` is `ok`. Cloudflared exposes its
readiness and metrics endpoint to the private edge network at
`http://cloudflared:2000` for future Status integration; it is not published
on the host.

Test each public hostname through Cloudflare after its instance is running.
A `502` means Nginx is reachable but that instance's application alias is not.

## Security boundary

- Permit Cloudflare Tunnel's required outbound traffic, including TCP/UDP
  `7844`; no inbound HTTP/HTTPS rule is needed.
- Do not open public SSH. Install Tailscale on the host and authorize Tailscale
  SSH using tailnet grants and SSH rules. The CI account's login shell runs a
  deploy and nothing else (`deploy/biotron-deploy-shell`).
- Nginx and cloudflared use read-only root filesystems and
  `no-new-privileges`. The tunnel token is a read-only Compose secret.
- Nginx accepts only the configured instances' hostnames, and takes the
  client's address from cloudflared alone (see "Client addresses"). It does
  not expose Docker's socket.
- Each instance has its own Postgres on its own network. A named
  instance publishes no host ports; administer it with
  `docker compose -p biotron-<instance> exec`.
- Each app connects to Postgres as its own role, with data rights in its own
  schema only; no app is a superuser. See "Database roles".
- Nginx is on every instance's network, so it could reach each instance's
  Postgres; nothing in its configuration does.

## Startup and updates

All long-running containers use `restart: unless-stopped`, so Docker restores
them after a cold boot with the settings they were deployed with. Enable
Docker and Tailscale on the host; cloudflared is not installed as a native
systemd service.

Every image here is pinned by version and digest, and `scripts/check-images`
fails on one that is not; Dependabot moves the tag and the digest together.
The edge and the six frontends run the same stable Nginx, and each image's
final stage upgrades its OS packages, so the Trivy gate of
`.github/workflows/deploy.yml` passes between base image updates. The
migrate image keeps Node but drops npm, npx, and Corepack from its runtime
stage: Prisma needs none of them. Infisical moves only by hand (see
`infisical/README.md`), so Dependabot skips it.

The local edge image is `biotron-edge-nginx:local`. See `deploy/README.md`
for the deployment workflow and GHCR image tags.

## Database roles

The cluster owner, `POSTGRES_USER` (`biotron`), owns every schema and table
and runs the migrations. Each app connects as a role of its own, which can
read and write the rows of its own schema and nothing else:

| Role | Schema | Password |
|------|--------|----------|
| `auth_app` | `oauth` | `AUTH_DB_PASSWORD` |
| `calendar_app` | `calendar` | `CALENDAR_DB_PASSWORD` |
| `status_app` | `logger` | `STATUS_DB_PASSWORD` |
| `sprinter_app` | `sprinter` | `SPRINTER_DB_PASSWORD` |

`postgres/init/05-app-roles.sh` makes them. The postgres image runs it once on
a fresh cluster; the `postgres-roles` one-shot service runs it again, with
`10-sprinter-reader.sh`, every time the instance starts, before any migration.
So an existing cluster gets new roles, and a password changed in Infisical (or
`infra/.env`) reaches the database on the next deploy (or `./launch infra`).
Default privileges give each app every table its migrations add.

On a laptop, a role whose password `infra/.env` leaves empty is not made, and
that app's `.env` must use the owner's URL instead. A deployment refuses to
start without all four.

## Read-only role for Sprinter

Sprinter's model tools read `logger.health_checks` and four `oauth` tables,
which Status and Auth own, through a dedicated role, `sprinter_reader`. What
keeps it read-only is its grants: SELECT on those tables and nothing else, no
other schema, and no temporary tables.
`default_transaction_read_only` is set too, as a second guard that the role
could turn off.

`postgres/init/10-sprinter-reader.sh` makes the role, and `postgres-roles`
runs it on every start. The table grants are made again after each Status and
Auth migration, by `migrate/grants/status.sql` and `migrate/grants/auth.sql`,
because on a fresh cluster the tables do not exist until then, and a migration
that recreates a table drops its grant. Nothing is run by hand, after a
restore included: the deploy that follows makes them.

The grants name tables, with no default privileges: `logger.health_checks`,
and in `oauth` `operators`, `apps`, `permissions`, and `grants`, never
`guest_keys`, which holds the daily Exo guest key in plaintext and must never
be readable outside Auth. A new table for the tools needs a line in both the
script and the grants file; `tests/test-database-roles.py` checks what the
role can read.

## Postgres major upgrades

The pinned image is `postgres:18.6-alpine`. Postgres 18 images keep the
cluster in `/var/lib/postgresql/18/docker` and expect the volume one level
above, at `/var/lib/postgresql`, which `docker-compose.yml` mounts. A new
major cannot read the old major's data directory, so a major upgrade is a dump
and restore, never an image bump alone; Dependabot is told not to propose one.

On a laptop, whose data is disposable, the simplest path discards it. **This
destroys every row in every schema.**

```bash
./launch stop
docker volume rm biotron-infrastructure_postgres-data
./launch infra     # a fresh cluster, with its roles
./launch web       # each app's *-migrate service applies its schema
```

The four schemas are `oauth` (Auth), `logger` (Status), `calendar`, and
`sprinter`. Exo GUI has none.

For an instance with data worth keeping, use "Copying an instance's data" in
`deploy/README.md`: deploy a fresh instance on the new major, dump the old
one with `pg_dump --format=custom --no-owner --no-privileges`, restore it,
and deploy again, which makes every role and grant.

## Tests

```bash
python3 infra/tests/test-runtime-config.py   # Compose, launch, deploy scripts, workflows, CSP hash; no containers
python3 infra/tests/test-edge.py             # builds the edge image and checks what it serves
python3 infra/tests/test-database-roles.py   # Postgres, the role scripts, and the migrate image
```

The last two need a Docker daemon. CI runs all three in its `runtime
configuration` job.

## Releases

Pushes to `main` build commit-tagged images for staging, and release tags
send those same images to production, each to its own host. See
`deploy/README.md`.
