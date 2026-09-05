# Server

Deployment and host operations for the BioTron platform. The personal staging
server and Oracle production VM use the same container topology:

```text
Cloudflare -> cloudflared -> Nginx -> application containers -> Postgres/Redis
```

Tailscale SSH is the separate administrative path. None of the edge or
application services require a publicly reachable host port.

## Layout

| Path | What |
|------|------|
| `docker-compose.yml` | Nginx, cloudflared, the shared Postgres instance, Redis, and their Docker networks |
| `.env.example` | Environment domain, tunnel-token path, image name, and shared data-service settings |
| `nginx/` | The versioned edge image, hostname routing, proxy headers, and health endpoint |

## Networks

- `biotron-edge` contains only `cloudflared` and Nginx. It publishes no host
  ports. `cloudflared` initiates the outbound connection to Cloudflare and
  reaches Nginx at `http://nginx:8080`.
- `biotron` contains Nginx, the application containers, Postgres, and Redis.
  Nginx is the only service attached to both networks.

Application stacks join the external `biotron` network. The frontends have
stable aliases (`site-web`, `calendar-web`, `oauth-web`, `exo-web`,
`logger-web`, and `sprinter-web`); APIs already have their own aliases.

## Hostname routing

For `BASE_DOMAIN=biotron-dev.com`, Nginx routes:

| Hostname | Frontend | `/api/` backend |
|----------|----------|------------------|
| `www.biotron-dev.com` | `site-web:80` | none |
| `calendar.biotron-dev.com` | `calendar-web:80` | `calendar-api:8080` |
| `auth.biotron-dev.com` | `oauth-web:80` | `oauth-manager:8080` |
| `exo.biotron-dev.com` | `exo-web:80` | `exo-api:8080` |
| `logs.biotron-dev.com` | `logger-web:80` | `logger-api:8080` |
| `sprinter.biotron-dev.com` | `sprinter-web:80` | `sprinter:8080` |

The apex hostname redirects to `www`. Unknown hostnames return `404`.
`/api/` is removed before forwarding so an external request for `/api/health`
reaches the Go service as `/health`. Calendar `/feeds/` is forwarded without
removing the prefix.

Frontend production builds should use relative API bases such as `/api`.
OAuthManager's staging callback is therefore:

```text
https://auth.biotron-dev.com/api/auth/github/callback
```

Production uses the same hostname layout under its own `BASE_DOMAIN`.

## First-time setup

### 1. Create the environment file

```bash
cp .env.example .env
```

Change at least `POSTGRES_PASSWORD`. Staging uses
`BASE_DOMAIN=biotron-dev.com`; production uses its production domain.

### 2. Create a Cloudflare Tunnel

Create one remotely managed tunnel for each environment in Cloudflare Zero
Trust. Do not reuse the staging tunnel in production.

Add each public hostname listed above, plus the apex hostname, to the tunnel.
Every public hostname uses the same origin service:

```text
http://nginx:8080
```

Nginx selects the application from the original HTTP `Host` header.

### 3. Install the tunnel token

Store only the tunnel token on the host; never commit it or put it directly in
`.env`. The default path is:

```text
/etc/biotron/secrets/cloudflare-tunnel-token
```

The parent directory should be accessible only to root. The token file must be
readable inside the non-root cloudflared container; a root-only directory with
a read-only file satisfies both requirements:

```bash
sudo install -d -o root -g root -m 0700 /etc/biotron/secrets
sudo install -o root -g root -m 0444 ./cloudflare-tunnel-token \
  /etc/biotron/secrets/cloudflare-tunnel-token
```

Set `CLOUDFLARE_TUNNEL_TOKEN_FILE` in `.env` if a different absolute path is
used. Rotating the tunnel token requires recreating the `cloudflared`
container.

### 4. Start shared infrastructure and the edge

```bash
docker compose up -d --build
```

Then start each application stack. They discover the `biotron` network and
their dependencies by Docker alias rather than host ports.

## Verification

```bash
docker compose ps
docker compose exec nginx nginx -t
docker compose exec nginx wget -qO- http://127.0.0.1:8080/_edge/health
docker compose logs --tail 100 nginx cloudflared
```

The Nginx health response is `ok`. Cloudflared exposes its readiness and
metrics endpoint to the private edge network at `http://cloudflared:2000` for
future Logger integration; it is not published on the host.

Test each public hostname through Cloudflare after its application is running.
A `502` means Nginx is reachable but that specific application alias is not.

## Security boundary

- Permit Cloudflare Tunnel's required outbound traffic, including TCP/UDP
  `7844`; no inbound HTTP/HTTPS rule is needed.
- Do not open public SSH. Install Tailscale on the host and authorize Tailscale
  SSH using tailnet grants and SSH rules.
- Nginx and cloudflared use read-only root filesystems and
  `no-new-privileges`. The tunnel token is a read-only Compose secret.
- Nginx accepts only the selected application hostnames and forwards
  `CF-Connecting-IP` as the client address. It does not expose Docker's socket.
- Postgres and Redis are one instance each per environment. Their current host
  bindings are loopback-only for administration and are never public.

## Startup and updates

All containers use `restart: unless-stopped`, so Docker restores them after a
cold boot. Enable Docker and Tailscale on the host; cloudflared is not installed
as a native systemd service.

For releases, build multi-architecture application images once, test immutable
digests in staging, and promote the same digests to production. The local image
name `biotron-edge-nginx:local` can be replaced through `NGINX_IMAGE` when the
edge image is published by CI.

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
cache and log tails and OAuthManager for grant-set caching, all through
`go-redis/v9`, which supports Redis 8 — and both are plain key, list and TTL
operations, well away from anything Redis 8 changed. Redis 8 folds in the
former Redis Stack modules and changes defaults around them, none of which this
workspace enables. The instance remains unauthenticated on the `biotron`
network, which is a separate open item (MEDIUM-3 in the security audit) and is
neither improved nor worsened by this bump.

## Releases

The plan is to build each image once in CI, pin it by digest, prove it in
staging, and promote the same digest to production without rebuilding. The
release workflow builds and pushes the images; recording digests per
environment and deploying from that record is not built yet.
