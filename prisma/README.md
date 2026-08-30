# prisma

Owns the Postgres schema (`schema.prisma`) and migrations for OAuthManager.

Prisma is **migrations-only** — the Go runtime queries via pgx
(`../backend/internal/store`). The `prisma-client-js` generator is a placeholder
(official Prisma Go client was sunset).

## Usage

```bash
# From the repository root after copying .env.example to .env:
docker compose run --rm migrate npm run migrate
```

Compose passes the repository-root `DATABASE_URL` to Prisma. Do not create a
second environment file in this directory.
