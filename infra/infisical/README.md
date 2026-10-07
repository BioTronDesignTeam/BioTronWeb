# Infisical

This stack owns a separate Postgres, Redis, network, and data volumes. It does
not use the application database or join `biotron`. `./launch all` and
`./launch stop` do not touch it. Start it before deploying app secrets.

## Bootstrap once

From the repository root:

```bash
sudo install -d -m 0700 /etc/biotron/infisical
sudo install -m 0600 infra/infisical/.env.example /etc/biotron/infisical/bootstrap.env
sudoedit /etc/biotron/infisical/bootstrap.env
```

Fill `ENCRYPTION_KEY` using `openssl rand -hex 16`, `AUTH_SECRET` using
`openssl rand -base64 32`, and `INFISICAL_DB_PASSWORD` using
`openssl rand -hex 32`. Use a hex database password so the connection URI
needs no percent encoding. These bootstrap credentials cannot live in
Infisical itself. Back them up securely, especially `ENCRYPTION_KEY`.

Run with an account that can read the bootstrap file and access Docker:

```bash
sudo ./launch secrets
curl --fail http://localhost:8090/api/status
```

`INFISICAL_BOOTSTRAP_FILE` can select a different absolute file path. The
UI binds to loopback only. For initial administration, use a Tailscale SSH
forward, for example `ssh -L 8090:127.0.0.1:8090 admin@host`, then open
`http://localhost:8090`. Create the administrator before granting access to
others. For shared use, configure private HTTPS ingress, set
`INFISICAL_SITE_URL` to that address, and recreate the Infisical container.
There is no public route or Cloudflare Tunnel added by this change.

Create a BioTronWeb project, environments `dev`, `staging`, and `prod`, and a
folder `/deploy` in each. Create separate machine identities for staging and
production with read access only to their own environment and `/deploy`.
Enable Universal Auth for them. Keep their client credentials outside Git,
separate from the Infisical server's bootstrap file.

## Move application secrets

Put these keys in `/deploy`. Values come from the existing untracked app
files; no real secrets belong in examples or commits.

| Infisical key | Old source |
|---|---|
| `AUTH_DATABASE_URL` | Auth `DATABASE_URL` |
| `CALENDAR_DATABASE_URL` | Calendar `DATABASE_URL` |
| `EXO_DATABASE_URL` | Exo `DATABASE_URL` |
| `LOGGER_DATABASE_URL` | Logger `DATABASE_URL` |
| `SPRINTER_DATABASE_URL` | Sprinter `DATABASE_URL` |
| `SPRINTER_READ_DATABASE_URL` | Sprinter `READ_DATABASE_URL` |
| `AUTH_GITHUB_CLIENT_ID` | Auth `GITHUB_CLIENT_ID` |
| `AUTH_GITHUB_CLIENT_SECRET` | Auth `GITHUB_CLIENT_SECRET` |
| `SPRINTER_DISCORD_TOKEN` | Sprinter `DISCORD_TOKEN` |
| `SPRINTER_GEMINI_API_KEY` | Sprinter `GEMINI_API_KEY` |
| `LOGGER_INGEST_TOKEN` | Shared by Logger and its clients |
| `POSTGRES_PASSWORD` | Infra database password |
| `SPRINTER_READER_PASSWORD` | Infra read-only role password |
| `CLOUDFLARE_TUNNEL_TOKEN` | Contents of the old tunnel-token file |

Keep every existing database URL's schema and reader permissions. Importing
values does not rotate existing Postgres role passwords. Changing the
Postgres container's password environment variable does not change a password
in an existing data volume; alter the role in Postgres when rotating it.

All app Compose inputs support `APP_VARIABLE`, such as
`AUTH_FRONTEND_URL` or `LOGGER_WEB_PORT`. These names are translated to the
existing variable names inside containers. Unprefixed values remain a local
development fallback; **do not export generic `DATABASE_URL`, `WEB_PORT`,
`API_PORT`, `FRONTEND_URL`, or `VITE_*` into a shared deployment shell**.
Put all injected secrets in `/deploy` using the names above, not repeated
unprefixed names in app folders. App identities are not isolated from each
other in this first phase: one deployment identity retrieves that environment's
app secrets. A per-service retrieval model would be a separate change.

