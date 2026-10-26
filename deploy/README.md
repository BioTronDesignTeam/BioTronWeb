# Deploying staging and production

Every push to `main` deploys to staging at `uwbiotron.dev`. Pushing a `v*`
tag, such as `v1.4.0`, on a `main` commit deploys that commit's images to
production at `uwbiotron.ca`. The two instances can run side by side on one
host or each on its own, and nothing either one does can stop or change the
other.

The host keeps no checkout and no `.env` files. Each deploy downloads the
commit's Compose files from GitHub, pulls that commit's images from GHCR, and
takes its settings from Infisical at the moment it starts the containers.
Only a handful of bootstrap credentials, which cannot live in Infisical, sit
on disk in root-only files under `/etc/biotron`. The Infisical CLI keeps an
offline copy of fetched secrets only for a signed-in user, never for the
machine identity a deploy uses, and a deploy runs it with a throwaway `HOME`
on the `/run` tmpfs, so the CLI leaves no copy of them on disk. The settings
of running containers still sit in Docker's own state under
`/var/lib/docker`, readable by root only.

## How it fits together

```text
Cloudflare ─ tunnel ─ cloudflared ─ Nginx ─┬─ biotron-staging network: staging apps, Postgres
              (biotron-edge project)       └─ biotron-prod network:    prod apps, Postgres
```

| Compose project | What | Settings come from |
| --- | --- | --- |
| `biotron-staging` | Staging's Postgres and six apps, images tagged with the `main` commit | Infisical `staging`, through `/etc/biotron/staging/identity.env` |
| `biotron-prod` | Production's, images of the release tag's commit | Infisical `prod`, through `/etc/biotron/prod/identity.env` |
| `biotron-edge` | Nginx and cloudflared, shared by both | `/etc/biotron/edge.env` |
| `biotron-secrets` | Infisical with its own Postgres and Redis | `/etc/biotron/infisical.env` |

What keeps the two instances apart:

- **Networks.** Each instance has its own Docker network, `biotron-staging` or
  `biotron-prod`, with the same service aliases (`postgres`,
  `oauth-manager`, `logger-api`) inside. A staging container cannot resolve or
  reach anything of production's. Only Nginx joins both, and it reaches the
  apps through aliases that carry the instance name, such as
  `prod-oauth-manager`.
- **Data.** Each instance runs its own Postgres on its own volume
  (`biotron-staging_postgres-data` and so on), with different passwords.
  Separate instances, rather than two databases in one server, mean a staging
  migration, restart, upgrade, or full disk of logs cannot touch production,
  and no role exists that can read both. The cost is one more Postgres
  process, small next to the host's memory.
- **Settings.** Each instance has its own Infisical environment and its own
  machine identity, which can read nothing else. Domains, cookie domains,
  GitHub Apps, and session keys all differ.
- **Names and ports.** Container names come from the project, so they never
  collide, and a named instance publishes no host ports at all
  (`infra/no-host-ports.yml`). Use `docker compose -p biotron-prod exec` for
  administration, from a directory with no Compose file, such as `/`, so
  Compose goes by the project name alone.
- **Lifecycles.** Deploying, stopping, or breaking one project leaves the others
  running. Nginx resolves each app per request, so a stopped instance answers
  `502` without affecting the other's hostnames.

On the host, `/opt/biotron/<staging|prod|edge|secrets>` holds the Compose files
of the commit last deployed there, root-owned and free of secrets, and a
`REVISION` file. The previous deploy's files stay beside it as `.previous`.

## Releasing

A push to `main` builds and scans all 17 images for amd64 and arm64, pushes
them as `ghcr.io/biotrondesignteam/<service>:<full-commit-SHA>`, and runs
`biotron-deploy staging <commit>` on the host over Tailscale SSH.

To release, tag a `main` commit that staging deployed successfully and push
the tag:

```sh
git tag -a v1.4.0 <commit> -m "v1.4.0"
git push origin v1.4.0
```

