# Server

Deployment and host operations for the BioTron platform. A host runs one or
more instances of the platform, such as `staging` and `prod`, behind one edge:

```text
Cloudflare -> cloudflared -> Nginx -> an instance's apps -> that instance's Postgres/Redis
```

Tailscale SSH is the separate administrative path. None of the edge or
application services require a publicly reachable host port. How a host is set
up, deployed, migrated, and rotated is in `deploy/README.md`.

## Layout

| Path | What |
|------|------|
| `docker-compose.yml` | Postgres and Redis of one instance; the root `docker-compose.yml` includes it |
| `edge/docker-compose.yml` | Nginx and cloudflared, the `biotron-edge` project shared by every instance |
| `infisical/` | The `biotron-secrets` project: Infisical with its own Postgres and Redis |
| `no-host-ports.yml` | Drops every host port from a named instance |
| `.env.example`, `edge/.env.example` | Local data-service settings; the edge's sites and tunnel token |
| `nginx/` | The versioned edge image, per-instance hostname routing, proxy headers, and health endpoint |
| `tests/` | Compose and launch regression checks; `python3 infra/tests/test-runtime-config.py` |

## Instances and networks

An instance is one Compose project holding its Postgres, Redis, and six apps:
`biotron` on a laptop, `biotron-<instance>` from
`./launch --instance <instance>`. Each has its own network,
`biotron-<instance>` (`biotron` on a laptop), its own volumes, and, when named,
no host ports.

- Inside an instance's network the services keep their plain aliases:
  `postgres`, `redis`, `oauth-manager`, `logger-api`, `calendar-api`,
  `site-web`, and so on. Apps reach each other and their data services by
  these names, and cannot see any other instance.
- Every web and API service also carries an instance-qualified alias, such as
  `staging-oauth-manager`, for the edge.
- The edge project has a private network holding only cloudflared and Nginx.
  `./launch edge` also attaches Nginx to the network of every instance named in
  `EDGE_SITES`, where it answers to `edge-proxy`. Nginx is the only container
  on more than one instance's network.

`./launch` creates the external networks when they are missing, so the edge
and the instances can start in any order.

## Hostname routing

`EDGE_SITES` in `/etc/biotron/edge.env` lists the instances this edge serves,
as `<instance>=<domain>[:noindex]` pairs:

```text
EDGE_SITES="staging=uwbiotron.dev:noindex prod=uwbiotron.ca"
```

When Nginx starts, `nginx/docker-entrypoint.d/40-biotron-sites.sh` renders
`nginx/sites/site.conf.template` once per instance. For `prod=uwbiotron.ca`:

| Hostname | Frontend | `/api/` backend |
|----------|----------|------------------|
| `uwbiotron.ca` | `prod-site-web:80` | none |
| `calendar.uwbiotron.ca` | `prod-calendar-web:80` | `prod-calendar-api:8080` |
| `auth.uwbiotron.ca` | `prod-oauth-web:80` | `prod-oauth-manager:8080` |
| `exogui.uwbiotron.ca` | `prod-exo-web:80` | `prod-exo-api:8080` |
| `status.uwbiotron.ca` | `prod-logger-web:80` | `prod-logger-api:8080` |
| `sprinter.uwbiotron.ca` | `prod-sprinter-web:80` | `prod-sprinter:8080` |

`www` redirects to the apex. Unknown hostnames return `404`. Upstreams are
resolved per request, so an instance that is down answers `502` and the
others are unaffected. `/api/` is removed before forwarding so an external
request for `/api/health` reaches the Go service as `/health`. Calendar
`/feeds/` is forwarded without removing the prefix.

`:noindex` marks an instance that search engines should leave alone, such as
staging: every response from its hostnames, errors included, carries
`X-Robots-Tag: noindex, nofollow`, and its `robots.txt` disallows everything.
An unmarked instance sends no such header and serves the site's own
`robots.txt`. This hides nothing from people; see "Search engines and access"
in `deploy/README.md`.

Frontend production builds use relative API bases such as `/api`.
OAuthManager's callback is therefore
`https://auth.<domain>/api/auth/github/callback`, one GitHub OAuth app per
instance.

## Cloudflare Tunnel

One remotely managed tunnel per host, not per instance: the edge is shared.
Add every hostname of every instance in `EDGE_SITES`, plus each `www`, to it.
Every public hostname uses the same origin service:

```text
http://nginx:8080
```

