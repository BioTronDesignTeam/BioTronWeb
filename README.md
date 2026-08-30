# Server

Oracle VM ops for BioTron containers: systemd units, the daily ~3am reset,
and cold-boot bring-up.

## Layout

| Path | What |
|------|------|
| `docker-compose.yml` | The one shared Postgres instance and one shared Redis instance |
| `.env.example` | Local names, credentials, and bound ports for shared infrastructure |
| `services/` | systemd units and watchdog config |

## Shared infrastructure

Copy `.env.example` to `.env`, replace the example Postgres password, then
start the shared data services:

```bash
docker compose up -d
```

Application repositories join the Compose network named `biotron`. Their
Prisma URLs select an application-owned Postgres schema; they do not start
their own Postgres or Redis containers.