Production refuses a tag that is not on `main` or whose commit has no
successful staging deployment, and it reuses that commit's images without
rebuilding: `biotron-deploy prod <commit>`. The host checks again, through the
GitHub API, that the commit is on `main` and that a `v*` tag names it, so
`biotron-deploy prod` refuses any other commit, whoever asks. While both
instances share a host, a release also redeploys the shared edge from the same commit, so a change to
`infra/nginx` reaches staging's hostnames with the next release, not with the
merge. Staging deploys one commit at a time, so when several merges land
together only the newest may deploy; tag that one. A fix must reach
production through `main` and a new tag.

## Rolling back

Run the deploy workflow by hand from Actions and choose an older release tag.
It redeploys that tag's commit to production, Compose files and images both.
Database migrations may not be reversible; check what the newer release
migrated before rolling back across it.

From the host, the same thing is `sudo biotron-deploy prod <older-commit>`,
for the commit of an older release tag.
Staging rolls back the same way, by redeploying an older `main` commit.

## Search engines and access

Staging is marked `:noindex` in the edge's `EDGE_SITES`, so every staging
response carries `X-Robots-Tag: noindex, nofollow` and staging's `robots.txt`
disallows everything. Production is not marked and serves the site's own
`robots.txt`. The flag belongs to the instance, not to a list of hostnames.

This only asks well-behaved crawlers to stay away. Staging is still public:
anyone with the address can use it. Whether to restrict it is an open
decision. The likely choice is Cloudflare Access in front of the staging
hostnames, which needs no code here; mind that calendar feed URLs and the
GitHub OAuth callback must still be reachable by whoever needs them.

## GitHub setup

Create the `staging` and `production` Environments in repository Settings >
Environments. Restrict `staging` to the `main` branch and `production` to `v*`
tags. A production reviewer is optional. In each, set:

| Name | Kind | Value |
| --- | --- | --- |
| `DEPLOY_HOST` | variable | The Tailscale name of the host that runs this instance |
| `DEPLOY_EDGE` | variable | `true` in exactly one Environment per host: `production` when both instances share a host, both when each has its own |
| `TS_OAUTH_CLIENT_ID`, `TS_AUDIENCE` | secrets | Tailscale federated identity credentials |

No application secret lives in GitHub. Delete the old `SITE_ENV`,
`CALENDAR_ENV`, `LOGGER_ENV`, `AUTH_ENV`, `EXO_ENV`, `SPRINTER_ENV`,
`INFRA_ENV`, and `CLOUDFLARE_TUNNEL_TOKEN` secrets and the `DEPLOY_DOMAIN`
variable once the host runs from Infisical.

Anyone with write access may push a `v*` tag: a release tag is the
production deploy button, and making a release is part of learning the
project. Production still deploys only a tag on a `main` commit that staging
deployed, so a tag cannot ship untested code.

Each Environment has its own Tailscale federated identity, with `auth_keys`
write permission and its own tag: `tag:biotron-ci-staging` for `staging` and
`tag:biotron-ci-prod` for `production`. In the tailnet policy, let each tag
reach only the deployment hosts on port 22 and log in only as its own
account, so a staging run, or a leak of staging's credentials, cannot reach
production:

```json
"ssh": [
  { "action": "accept", "src": ["tag:biotron-ci-staging"], "dst": ["tag:biotron-host"], "users": ["deploy-staging"] },
  { "action": "accept", "src": ["tag:biotron-ci-prod"], "dst": ["tag:biotron-host"], "users": ["deploy-prod"] }
]
```

`tag:biotron-host` stands for however the policy names the deployment hosts.
No public SSH port is needed.

## What Infisical holds

One project, environments `staging` and `prod`, a folder `/deploy` in each.
Each environment needs:

| Key | What |
| --- | --- |
| `BASE_DOMAIN` | `uwbiotron.dev` for staging, `uwbiotron.ca` for production |
| `POSTGRES_PASSWORD` | The instance's Postgres owner password. `openssl rand -hex 32` |
| `SPRINTER_READER_PASSWORD` | The read-only `sprinter_reader` role's password. `openssl rand -hex 32` |
| `AUTH_GITHUB_APP_ID`, `AUTH_GITHUB_APP_CLIENT_ID`, `AUTH_GITHUB_APP_CLIENT_SECRET`, `AUTH_GITHUB_APP_PRIVATE_KEY` | This instance's GitHub App. See "The GitHub App" in `apps/auth/README.md` |
| `AUTH_SESSION_SIGNING_KEY` | Signs Auth's session cookies. `openssl rand -base64 48`; changing it signs everyone out |
| `BIOTRON_ENV` | `staging` or `production`. Tags Grafana and PostHog events |
| `GRAFANA_OTLP_ENDPOINT`, `GRAFANA_OTLP_USER`, `GRAFANA_OTLP_TOKEN` | Optional. Grafana Cloud logs, where every service's events go; see `go/logclient/README.md` |
| `POSTHOG_API_KEY`, `POSTHOG_HOST` | Optional. PostHog audit events |
| `SPRINTER_DISCORD_ENABLED` | `true` in staging. Leave unset in production, where Sprinter runs without its bot |
| `AUTH_SUPERUSER_GITHUB_IDS` | Optional. Numeric GitHub IDs allowed to bootstrap administration |
| `SPRINTER_DISCORD_TOKEN`, `SPRINTER_DISCORD_GUILD_ID`, `SPRINTER_GEMINI_API_KEY` | Optional. Sprinter's bot and model |

`deploy/instance-defaults.sh` derives the rest from these: every app's
`DATABASE_URL`, Sprinter's read-only URL, and the domain-dependent URLs, CORS
lists, and cookie domain. Any compose input with an app prefix, such as
`AUTH_LOG_LEVEL` or `CALENDAR_DEFAULT_TIMEZONE`, can be set in Infisical too,
and a value set there always wins over a derived one. The Postgres passwords
go into URLs unescaped, so use hex.

Each GitHub App's callback is
`https://auth.<BASE_DOMAIN>/api/auth/github/callback`. Use one App per
instance; never share one between staging and production.

## Bootstrap files

These cannot live in Infisical: the first holds Infisical's own keys, the
others are what a deploy uses to reach it, and the tunnel must start without
it. Each is owned by root with mode `0600` in a root-only directory, and
`biotron-deploy` refuses to run if one is not. Create each the same way, so it
never exists with wider permissions:

```sh
sudo install -d -o root -g root -m 0700 /etc/biotron /etc/biotron/staging /etc/biotron/prod
sudo install -o root -g root -m 0600 /dev/null /etc/biotron/edge.env
sudoedit /etc/biotron/edge.env
```

| File | Keys |
| --- | --- |
| `/etc/biotron/infisical.env` | `ENCRYPTION_KEY`, `AUTH_SECRET`, `INFISICAL_DB_PASSWORD`, optionally `INFISICAL_SITE_URL`, `INFISICAL_PORT`, `INFISICAL_IMAGE`. See `infra/infisical/.env.example` |
| `/etc/biotron/edge.env` | `CLOUDFLARE_TUNNEL_TOKEN` and `EDGE_SITES="staging=uwbiotron.dev:noindex prod=uwbiotron.ca"` |
| `/etc/biotron/staging/identity.env` | `INFISICAL_CLIENT_ID`, `INFISICAL_CLIENT_SECRET`, `INFISICAL_PROJECT_ID` of the staging machine identity |
| `/etc/biotron/prod/identity.env` | The same for the production identity |

An identity file may also set `INFISICAL_ENVIRONMENT` (default: the instance
name), `INFISICAL_PATH` (default `/deploy`), and `INFISICAL_API_URL` (default
`http://127.0.0.1:8090/api`). Back up `infisical.env` somewhere safe off the
host: without its `ENCRYPTION_KEY` nothing in Infisical can be read again.

## Setting up a host from scratch