Public configuration stays outside the secrets manager. Copy
`infra/runtime-config.example.sh` to `/etc/biotron/runtime-config.sh`, choose
the domain, set admin IDs and any other public overrides, and source it.
`SPRINTER_DISCORD_GUILD_ID`, model settings, ports and timeouts can also be
set here. Only declared Compose inputs reach containers; the machine identity
token and bootstrap keys are not passed into the apps.

## Deploy

Install the [Infisical CLI](https://infisical.com/docs/cli/usage) and `jq` on
the deployment host. Authenticate interactively with `infisical login`, or
use a machine identity in a shell whose credentials are loaded securely:

```bash
export INFISICAL_DOMAIN=http://localhost:8090
# INFISICAL_CLIENT_ID and INFISICAL_CLIENT_SECRET are supplied out of band.
export INFISICAL_TOKEN="$(infisical login --method=universal-auth \
  --client-id="$INFISICAL_CLIENT_ID" --client-secret="$INFISICAL_CLIENT_SECRET" \
  --silent --plain)"
. /etc/biotron/runtime-config.sh
infisical run --projectId=YOUR_PROJECT_ID --env=staging --path=/deploy -- \
  ./launch all --runtime --build
unset INFISICAL_TOKEN INFISICAL_CLIENT_SECRET
```

Use `--env=prod` with the production identity and public configuration on the
production host. The local loopback API keeps host-side retrieval independent
of the application's edge. Remote CLI users need the private HTTPS address.
Never enable shell tracing while handling credentials.

`--runtime` ignores root, infra, and app `.env` files. Missing database URLs
and the selected Logger ingestion token fail before starting the apps, while
excluded apps need no secrets. Infra still requires its domain and database
credentials when Compose parses it, including for `--local`.

Cloudflared still reads `/run/secrets/cloudflare_tunnel_token`, now backed by
a Compose secret sourced from `CLOUDFLARE_TUNNEL_TOKEN` in the deployment
process. Compose 2.30+ is the baseline (also supports environment-backed
secrets); it is not Docker Swarm's encrypted secret store.

Secrets are fetched at deploy/container creation time. Running containers keep
their values through an ordinary restart, including a host reboot. Rotation
requires fetching again and recreating affected containers, for example
rerunning the command above. Compose will detect changed environment values;
for a rotated environment-backed tunnel secret, explicitly force recreation:

```bash
infisical run --projectId=YOUR_PROJECT_ID --env=staging --path=/deploy -- \
  docker compose -f infra/docker-compose.yml --env-file /dev/null \
  up -d --force-recreate cloudflared
```

Docker administrators can inspect app environment secrets. Frontend `VITE_*`
settings are public and remain build-time values; runtime frontend config is
not included here.

After verifying the injected deployment, remove the obsolete app and infra
secret files from the host. Local developers may keep the existing `.env`
workflow: `./launch` selects files that exist, and missing files are optional.
Direct root `docker compose` defaults to injected variables only; direct
Compose from an app directory still loads that app's `.env`.

## Maintenance and recovery

```bash
umask 077
sudo ./launch stop secrets
sudo ./launch secrets
sudo docker compose -f infra/infisical/docker-compose.yml \
  --env-file /etc/biotron/infisical/bootstrap.env ps
sudo docker compose -f infra/infisical/docker-compose.yml \
  --env-file /etc/biotron/infisical/bootstrap.env exec -T db \
  pg_dump -U infisical infisical > infisical-backup.sql
```

Protect the dump with restrictive permissions and encryption. Back up the
bootstrap file separately. Test restoration into a separate stack using the
same image version and encryption key before upgrading. Do not delete
`biotron-secrets_postgres-data`, use `down -v`, regenerate the encryption key,
or point this stack at the application Postgres volume. Redis's volume stores
queues and cache. Update `INFISICAL_IMAGE` deliberately after reviewing
release notes and taking a backup. The bootstrap example pins an initial
version; it is not an automatic upgrade policy.

Official references: [Compose deployment](https://infisical.com/docs/self-hosting/deployment-options/docker-compose)
and [CLI authentication](https://infisical.com/docs/cli/usage).

Configuration regression checks (no daemon needed):

```bash
python3 infra/tests/test-runtime-config.py
```
