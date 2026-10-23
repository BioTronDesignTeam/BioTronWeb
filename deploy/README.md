# Deploying staging and production

Every push to `main` deploys to staging at `uwbiotron.dev`. Pushing a `v*`
tag, such as `v1.4.0`, on a `main` commit deploys that commit's images to
production at `uwbiotron.ca`. Until production gets its own Oracle VM, both
run side by side on one host, `haimekena`, and nothing either one does can
stop or change the other.

The host keeps no checkout and no `.env` files. Each deploy downloads the
commit's Compose files from GitHub, pulls that commit's images from GHCR, and
takes its settings from Infisical at the moment it starts the containers.
Only a handful of bootstrap credentials, which cannot live in Infisical, sit
on disk in root-only files under `/etc/biotron`.

## How it fits together

```text
Cloudflare ─ tunnel ─ cloudflared ─ Nginx ─┬─ biotron-staging network: staging apps, Postgres, Redis
              (biotron-edge project)       └─ biotron-prod network:    prod apps, Postgres, Redis
```

| Compose project | What | Settings come from |
| --- | --- | --- |
| `biotron-staging` | Staging's Postgres, Redis, and six apps, images tagged with the `main` commit | Infisical `staging`, through `/etc/biotron/staging/identity.env` |
| `biotron-prod` | Production's, images of the release tag's commit | Infisical `prod`, through `/etc/biotron/prod/identity.env` |
| `biotron-edge` | Nginx and cloudflared, shared by both | `/etc/biotron/edge.env` |
| `biotron-secrets` | Infisical with its own Postgres and Redis | `/etc/biotron/infisical.env` |

What keeps the two instances apart:

- **Networks.** Each instance has its own Docker network, `biotron-staging` or
  `biotron-prod`, with the same service aliases (`postgres`, `redis`,
  `oauth-manager`, `logger-api`) inside. A staging container cannot resolve or
  reach anything of production's. Only Nginx joins both, and it reaches the
  apps through aliases that carry the instance name, such as
  `prod-oauth-manager`.
- **Data.** Each instance runs its own Postgres and its own Redis on its own
  volumes (`biotron-staging_postgres-data` and so on), with different
  passwords. Separate instances, rather than two databases or two Redis
  indexes in one server, mean a staging migration, restart, upgrade,
  `FLUSHALL`, or full disk of logs cannot touch production, and no role exists
  that can read both. The cost is one more Postgres and Redis process, small
  next to the host's memory.
- **Settings.** Each instance has its own Infisical environment and its own
  machine identity, which can read nothing else. Domains, cookie domains,
  GitHub OAuth apps, and the Logger ingestion token all differ.
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
rebuilding: `biotron-deploy prod <commit>`. While both instances share a host,
a release also redeploys the shared edge from the same commit, so a change to
`infra/nginx` reaches staging's hostnames with the next release, not with the
merge. Staging deploys one commit at a time, so when several merges land
together only the newest may deploy; tag that one. A fix must reach
production through `main` and a new tag.

## Rolling back

Run the deploy workflow by hand from Actions and choose an older release tag.
It redeploys that tag's commit to production, Compose files and images both.
Database migrations may not be reversible; check what the newer release
migrated before rolling back across it.

From the host, the same thing is `sudo biotron-deploy prod <older-commit>`.
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
| `DEPLOY_HOST` | variable | The host's Tailscale name, `haimekena` for both today |
| `DEPLOY_EDGE` | variable | `true` in exactly one Environment per host: `production` today. Both once production has its own host |
| `TS_OAUTH_CLIENT_ID`, `TS_AUDIENCE` | secrets | Tailscale federated identity credentials |

No application secret lives in GitHub. Delete the old `SITE_ENV`,
`CALENDAR_ENV`, `LOGGER_ENV`, `AUTH_ENV`, `EXO_ENV`, `SPRINTER_ENV`,
`INFRA_ENV`, and `CLOUDFLARE_TUNNEL_TOKEN` secrets and the `DEPLOY_DOMAIN`
variable once the host runs from Infisical.

Add a tag ruleset for `v*` so only release managers can create, move or
delete release tags. A release tag is the production deploy button.

Configure the Tailscale federated identity with `auth_keys` write permission
and `tag:biotron-ci`; allow the tag to reach only the deployment hosts on port
22 and log in as `deploy`. No public SSH port is needed.

## What Infisical holds

One project, environments `staging` and `prod`, a folder `/deploy` in each.
Each environment needs:

| Key | What |
| --- | --- |
| `BASE_DOMAIN` | `uwbiotron.dev` for staging, `uwbiotron.ca` for production |
| `POSTGRES_PASSWORD` | The instance's Postgres owner password. `openssl rand -hex 32` |
| `SPRINTER_READER_PASSWORD` | The read-only `sprinter_reader` role's password. `openssl rand -hex 32` |
| `LOGGER_INGEST_TOKEN` | Shared by Logger and every service that logs to it |
| `AUTH_GITHUB_CLIENT_ID`, `AUTH_GITHUB_CLIENT_SECRET` | This instance's GitHub OAuth app |
| `AUTH_SUPERUSER_GITHUB_IDS` | Optional. Numeric GitHub IDs allowed to bootstrap administration |
| `SPRINTER_DISCORD_TOKEN`, `SPRINTER_DISCORD_GUILD_ID`, `SPRINTER_GEMINI_API_KEY` | Optional. Sprinter's bot and model |

