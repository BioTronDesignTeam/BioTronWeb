# Deploying staging and production

Every push to `main` deploys to staging at `uwbiotron.dev`. Pushing a `v*`
tag, such as `v1.4.0`, on a `main` commit deploys that commit's images to
production at `uwbiotron.ca`.

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

## Where each instance runs

Each instance has a host of its own, and each host runs everything that
instance needs: its own Infisical, its own edge (Nginx and cloudflared), and
its own Cloudflare tunnel. Nothing is shared between the hosts, so a staging
host that is down, compromised, or being rebuilt cannot reach or stop
production.

| Host | Instance | Domain | Runs |
| --- | --- | --- | --- |
| The Oracle Cloud Arm VM | `prod` | `uwbiotron.ca` | `biotron-prod`, `biotron-edge` with the production tunnel, `biotron-secrets` (production's Infisical) |
| `haimekena` | `staging` | `uwbiotron.dev` | `biotron-staging`, `biotron-edge` with the staging tunnel, `biotron-secrets` (staging's Infisical) |

```text
prod host:     Cloudflare ─ prod tunnel ─ cloudflared ─ Nginx ─ biotron-prod network: apps, Postgres
                                                        Infisical on 127.0.0.1:8090
staging host:  Cloudflare ─ staging tunnel ─ cloudflared ─ Nginx ─ biotron-staging network: apps, Postgres
                                                        Infisical on 127.0.0.1:8090
```

| Compose project | What | Settings come from |
| --- | --- | --- |
| `biotron-<instance>` | The instance's Postgres and five apps, images tagged with a `main` commit | Its host's Infisical, environment `<instance>`, through `/etc/biotron/<instance>/identity.env` |
| `biotron-edge` | Nginx and cloudflared | `/etc/biotron/edge.env` |
| `biotron-secrets` | Infisical with its own Postgres and Redis | `/etc/biotron/infisical.env` |

A deploy reaches Infisical on its own host's loopback,
`http://127.0.0.1:8090/api`, the default `INFISICAL_API_URL`. Each Infisical
holds one environment, its own host's instance, so a stolen staging
identity, or the staging Infisical itself, holds nothing of production's.

The scripts still keep two instances apart on one host, as a laptop or a
temporary migration may run them:

- **Networks.** Each instance has its own Docker network, `biotron-<instance>`,
  with a fixed subnet (`10.213.<n>.0/24`; `prod` is 2 and `staging` 1), and
  the same service aliases (`postgres`, `oauth-manager`, `calendar-api`)
  inside. A container cannot resolve or reach another instance's. Only Nginx
  joins every instance's network, always at address `.2`, and it reaches the
  apps through aliases that carry the instance name, such as
  `prod-oauth-manager`.
- **Data.** Each instance runs its own Postgres on its own volume
  (`biotron-<instance>_postgres-data`), with different passwords.
- **Names and ports.** Container names come from the project, so they never
  collide, and a named instance publishes no host ports at all
  (`infra/no-host-ports.yml`). Use `docker compose -p biotron-<instance> exec`
  for administration, from a directory with no Compose file, such as `/`, so
  Compose goes by the project name alone.
- **Lifecycles.** Deploying, stopping, or breaking one project leaves the others
  running. Nginx resolves each app per request, so a stopped instance answers
  `502` without affecting the other's hostnames.

On the host, `/opt/biotron/<instance|edge|secrets>` holds the Compose files
of the commit last deployed there, root-owned and free of secrets, and a
`REVISION` file. The previous deploy's files stay beside it as `.previous`.

## Database roles

Each app connects to its instance's Postgres as a role of its own, with rights
to read and write the rows of its own schema and nothing else:

| Role | Schema | Password key |
| --- | --- | --- |
| `auth_app` | `oauth` | `AUTH_DB_PASSWORD` |
| `calendar_app` | `calendar` | `CALENDAR_DB_PASSWORD` |
| `sprinter_app` | `sprinter` | `SPRINTER_DB_PASSWORD` |
| `sprinter_reader` | SELECT on four `oauth` tables | `SPRINTER_READER_PASSWORD` |

None of them is a superuser, owns a table, or can change a schema, so a flaw
in one app cannot reach another app's data or run programs in the database
container (`COPY ... TO PROGRAM`). The cluster owner, `POSTGRES_USER`
(`biotron`), owns every schema and runs the migrations only.

On every deploy, before any migration, the `postgres-roles` one-shot service
runs `infra/postgres/init/*.sh` as the owner. It creates any role that is
missing, sets each password from Infisical, and grants each app its schema
and the default privileges that cover every table a migration adds. After each
app's migrations, the migrate image runs `infra/migrate/grants/<app>.sql`,
which grants Sprinter's read-only role its tables again. No step is manual,
including after a restore.

## Releasing

A push to `main` builds and scans all 13 images, for amd64 and arm64, each on
a native runner of its own architecture. It pushes them as
`ghcr.io/biotrondesignteam/<service>:<full-commit-SHA>`, and runs
`biotron-deploy staging <commit>` on the staging host over Tailscale SSH.
An image that a commit already has is never built or pushed again: rerunning
the workflow for a commit reuses its images, so production always gets
exactly what staging ran.

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
`biotron-deploy prod` refuses any other commit, whoever asks. Each deploy
also redeploys its host's edge from the same commit, so a change to
`infra/nginx` reaches staging with the merge and production with the release.
Staging deploys one commit at a time, so when several merges land together
only the newest may deploy; tag that one. A fix must reach production through
`main` and a new tag.

Each deploy waits until every container is ready: each one-shot (the
migrations and `postgres-roles`) has exited 0, and every other container is
running, healthy where it has a health check, and has stayed up without a
restart for 20 seconds. A failed migration or a crash loop fails the deploy at
once, naming each service that is not ready and its state; so does a Compose
start that fails outright. Their logs go to
`/var/log/biotron-deploy/<target>-<commit>-<time>.log`, readable by root
only: the Actions logs of this public repository never show them.

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

## Edge headers

The edge enforces a Content-Security-Policy on every app
(`infra/nginx/sites/csp.template`). Browsers report each violation to
`/_edge/csp-report`, and the edge logs it as a `"type":"csp-report"` line of
its access log, with the directive and the blocked and page addresses cut at
their query strings: `sudo docker compose -p biotron-edge logs nginx | grep
csp-report`. HSTS is `max-age=300` for each hostname alone, with no
`includeSubDomains` and no preload, until both domains have run on it for a
while.

## GitHub setup

Create the `staging` and `production` Environments in repository Settings >
Environments. Restrict `staging` to the `main` branch and `production` to `v*`
tags. A production reviewer is optional. In each, set:

| Name | Kind | Value |
| --- | --- | --- |
| `DEPLOY_HOST` | variable | The Tailscale name of the host that runs this instance: `haimekena` for staging, the Oracle VM for production |
| `DEPLOY_EDGE` | variable | `true` in both: each host runs its own edge |
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
reach only its own host on port 22 and log in only as its own account:

```json
"ssh": [
  { "action": "accept", "src": ["tag:biotron-ci-staging"], "dst": ["tag:biotron-staging"], "users": ["deploy-staging"] },
  { "action": "accept", "src": ["tag:biotron-ci-prod"], "dst": ["tag:biotron-prod"], "users": ["deploy-prod"] }
]
```

`tag:biotron-staging` and `tag:biotron-prod` stand for however the policy
names the two hosts. No public SSH port is needed.

The accounts themselves can do one thing only. Their login shell is
`deploy/biotron-deploy-shell`, which runs a single command,
`sudo /usr/local/sbin/biotron-deploy <target> <commit>`, and refuses an
interactive login or any other command, so a leaked CI identity gets a deploy
of a `main` commit and no shell. Tailscale SSH runs every remote command
through that login shell; there is no `authorized_keys` to put a forced
command in. Tailscale SSH can still forward a port for an account it lets in,
which the shell cannot see; on a host of one instance that reaches nothing
beyond what that instance's CI identity could deploy anyway, but keep the
tailnet policy above narrow.

## What Infisical holds

On each host, one project with one environment, the host's instance (`staging`
or `prod`), and a folder `/deploy` in it. It needs:

| Key | What |
| --- | --- |
| `BASE_DOMAIN` | `uwbiotron.dev` for staging, `uwbiotron.ca` for production |
| `POSTGRES_PASSWORD` | The instance's Postgres owner password, which migrations use. `openssl rand -hex 32` |
| `AUTH_DB_PASSWORD`, `CALENDAR_DB_PASSWORD`, `SPRINTER_DB_PASSWORD` | Each app's own role (see "Database roles"). `openssl rand -hex 32` each |
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
`DATABASE_URL`, as its own role, and the owner's `*_MIGRATE_DATABASE_URL` for
its migrations, Sprinter's read-only URL, and the domain-dependent URLs, CORS
lists, and cookie domain. Any compose input with an app prefix, such as
`AUTH_LOG_LEVEL` or `CALENDAR_DEFAULT_TIMEZONE`, can be set in Infisical too,
and a value set there always wins over a derived one. The Postgres passwords
go into URLs unescaped, so use hex; a deploy refuses any other character.

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
sudo install -d -o root -g root -m 0700 /etc/biotron /etc/biotron/<instance>
sudo install -o root -g root -m 0600 /dev/null /etc/biotron/edge.env
sudoedit /etc/biotron/edge.env
```

| File | Keys |
| --- | --- |
| `/etc/biotron/infisical.env` | `ENCRYPTION_KEY`, `AUTH_SECRET`, `INFISICAL_DB_PASSWORD`, optionally `INFISICAL_SITE_URL`, `INFISICAL_PORT`, `INFISICAL_IMAGE`. See `infra/infisical/.env.example` |
| `/etc/biotron/edge.env` | `CLOUDFLARE_TUNNEL_TOKEN` of this host's tunnel, and `EDGE_SITES`: `staging=uwbiotron.dev:noindex` on the staging host, `prod=uwbiotron.ca` on the production host. Optionally `EDGE_SUBNET` (default `10.213.255.0/29`), the edge's own network, if that range is taken on the host |
| `/etc/biotron/<instance>/identity.env` | `INFISICAL_CLIENT_ID`, `INFISICAL_CLIENT_SECRET`, `INFISICAL_PROJECT_ID` of the instance's machine identity |

An identity file may also set `INFISICAL_ENVIRONMENT` (default: the instance
name), `INFISICAL_PATH` (default `/deploy`), and `INFISICAL_API_URL` (default
`http://127.0.0.1:8090/api`, the host's own Infisical). Back up
`infisical.env` somewhere safe off the host: without its `ENCRYPTION_KEY`
nothing in that Infisical can be read again.

## Setting up a host from scratch

Do this once on each host, for its one instance (`<instance>` is `staging` or
`prod`). On `haimekena`, which already runs the old layout, follow
"Cutting haimekena over" below instead of step 8.

1. Install Docker Engine with the Compose plugin, `curl`, `jq`, `tar`,
   Tailscale with Tailscale SSH, and the
   [Infisical CLI](https://infisical.com/docs/cli/overview). Check that the
   filesystem holding `/var/lib/docker` has room for two commits' images,
   about 5 GB free; see "Disk space" below.
2. Install the deploy script and the CI account's login shell from a `main`
   commit, root-owned so the CI account cannot change them. Reinstall them the
   same way whenever `deploy/biotron-deploy` or `deploy/biotron-deploy-shell`
   changes; a deploy prints a warning when its copy on the host differs from
   the commit it deploys.

   ```sh
   for file in biotron-deploy biotron-deploy-shell; do
     curl -fsSL "https://raw.githubusercontent.com/BioTronDesignTeam/BioTronWeb/<commit>/deploy/$file" |
       sudo install -o root -g root -m 0755 /dev/stdin "/usr/local/sbin/$file"
   done
   grep -qx /usr/local/sbin/biotron-deploy-shell /etc/shells ||
     echo /usr/local/sbin/biotron-deploy-shell | sudo tee -a /etc/shells
   ```

3. Create the account CI logs in as: `deploy-staging` on the staging host,
   `deploy-prod` on the production host. It does not need the Docker group.
   It may run the deploy script as root for its own instance and its host's
   edge, and nothing else. Infisical itself (`secrets`) is deployed by an
   administrator by hand, never by CI. The script refuses anything but
   exactly a target and a 40-character commit, so the trailing `*` cannot
   smuggle in a second target.

   ```sh
   instance=staging   # or prod
   sudo useradd --create-home --shell /usr/local/sbin/biotron-deploy-shell "deploy-$instance"
   printf '%s\n' \
     "deploy-$instance ALL=(root) NOPASSWD: /usr/local/sbin/biotron-deploy $instance *, /usr/local/sbin/biotron-deploy edge *" |
     sudo install -o root -g root -m 0440 /dev/stdin /etc/sudoers.d/biotron-deploy
   sudo visudo -c
   ```

   An account made earlier with `--shell /bin/bash`, or the old single
   `deploy` account: `sudo usermod --shell /usr/local/sbin/biotron-deploy-shell deploy-<instance>`,
   replace `/etc/sudoers.d/biotron-deploy` as above, and `sudo userdel -r deploy`.
   Check it: `sudo -u deploy-<instance> -s` must refuse, and so must
   `tailscale ssh deploy-<instance>@<host> id` from a CI-tagged node.

4. Let root pull from GHCR: make the `biotrondesignteam` container packages
   public, or run `sudo docker login ghcr.io` with a token that can only read
   packages.
5. Create `/etc/biotron/infisical.env` as above, with `ENCRYPTION_KEY` from
   `openssl rand -hex 16`, `AUTH_SECRET` from `openssl rand -base64 32`, and
   `INFISICAL_DB_PASSWORD` from `openssl rand -hex 32`. Start this host's
   Infisical with `sudo biotron-deploy secrets <commit>`. Its UI listens on the
   host's loopback only; reach it with `ssh -L 8090:127.0.0.1:8090 <host>` and
   open `http://localhost:8090`. Create the administrator account first.
6. In this Infisical, create the project, the `<instance>` environment, the
   `/deploy` folder in it, and the keys above. Create a machine identity with
   Universal Auth, `biotron-<instance>`, and give it a project role that can
   only read secrets in that environment. Write its client ID, client secret,
   and the project ID into `/etc/biotron/<instance>/identity.env`.
7. In Cloudflare Zero Trust, create a remotely managed tunnel for this host
   and add the public hostnames of its domain: the apex, `www`, `calendar`,
   `auth`, `exogui`, and `sprinter`. Every hostname uses the origin
   `http://nginx:8080`. `status` is not one: it is a Cloudflare redirect to
   the public status page, set up outside this repository. Put the tunnel
   token and `EDGE_SITES` in `/etc/biotron/edge.env`.
8. Deploy: push to `main` (staging) or a release tag (production), or by
   hand:

   ```sh
   sudo biotron-deploy <instance> <commit>
   sudo biotron-deploy edge <commit>
   ```

9. Check every hostname through Cloudflare, that only staging's responses
   carry `X-Robots-Tag`, and that `curl -sI https://<domain>` shows
   `strict-transport-security: max-age=300` and the
   `content-security-policy`.

## Cutting haimekena over

`haimekena` still runs the layout of the old `main`, from a checkout in
`~levon/BioTronWeb`: the `biotron-infrastructure` Compose project (Nginx,
cloudflared, Postgres, and Redis) and the `biotron` project with the apps
(`docker compose ls` shows both; seen in November 2026). Nothing in the new
scripts stops them, and the old cloudflared is a connector of a tunnel that
the new edge would join too. Retire them in this order, so the old and new
stacks never serve at once.

1. **Set the host up** with steps 1 to 7 above, for `staging`. Use a new
   Cloudflare tunnel for the new edge, not the old tunnel's token: two
   connectors on one tunnel would split requests between the old and new
   Nginx at random.
2. **Deploy staging** with `sudo biotron-deploy staging <main-commit>`. It
   starts empty and migrated, beside the old stack, with no host ports and
   no tunnel yet. Then stop its apps, keeping its Postgres running:

   ```sh
   cd /
   sudo docker compose -p biotron-staging stop $(sudo docker compose -p biotron-staging ps --services | grep -vx postgres)
   ```

3. **Stop writes on the old stack** by stopping its apps. Downtime starts
   here.

   ```sh
   cd / && sudo docker compose -p biotron stop
   ```

4. **Dump the old database**, from the old project:

   ```sh
   cd / && sudo docker compose -p biotron-infrastructure exec -T postgres \
     pg_dump -U biotron -d biotron --format=custom --no-owner --no-privileges > ~/biotron.dump
   ```

5. **Restore it into staging**, then deploy once more, which reapplies newer
   migrations, the roles, and the grants, and starts the apps:

   ```sh
   cd / && sudo docker compose -p biotron-staging exec -T postgres \
     pg_restore -U biotron -d biotron --clean --if-exists --no-owner --no-privileges \
       --exclude-schema=logger < ~/biotron.dump
   sudo biotron-deploy staging <main-commit>
   ```

   `--exclude-schema=logger` leaves out the retired Status app's schema. An
   instance restored without it drops it, and its role, once:
   `sudo docker compose -p biotron-<instance> exec -T postgres psql -U biotron -d biotron -c 'DROP SCHEMA IF EXISTS logger CASCADE; DROP ROLE IF EXISTS status_app'`.

6. **Retire the old stack**, keeping its volumes for now: no `-v`.

   ```sh
   cd /
   sudo docker compose -p biotron down
   sudo docker compose -p biotron-infrastructure down
   ```

   The old checkout in `~levon/BioTronWeb` is then unused; remove it once the
   week below has passed.

   In Cloudflare, delete the old tunnel's connector (or the old tunnel), and
   move the `uwbiotron.dev` hostnames to the new tunnel.
7. **Start the edge** with the new tunnel's token: `sudo biotron-deploy edge <main-commit>`.
8. **Verify** every staging hostname and a sign-in. Keep the dump and the old
   volumes for a week as a fallback, then `shred -u ~/biotron.dump` and
   remove the old volumes (`sudo docker volume ls`).

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

4. **Restore it into the target** and deploy once more, which reapplies any
   newer migrations, the roles, and every grant, and starts the apps:

   ```sh
   cd /
   sudo docker compose -p biotron-<instance> exec -T postgres \
     pg_restore -U biotron -d biotron --clean --if-exists --no-owner --no-privileges \
       --exclude-schema=logger < ~/biotron.dump
   sudo biotron-deploy <instance> <commit>
   ```

5. **Switch traffic** if the instance moved hosts: move its hostnames to the
   new host's Cloudflare tunnel, and set `DEPLOY_HOST` in its GitHub
   Environment.
6. **Verify** every hostname, a sign-in, and that only staging's responses
   carry `X-Robots-Tag`. Keep the dump and the old volumes for a week as a
   fallback, then `shred -u` the dump.

## Rotation

- **An Infisical value** (an API key, an OAuth secret, a session key):
  change it in Infisical, then redeploy that instance with
  `sudo biotron-deploy <instance> $(cat /opt/biotron/<instance>/REVISION)`.
  Rerunning its last deploy workflow does the same and rebuilds nothing.
  Compose recreates the containers whose settings changed. Running
  containers keep the old value until then, including across restarts.
- **An app role's password, or `sprinter_reader`'s:** change it in
  Infisical and redeploy as above. `postgres-roles` sets the new password
  before the apps restart with it.
- **The owner's password (`POSTGRES_PASSWORD`):** Infisical's value only
  reaches new clusters, and `postgres-roles` signs in with it. Change it in
  the database first, then in Infisical, then redeploy:

  ```sh
  cd / && sudo docker compose -p biotron-prod exec postgres psql -U biotron -d biotron \
    -c "ALTER ROLE biotron PASSWORD '<new>'"
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

The images for one commit take a few GB, and a rollback or a deploy keeps the
previous commit's until the next prune. A deploy removes this registry's
images that no container uses any more; every commit's images stay in GHCR
for a rollback. Keep about 5 GB free on the filesystem that holds
`/var/lib/docker`. If it sits on a small partition, move Docker's data root
(`"data-root"` in `/etc/docker/daemon.json`, then restart Docker) or grow the
partition.

An instance deployed before Redis was removed keeps its `redis-data` volume.
The first deploy without Redis removes the container; the volume holds
nothing anyone reads, so delete it by hand:

```sh
sudo docker volume rm biotron-<instance>_redis-data
```
