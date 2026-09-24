# Deploying staging and production

`staging` deploys to `uwbiotron.dev`. Merge its tested tree into `main` to
deploy the same images to `uwbiotron.ca`. Each host has separate data volumes,
credentials, domain, and Cloudflare tunnel. The checkout supplies Compose
manifests; the hosts pull application images and do not build them.

## GitHub Environments

Create `staging` and `production` in repository Settings > Environments.
Restrict them to the respective branches. A production reviewer is optional.
In each, set variables `DEPLOY_HOST` (its Tailscale name) and `DEPLOY_DOMAIN`
(`uwbiotron.dev` or `uwbiotron.ca`). Set these secrets in **both**, with
different credentials and complete file contents:

| Secret | Content |
| --- | --- |
| `SITE_ENV` | `apps/site/.env` |
| `CALENDAR_ENV` | `apps/calendar/.env` |
| `LOGGER_ENV` | `apps/logger/.env` |
| `AUTH_ENV` | `apps/auth/.env` |
| `EXO_ENV` | `apps/exo/.env` |
| `SPRINTER_ENV` | `apps/sprinter/.env` |
| `INFRA_ENV` | `infra/.env` |
| `CLOUDFLARE_TUNNEL_TOKEN` | This environment's tunnel token |
| `TS_OAUTH_CLIENT_ID`, `TS_AUDIENCE` | Tailscale federated identity credentials |

Start from each `.env.example`. Configure database URLs and passwords, OAuth
credentials and callback, cookie domain, HTTPS origins, logging tokens and
the application keys. The workflow adds `BASE_DOMAIN` to every app file and
adds `BASE_DOMAIN` and `CLOUDFLARE_TUNNEL_TOKEN_FILE` to `infra/.env`, overriding
any earlier entries for those keys. Use `.uwbiotron.dev` and `.uwbiotron.ca`
for the respective `COOKIE_DOMAIN` values, with `COOKIE_SECURE=true`.
Frontend builds use a neutral public domain marker replaced at startup.
Changing an Environment secret takes effect on its next deployment; the
workflow also supports manual dispatch from the corresponding branch.

## Host and network setup

Install Docker Compose, Git and Tailscale on both hosts. Enable Tailscale SSH
and create a `deploy` account that owns a clone at `/opt/biotron` and can run
Docker. Docker group membership grants root-equivalent access, so limit that
account and its Tailscale SSH policy to CI. Configure the federated identity
with `auth_keys` write permission and `tag:biotron-ci`; allow the tag to reach
only the deployment hosts on port 22 and log in as `deploy`. No public SSH
port is needed. The workflow writes the tunnel token to a private file at
`/opt/biotron/infra/cloudflare-tunnel-token`. Set up separate Cloudflare
tunnels and hostname routes. The GHCR packages must be public, or the hosts
must have read-only GHCR credentials for pulling them.

## Promotion

Pushing to `staging` builds and scans all 17 images for amd64 and arm64,
pushes them as `ghcr.io/biotrondesignteam/<service>:<full-staging-SHA>`,
transfers the env files over Tailscale SSH, then syncs the host checkout and
runs Compose `pull` and `up --no-build`. Production verifies the main source
tree equals a staging commit with a successful deployment and uses those
same tags, without rebuilding. Merge staging into main normally, without
squashing or editing conflicts. To change the promoted tree, deploy it to
staging first. Protect the branches to require CI and restrict direct pushes
to main.

Full SHA tags identify the source but are technically mutable on a rerun.
GHCR tag immutability or a digest lockfile would make the artifact guarantee
strict. Frontend images render their public domain marker when started, so
the registry image bytes are identical across environments. Rollback requires
restoring a previously deployed source tree and tag; database migrations may
not be reversible.
