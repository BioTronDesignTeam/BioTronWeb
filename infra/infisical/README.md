# Infisical

Infisical holds every deployed instance's settings and secrets. This stack is
the `biotron-secrets` Compose project: Infisical with its own Postgres, Redis,
network, and volumes. It shares nothing with the instances, and
`./launch all` and `./launch stop` never touch it. One Infisical serves every
instance on a host, with one environment and one machine identity per
instance. What goes in it, and how a deploy reads it, is in
`deploy/README.md`.

## Bootstrap

Infisical's own keys cannot live in Infisical. They go in
`/etc/biotron/infisical.env`, owned by root, mode `0600`, outside any
checkout. Start from `.env.example` beside this file:

```bash
sudo install -d -o root -g root -m 0700 /etc/biotron
sudo install -o root -g root -m 0600 /dev/null /etc/biotron/infisical.env
sudoedit /etc/biotron/infisical.env
```

Fill `ENCRYPTION_KEY` using `openssl rand -hex 16`, `AUTH_SECRET` using
`openssl rand -base64 32`, and `INFISICAL_DB_PASSWORD` using
`openssl rand -hex 32`. Use a hex database password so the connection URI
needs no percent encoding. Back the file up securely off the host, especially
`ENCRYPTION_KEY`: without it nothing stored can be decrypted.

On a host, start or update the stack from a `main` commit:

```bash
sudo biotron-deploy secrets <commit>
curl --fail http://127.0.0.1:8090/api/status
```

From a checkout, `sudo ./launch secrets` does the same with the files at hand;
`INFISICAL_BOOTSTRAP_FILE` selects a different file.

The UI binds to loopback only. For administration, use a Tailscale SSH
forward, for example `ssh -L 8090:127.0.0.1:8090 admin@host`, then open
`http://localhost:8090`. Create the administrator before granting access to
others. For shared use, configure private HTTPS ingress, set
`INFISICAL_SITE_URL` to that address, and recreate the Infisical container.
There is no public route or Cloudflare Tunnel to it.

## Project layout

Create a BioTronWeb project, environments `staging` and `prod`, and a folder
`/deploy` in each. Create one machine identity per instance,
`biotron-staging` and `biotron-prod`, with Universal Auth, and give each a
project role that can only read secrets in its own environment. Each
identity's client ID and secret go in `/etc/biotron/<instance>/identity.env`
on the host that runs that instance, never in Git or Infisical.

All app Compose inputs support `APP_VARIABLE`, such as `AUTH_FRONTEND_URL` or
`LOGGER_HEALTH_INTERVAL`. These names are translated to the existing variable
names inside containers. Unprefixed values remain a local development
fallback; **do not put generic `DATABASE_URL`, `WEB_PORT`, `API_PORT`,
`FRONTEND_URL`, or `VITE_*` keys in Infisical**. `VITE_*` settings are build
time inputs and have no effect on a deployed image. App identities are not
isolated from each other inside an instance: one identity retrieves that
instance's settings for all six apps.

Secrets are fetched at deploy time and handed to Compose in its environment;
nothing is written to disk. Running containers keep their values through an
ordinary restart, including a host reboot. Changing a value takes effect at
the next deploy of that instance. Docker administrators can inspect container
environments.

## Maintenance and recovery

```bash
umask 077
sudo docker compose -p biotron-secrets ps
sudo docker compose -p biotron-secrets exec -T db \
  pg_dump -U infisical infisical > infisical-backup.sql
```

Protect the dump with restrictive permissions and encryption. Back up the
bootstrap file separately. Test restoration into a separate stack using the
same image version and encryption key before upgrading. Do not delete
`biotron-secrets_postgres-data`, use `down -v`, regenerate the encryption key,
or point this stack at an instance's Postgres volume. Redis's volume stores
queues and cache. Update `INFISICAL_IMAGE` deliberately after reviewing
release notes and taking a backup. The bootstrap example pins an initial
version; it is not an automatic upgrade policy.

Official references: [Compose deployment](https://infisical.com/docs/self-hosting/deployment-options/docker-compose)
and [CLI authentication](https://infisical.com/docs/cli/usage).

Configuration regression checks (no daemon needed):

```bash
python3 infra/tests/test-runtime-config.py
```
