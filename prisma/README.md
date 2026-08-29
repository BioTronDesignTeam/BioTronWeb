# prisma

Owns the Postgres schema (`schema.prisma`) and migrations for OAuthManager.

Prisma is **migrations-only** — the Go runtime queries via pgx
(`../backend/internal/store`). The `prisma-client-js` generator is a placeholder
(official Prisma Go client was sunset).

## Usage

```bash
# from repo root, with docker compose up
cp .env.example .env   # if needed
npm install
npm run migrate        # prisma migrate dev
npm run studio         # browse the DB
```

`DATABASE_URL` defaults to the local compose Postgres:
`postgresql://oauth:oauth@127.0.0.1:5434/oauth?schema=public`
