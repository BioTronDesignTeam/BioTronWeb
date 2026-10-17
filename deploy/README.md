# Deploying staging and production

Every push to `main` deploys to staging at `uwbiotron.dev`. Pushing a `v*`
tag, such as `v1.4.0`, on a `main` commit deploys that commit's images to
production at `uwbiotron.ca`. Each host has separate data volumes,
credentials, domain, and Cloudflare tunnel. The checkout supplies Compose
manifests; the hosts pull application images and do not build them.

## GitHub Environments

Create `staging` and `production` in repository Settings > Environments.
Restrict `staging` to the `main` branch and `production` to `v*` tags. A
production reviewer is optional. In each, set variables `DEPLOY_HOST` (its
Tailscale name) and `DEPLOY_DOMAIN` (`uwbiotron.dev` or `uwbiotron.ca`). Set
these secrets in **both**, with different credentials and complete file
contents:

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
Changing an Environment secret takes effect on its next deployment.

Add a tag ruleset for `v*` so only release managers can create, move or
delete release tags. A release tag is the production deploy button.

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

## Releasing

A push to `main` builds and scans all 17 images for amd64 and arm64, pushes
them as `ghcr.io/biotrondesignteam/<service>:<full-commit-SHA>`, transfers
the env files over Tailscale SSH, then checks out that commit on the host and
runs Compose `pull` and `up --no-build`.

To release, tag a `main` commit that staging deployed successfully and push
the tag:

```sh
git tag -a v1.4.0 <commit> -m "v1.4.0"
git push origin v1.4.0
```

Production refuses a tag that is not on `main` or whose commit has no
successful staging deployment, and it reuses that commit's images without
rebuilding. Staging deploys one commit at a time, so when several merges land
together only the newest may deploy; tag that one. A fix must reach
production through `main` and a new tag.

To roll back, run the deploy workflow by hand from Actions and choose an
older release tag. Database migrations may not be reversible.

Full SHA tags identify the source but are technically mutable on a rerun.
GHCR tag immutability or a digest lockfile would make the artifact guarantee
strict. Frontend images render their public domain marker when started, so
the registry image bytes are identical across environments.
