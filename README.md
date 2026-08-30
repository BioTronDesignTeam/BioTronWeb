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
| `services/` | Host systemd timers and watchdog templates |

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