Nginx selects the instance and application from the original HTTP `Host`
header. The tunnel token lives in `/etc/biotron/edge.env` as
`CLOUDFLARE_TUNNEL_TOKEN`; Compose hands it to cloudflared as the read-only
secret file `/run/secrets/cloudflare_tunnel_token`. Rotating it requires
recreating cloudflared.

## Local use

```bash
./launch infra                       # Postgres and Redis, reading infra/.env
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
`http://cloudflared:2000` for future Logger integration; it is not published
on the host.

Test each public hostname through Cloudflare after its instance is running.
A `502` means Nginx is reachable but that instance's application alias is not.

## Security boundary

- Permit Cloudflare Tunnel's required outbound traffic, including TCP/UDP
  `7844`; no inbound HTTP/HTTPS rule is needed.
- Do not open public SSH. Install Tailscale on the host and authorize Tailscale
  SSH using tailnet grants and SSH rules.
- Nginx and cloudflared use read-only root filesystems and
  `no-new-privileges`. The tunnel token is a read-only Compose secret.
- Nginx accepts only the configured instances' hostnames and forwards
  `CF-Connecting-IP` as the client address. It does not expose Docker's socket.
- Each instance has its own Postgres and Redis on its own network. A named
  instance publishes no host ports; administer it with
  `docker compose -p biotron-<instance> exec`.
- Nginx is on every instance's network, so it could reach each instance's
  Postgres and Redis; nothing in its configuration does.

## Startup and updates

All containers use `restart: unless-stopped`, so Docker restores them after a
cold boot with the settings they were deployed with. Enable Docker and
Tailscale on the host; cloudflared is not installed as a native systemd
service.

The local edge image is `biotron-edge-nginx:local`. See `deploy/README.md`
for the deployment workflow and GHCR image tags.

## Data service major upgrades: Postgres 16 to 18, Redis 7 to 8

The pinned images moved from `postgres:16-alpine` and `redis:7-alpine` to
`postgres:18.6-alpine` and `redis:8.10.1-alpine`. Redis needs nothing from you.
Postgres does: **a Postgres major upgrade cannot read an existing data
directory from an older major**, so the existing
`biotron-infrastructure_postgres-data` volume has to be dealt with deliberately
before the stack will come up.

Nothing in this repository performs any of the steps below. They are commands
for a human to run at a moment of their choosing.

### What changes about the Postgres volume

Postgres 18 images store the cluster in a major-version-specific directory and
expect the mount one level above it:

| | 16 | 18 |
|---|---|---|
| `PGDATA` | `/var/lib/postgresql/data` | `/var/lib/postgresql/18/docker` |
| Mount point | `/var/lib/postgresql/data` | `/var/lib/postgresql` |

`docker-compose.yml` has been updated to mount `postgres-data` at
`/var/lib/postgresql` to match. This is why the old volume cannot simply be
carried across: its contents sit at the layout 16 used.

The failure is loud, not silent. Starting 18.6 against a volume that still
holds a 16 cluster aborts with a long explanatory error rather than quietly
initialising an empty database:

```text
Error: in 18+, these Docker images are configured to store database data in a
       format which is compatible with "pg_ctlcluster" ...
       Counter to that, there appears to be PostgreSQL data in:
         /var/lib/postgresql/data
```

### Recreating the volume (the normal procedure here)

This volume holds local development data only, so the expected path is simply
to discard it and rebuild the schemas from each repository's migrations.

**This destroys every row in every schema.** It is the right thing to do in
this workspace, and the wrong thing to do anywhere with data worth keeping.

```bash
# 1. Stop the stack.
cd Server
docker compose down

# 2. Delete the old 16 volume.
docker volume rm biotron-infrastructure_postgres-data

# 3. Bring it back up. Postgres 18.6 initialises a fresh cluster.
docker compose up -d
docker compose ps
docker compose exec postgres pg_isready -U biotron -d biotron

# 4. Rebuild the five schemas.
for repo in OAuthManager Logger exo-gui BiotronCalendar Sprinter; do
  (cd "../$repo/prisma" && npm install && npm run deploy)
done
```

The five repositories each own one schema inside the single shared `biotron`
database — `oauth`, `logger`, `exo`, `calendar` and `sprinter` — so they do not
contend and the order they run in does not matter. All five must run before the
platform is fully functional. Sprinter currently defines no models and has no
`prisma/migrations` directory, so its `migrate deploy` applies nothing; that is
expected, not a failure.

### If you ever need to preserve the data