`deploy/instance-defaults.sh` derives the rest from these: every app's
`DATABASE_URL`, Sprinter's read-only URL, and the domain-dependent URLs, CORS
lists, and cookie domain. Any compose input with an app prefix, such as
`AUTH_LOG_LEVEL` or `CALENDAR_DEFAULT_TIMEZONE`, can be set in Infisical too,
and a value set there always wins over a derived one. The Postgres passwords
go into URLs unescaped, so use hex.

Each GitHub OAuth app's callback is
`https://auth.<BASE_DOMAIN>/api/auth/github/callback`. Use one app per
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
2. Create the `deploy` account CI logs in as. It needs no Docker group, only
   permission to run the deploy script as root:

   ```sh
   sudo useradd --create-home --shell /bin/bash deploy
   echo 'deploy ALL=(root) NOPASSWD: /usr/local/sbin/biotron-deploy' |
     sudo install -o root -g root -m 0440 /dev/stdin /etc/sudoers.d/biotron-deploy
   sudo visudo -c
   ```

3. Install the deploy script from a `main` commit, root-owned so the `deploy`
   account cannot change it. Reinstall it the same way whenever
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
   migration has finished, and fails with the logs of what is not.
9. Check every hostname of both domains through Cloudflare, and that only
   staging's responses carry `X-Robots-Tag`.

A fresh Postgres creates `sprinter_reader` on its first start but cannot grant
it Auth's tables, which do not exist yet. After the first deploy of an
instance, run the grant script once more:

```sh
cd / && sudo docker compose -p biotron-prod exec -T postgres bash < /opt/biotron/prod/infra/postgres/init/10-sprinter-reader.sh
```

## Migrating haimekena from the checkout and `.env` files

Today `haimekena` runs one instance from a checkout of `main` at
`~/BioTronWeb`, with seven `.env` files, the Compose projects `biotron` (apps)
and `biotron-infrastructure` (Nginx, cloudflared, Postgres, Redis), images
built on the host, and the tunnel token in
`/etc/biotron/secrets/cloudflare-tunnel-token`. It serves `uwbiotron.dev`.

**Decide first which data is production's.** This plan treats today's data as
production's: it moves to `biotron-prod` at `uwbiotron.ca`, and staging, which
keeps `uwbiotron.dev`, gets a copy. People who use `uwbiotron.dev` today, and
calendar subscribers whose feed URLs name it, would then be looking at
staging. Either tell them about `uwbiotron.ca` first, or swap the two domains
in Infisical and `EDGE_SITES`; nothing else changes.

Everything up to the cutover runs beside the old stack without touching it:
new projects, networks, volumes, and container names never collide with the
old ones, and the new instances publish no host ports. Staging has no
downtime. Production needs a write freeze of a minute or two while its data is
copied, because two copies must not both take writes.

1. **Make room.** `docker system df` on the host shows about 4 GB of
   reclaimable build cache and only 2.7 GB free on `/var`. Run
   `sudo docker builder prune -af`, and preferably move Docker's data root to
   the root filesystem or grow `/var` (see "Disk space").
2. **Merge to `main`.** Only commits on `main` deploy. The first staging deploy
   job fails because the host is not ready; that changes nothing on the host.
   Rerun it after step 8.
3. **Install tools and the deploy account:** steps 1–4 of the fresh-host setup.
4. **Start Infisical** (step 5) and **fill it** (step 6). For production,
   take `LOGGER_INGEST_TOKEN` and the optional Sprinter keys from the current
   `apps/*/.env` files, and register a new GitHub OAuth app for
   `auth.uwbiotron.ca`. Staging keeps today's GitHub OAuth app, whose callback
   is already `auth.uwbiotron.dev`, and gets its own new `LOGGER_INGEST_TOKEN`.
   Generate fresh Postgres passwords for both; the new clusters start empty.
5. **Write `/etc/biotron/edge.env`** with the token from
   `/etc/biotron/secrets/cloudflare-tunnel-token` and
   `EDGE_SITES="staging=uwbiotron.dev:noindex prod=uwbiotron.ca"`. In
   Cloudflare, add the `uwbiotron.ca` hostnames to the existing tunnel, origin
   `http://nginx:8080`. Until the new edge runs, the old Nginx answers them
   with `404`.
6. **Deploy staging:** `sudo biotron-deploy staging <commit>`. It starts empty,
   migrated, and unreachable from outside, because the old edge still serves
   `uwbiotron.dev`.
7. **Deploy production the same way:** `sudo biotron-deploy prod <commit>`.
   Then stop its apps, keeping its Postgres and Redis running, and pull the
   edge image so the cutover does not wait for it. Run these from a directory
   with no Compose file, such as `/`, so Compose goes by the project name alone:

   ```sh
   cd /
   sudo docker compose -p biotron-prod stop $(sudo docker compose -p biotron-prod ps --services | grep -vx 'postgres\|redis')
   sudo docker pull ghcr.io/biotrondesignteam/edge-nginx:<commit>
   ```