1. Install Docker Engine with the Compose plugin, `curl`, `jq`, `tar`,
   Tailscale with Tailscale SSH, and the
   [Infisical CLI](https://infisical.com/docs/cli/overview). Check that the
   filesystem holding `/var/lib/docker` has room for two sets of images, about
   10 GB free; see "Disk space" below.
2. Create the two accounts CI logs in as, one per Environment. Neither needs
   the Docker group. Each may run the deploy script as root for its own
   targets and nothing else: `deploy-staging` for staging, `deploy-prod` for
   production and the shared edge. Infisical itself (`secrets`) is deployed by
   an administrator by hand, never by CI. The script refuses anything but
   exactly a target and a 40-character commit, so the trailing `*` cannot
   smuggle in a second target.

   ```sh
   sudo useradd --create-home --shell /bin/bash deploy-staging
   sudo useradd --create-home --shell /bin/bash deploy-prod
   printf '%s\n' \
     'deploy-staging ALL=(root) NOPASSWD: /usr/local/sbin/biotron-deploy staging *' \
     'deploy-prod ALL=(root) NOPASSWD: /usr/local/sbin/biotron-deploy prod *, /usr/local/sbin/biotron-deploy edge *' |
     sudo install -o root -g root -m 0440 /dev/stdin /etc/sudoers.d/biotron-deploy
   sudo visudo -c
   ```

   A host that serves one instance creates only that instance's account and
   also lets it run `biotron-deploy edge *`, since it owns that host's edge.
   A host set up with the old single `deploy` account: create the two
   accounts, replace `/etc/sudoers.d/biotron-deploy` as above, move the
   Tailscale policy to the two tags, then `sudo userdel -r deploy`.

3. Install the deploy script from a `main` commit, root-owned so neither CI
   account can change it. Reinstall it the same way whenever
   `deploy/biotron-deploy` changes; a deploy prints a warning when the copy on
   the host differs from the commit it deploys.

   ```sh
   curl -fsSL https://raw.githubusercontent.com/BioTronDesignTeam/BioTronWeb/<commit>/deploy/biotron-deploy |
     sudo install -o root -g root -m 0755 /dev/stdin /usr/local/sbin/biotron-deploy
   ```

4. Let root pull from GHCR: make the `biotrondesignteam` container packages
   public, or run `sudo docker login ghcr.io` with a token that can only read
   packages.
5. Create `/etc/biotron/infisical.env` as above, with `ENCRYPTION_KEY` from
   `openssl rand -hex 16`, `AUTH_SECRET` from `openssl rand -base64 32`, and
   `INFISICAL_DB_PASSWORD` from `openssl rand -hex 32`. Start Infisical with
   `sudo biotron-deploy secrets <commit>`. Its UI listens on the host's
   loopback only; reach it with `ssh -L 8090:127.0.0.1:8090 <host>` and open
   `http://localhost:8090`. Create the administrator account first.
6. In Infisical, create the project, the `staging` and `prod` environments,
   the `/deploy` folder in each, and the keys above. Create two machine
   identities with Universal Auth, `biotron-staging` and `biotron-prod`, and
   give each a project role that can only read secrets in its own environment.
   Write each one's client ID, client secret, and the project ID into its
   `identity.env`.
7. In Cloudflare Zero Trust, create a remotely managed tunnel and add every
   public hostname of both domains: the apex, `www`, `calendar`, `auth`,
   `exogui`, `status`, and `sprinter`. Every hostname uses the origin
   `http://nginx:8080`. Put the tunnel token and `EDGE_SITES` in
   `/etc/biotron/edge.env`.
8. Deploy: push to `main` and a release tag, or by hand:

   ```sh
   sudo biotron-deploy staging <main-commit>
   sudo biotron-deploy prod <release-commit>
   sudo biotron-deploy edge <release-commit>
   ```

   Each deploy waits until every container is running and healthy and every
   migration has finished. Otherwise it fails, naming each service that is
   not ready and its state, and writes their logs to
   `/var/log/biotron-deploy/<target>-<commit>-<time>.log`, readable by root
   only: the Actions logs of this public repository never show them.
9. Check every hostname of both domains through Cloudflare, and that only
   staging's responses carry `X-Robots-Tag`.

A fresh Postgres creates `sprinter_reader` on its first start but cannot grant
it Logger's and Auth's tables, which do not exist yet. After the first deploy
of an instance, and after a deploy that changes the script, run the grant
script once more:

```sh
cd / && sudo docker compose -p biotron-prod exec -T postgres bash < /opt/biotron/prod/infra/postgres/init/10-sprinter-reader.sh
```

## Copying an instance's data

Use this to seed a new instance from an existing database, or to move an
instance to another host. Writes must stop on the source while the copy runs,
because two copies must not both take writes.

1. **Deploy the target** with `sudo biotron-deploy <instance> <commit>`. It
   starts empty and migrated. Then stop its apps, keeping its Postgres
   running. Run these from a directory with no Compose file, such as `/`, so
   Compose goes by the project name alone:

   ```sh
   cd /
   sudo docker compose -p biotron-<instance> stop $(sudo docker compose -p biotron-<instance> ps --services | grep -vx postgres)
   ```

2. **Stop the source's apps** the same way. Downtime starts here.
3. **Dump the source** and, for another host, copy the file there over
   Tailscale:

   ```sh
   cd / && sudo docker compose -p biotron-<instance> exec -T postgres \
     pg_dump -U biotron -d biotron --format=custom --no-owner --no-privileges > ~/biotron.dump
   ```

4. **Restore it into the target**, grant the read-only role again, and deploy
   once more, which reapplies any newer migrations and starts the apps:

   ```sh
   cd /
   sudo docker compose -p biotron-<instance> exec -T postgres \
     pg_restore -U biotron -d biotron --clean --if-exists --no-owner --no-privileges < ~/biotron.dump
   sudo docker compose -p biotron-<instance> exec -T postgres bash < /opt/biotron/<instance>/infra/postgres/init/10-sprinter-reader.sh
   sudo biotron-deploy <instance> <commit>
   ```

5. **Switch traffic** if the instance moved hosts: move its hostnames to the
   new host's Cloudflare tunnel, update `EDGE_SITES` on both hosts, and set
   `DEPLOY_HOST` (and `DEPLOY_EDGE`) in its GitHub Environment.
6. **Verify** every hostname, a sign-in, and that only staging's responses
   carry `X-Robots-Tag`. Keep the dump and the old volumes for a week as a
   fallback, then `shred -u` the dump.

## Rotation

- **An Infisical value** (an API key, an OAuth secret, a session key):
  change it in Infisical, then redeploy that instance (rerun its last deploy
  workflow, or `sudo biotron-deploy <instance> $(cat /opt/biotron/<instance>/REVISION)`).
  Compose recreates the containers whose settings changed. Running
  containers keep the old value until then, including across restarts.
- **A Postgres password:** the password in Infisical only reaches new
  clusters. Change it in the database first, then in Infisical, then redeploy:

  ```sh
  sudo docker compose -p biotron-prod exec postgres psql -U biotron -d biotron \
    -c "ALTER ROLE biotron PASSWORD '<new>'"   # or sprinter_reader
  ```

- **A machine identity's client secret:** create a new client secret for the
  identity in Infisical, put it in that `identity.env` with `sudoedit`, check a
  deploy works, then revoke the old one.
- **The tunnel token:** rotate it in Cloudflare, put it in
  `/etc/biotron/edge.env`, then
  remove the cloudflared container so the next deploy creates it with the new
  token, a few seconds without the tunnel:
  `sudo docker rm -f biotron-edge-cloudflared-1 && sudo biotron-deploy edge $(cat /opt/biotron/edge/REVISION)`.
- **Infisical's own keys:** `AUTH_SECRET` and `INFISICAL_DB_PASSWORD` can be
  rotated like any password (the database one with `ALTER ROLE` in the
  `biotron-secrets` Postgres first). Never change `ENCRYPTION_KEY`: it
  encrypts everything stored. See `infra/infisical/README.md` for backups.

## Disk space

The images for one commit take about 4 GB, most of it the five `*-migrate`
images, and each instance can run a different commit. A deploy removes this
registry's images that no container uses any more; every commit's images stay
in GHCR for a rollback. Keep about 10 GB free on the filesystem that holds
`/var/lib/docker` for a host running both instances. If it sits on a small
partition, move Docker's data root (`"data-root"` in
`/etc/docker/daemon.json`, then restart Docker) or grow the partition.

An instance deployed before Redis was removed keeps its `redis-data` volume.
The first deploy without Redis removes the container; the volume holds
nothing anyone reads, so delete it by hand:

```sh
sudo docker volume rm biotron-<instance>_redis-data
```