Dump from a temporary 16 container mounted the way 16 expects, then restore
into the new 18 cluster. The old image must be used for the dump, because 18
cannot read the 16 directory at all.

```bash
# Dump, with the stack down. Mount the existing volume at the OLD path.
docker run --rm -d --name pg-dump-tmp \
  -e POSTGRES_PASSWORD="$POSTGRES_PASSWORD" \
  -v biotron-infrastructure_postgres-data:/var/lib/postgresql/data \
  postgres:16-alpine
until docker exec pg-dump-tmp pg_isready -U biotron -d biotron; do sleep 1; done
docker exec pg-dump-tmp pg_dumpall -U biotron > biotron-16.sql
docker rm -f pg-dump-tmp

# Then delete the volume, start 18.6, and load the dump back in.
docker volume rm biotron-infrastructure_postgres-data
docker compose up -d postgres
until docker compose exec -T postgres pg_isready -U biotron -d biotron; do sleep 1; done
docker compose exec -T postgres psql -U biotron -d biotron < biotron-16.sql
```

Keep `biotron-16.sql` until the restore has been verified. `pg_dumpall` carries
roles as well as data; the `biotron` role is recreated from `POSTGRES_USER` and
`POSTGRES_PASSWORD` regardless, so role errors on restore are harmless.

`pg_upgrade` is the other option and is why the mount now sits at
`/var/lib/postgresql`: both clusters can live inside one mount point. It needs
an image carrying both major versions' binaries, which neither official image
provides, so dump and restore is the simpler path at this size.

### Connection settings are unaffected

Checked against both images before the bump: `password_encryption`,
`listen_addresses` and `port` are identical in 16 and 18 (`scram-sha-256`, `*`,
`5432`), the `biotron` role is stored as `scram-sha-256` in both, and the
generated `pg_hba.conf` still ends in `host all all all scram-sha-256`. The
`postgresql://biotron:...@postgres:5432/biotron?schema=<name>` URLs the five
repositories use need no change, and `pgx` already speaks SCRAM.

### Redis 7 to 8

No procedure required. The RDB and AOF formats are forward compatible, so 8.10.1
opens a volume written by 7 and starts normally; this was confirmed against a
throwaway volume seeded on `redis:7-alpine`, including that existing keys
survived and `redis-cli ping` still answers the compose healthcheck.

Nothing this workspace asks of Redis is affected. Logger uses it for the health
cache and OAuthManager for grant-set caching, all through
`go-redis/v9`, which supports Redis 8 — and both are plain key, list and TTL
operations, well away from anything Redis 8 changed. Redis 8 folds in the
former Redis Stack modules and changes defaults around them, none of which this
workspace enables. The instance remains unauthenticated on the `biotron`
network, which is a separate open item (MEDIUM-3 in the security audit) and is
neither improved nor worsened by this bump.

## Read-only role for Sprinter

Sprinter's model tools read the `logger` and `oauth` schemas (today only
`logger.health_checks` and four `oauth` tables), which Logger and
Auth own. The read-only guarantee lives in Postgres, not in Sprinter's Go
code, through a dedicated role: `sprinter_reader`.

A fresh cluster gets the role from
`postgres/init/10-sprinter-reader.sh`, mounted into the container's
`/docker-entrypoint-initdb.d/` and run once, automatically, the first time the
volume is empty. An existing cluster needs the same script run by hand:

```bash
docker compose exec -T postgres bash < infra/postgres/init/10-sprinter-reader.sh
```

The Postgres container already has `SPRINTER_READER_PASSWORD` in its
environment. For a named instance on a host, add `-p biotron-<instance>` and
use the copy in `/opt/biotron/<instance>/infra/postgres/init/`.

Rerun this command after Logger's and Auth's first migrations on a fresh
cluster. At init time neither schema has tables yet, so the script's grants
have nothing to grant; rerunning it once the apps have created their tables
picks them up. Rerun it on an existing cluster too after the script changes:
it revokes the schema-wide `logger` grant that earlier versions gave.

Both schemas get explicit grants on named tables only, with no default
privileges: `logger.health_checks`, and in `oauth` `operators`, `apps`,
`permissions`, and `grants`, never `sessions` or `guest_keys` — `guest_keys` holds the daily Exo guest key in
plaintext, so it must never be readable outside Auth.

## Releases

Pushes to `main` build commit-tagged images for staging, and release tags
send those same images to production, each to its own instance. See
`deploy/README.md`.