8. **Cut over.** Downtime starts here.

   ```sh
   cd ~/BioTronWeb && docker compose stop && cd /         # old apps: no more writes
   docker exec biotron-infrastructure-postgres-1 \
     pg_dump -U biotron -d biotron --format=custom --no-owner --no-privileges > ~/biotron-cutover.dump
   sudo docker compose -p biotron-prod exec -T postgres \
     pg_restore -U biotron -d biotron --clean --if-exists --no-owner --no-privileges < ~/biotron-cutover.dump
   sudo docker compose -p biotron-prod exec -T postgres bash < /opt/biotron/prod/infra/postgres/init/10-sprinter-reader.sh
   sudo biotron-deploy prod <commit>                       # reapplies newer migrations, starts the apps
   docker stop biotron-infrastructure-cloudflared-1 biotron-infrastructure-nginx-1
   sudo biotron-deploy edge <commit>
   ```

   The restore brings back the old schema with its migration history, and the
   second deploy reruns each app's migrations, applying whatever the new
   commit adds. The new cloudflared connects with the same token, and
   `uwbiotron.dev` and `uwbiotron.ca` are served again, by staging and
   production. Redis is not copied: it holds caches, Logger's recent tails,
   and health state, which refill on their own.
9. **Give staging its copy** of the same dump, now or whenever staging needs
   fresh data:

   ```sh
   cd /
   sudo docker compose -p biotron-staging stop $(sudo docker compose -p biotron-staging ps --services | grep -vx 'postgres\|redis')
   sudo docker compose -p biotron-staging exec -T postgres \
     pg_restore -U biotron -d biotron --clean --if-exists --no-owner --no-privileges < ~/biotron-cutover.dump
   sudo docker compose -p biotron-staging exec -T postgres bash < /opt/biotron/staging/infra/postgres/init/10-sprinter-reader.sh
   sudo biotron-deploy staging <commit>
   ```

10. **Verify** every hostname of both domains, sign-in on both, and the
    `X-Robots-Tag` header on staging only. Rerun the failed staging deploy job
    from step 2 and check it passes.
11. **Remove the old layout** once satisfied. Keep the old Postgres volume and
    the dump for a week as a fallback:

    ```sh
    cd ~/BioTronWeb
    docker compose down                                   # old apps
    docker compose -f infra/docker-compose.yml --env-file infra/.env down   # keeps volumes
    docker network rm biotron biotron-edge 2>/dev/null || true
    docker image rm $(docker image ls --format '{{.Repository}}:{{.Tag}}' 'biotron-*')
    shred -u apps/*/.env infra/.env
    cd ~ && rm -rf ~/BioTronWeb
    sudo shred -u /etc/biotron/secrets/cloudflare-tunnel-token && sudo rmdir /etc/biotron/secrets
    # a week later:
    docker volume rm biotron-infrastructure_postgres-data biotron-infrastructure_redis-data
    shred -u ~/biotron-cutover.dump
    ```

To back out before step 11, stop the new edge and start the old stack again:

```sh
cd / && sudo docker compose -p biotron-edge stop
docker start biotron-infrastructure-nginx-1 biotron-infrastructure-cloudflared-1
cd ~/BioTronWeb && docker compose start
```

Writes made to production in between stay in `biotron-prod` and are not in
the old database.

## Rotation

- **An Infisical value** (an API key, an OAuth secret, the ingestion token):
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

## Moving production to the Oracle VM

Production's data and settings are already separate, so the move is a copy
and a switch of hosts:

1. Set up the VM as in "Setting up a host from scratch", with its own
   Infisical or by pointing `INFISICAL_API_URL` in its `prod/identity.env` at
   the existing one over Tailscale (then publish that Infisical privately, not
   on loopback). Create a new Cloudflare tunnel for production and set the
   VM's `EDGE_SITES="prod=uwbiotron.ca"`.
2. Deploy production on the VM, stop its apps, and copy the data with the
   cutover commands above, from `biotron-prod` on haimekena to the VM.
3. Move the `uwbiotron.ca` hostnames from haimekena's tunnel to the VM's,
   then stop `biotron-prod` on haimekena.
4. In GitHub, set `DEPLOY_HOST` in the `production` Environment to the VM and
   `DEPLOY_EDGE=true` in both Environments. On haimekena, set
   `EDGE_SITES="staging=uwbiotron.dev:noindex"` and redeploy its edge.
5. Once production on the VM has run for a while, remove `biotron-prod` and
   its volumes from haimekena.

## Disk space

The images for one commit take about 4 GB, most of it the five `*-migrate`
images, and each instance can run a different commit. A deploy removes this
registry's images that no container uses any more; every commit's images stay
in GHCR for a rollback. On haimekena, `/var`, which holds `/var/lib/docker`,
is 12 GB. Move Docker's data root to the larger root filesystem
(`"data-root"` in `/etc/docker/daemon.json`, then restart Docker) or grow
`/var` before running both instances.
