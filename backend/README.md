# backend

Go Fiber service: GitHub OAuth, session cookies, apps/grants/access-request APIs.
Postgres via pgx; Prisma owns schema (`../prisma`). Redis caches sessions + grants.

## Routes

| Method | Path | Notes |
|--------|------|-------|
| GET | `/health` | liveness |
| GET | `/auth/github/login` | start OAuth; optional `?redirect=<allowed origin>` |
| GET | `/auth/github/callback` | finish OAuth |
| POST | `/auth/guest` | product daily-key login; requires `app_id` + `key` (rate limited) |
| POST | `/auth/logout` | session + XHR |
| GET | `/auth/me` | current operator |
| GET | `/auth/guest-keys` | today's product keys (manager/superuser) |
| GET | `/apps` | app catalog |
| POST | `/apps` | superuser create app |
| GET | `/me/grants` | my grants |
| GET | `/me/requests` | my requests |
| POST | `/requests` | request access |
| GET | `/requests/pending` | approver queue |
| POST | `/requests/:id/approve` | approve → grant |
| POST | `/requests/:id/deny` | deny |
| POST/DELETE | `/grants` | direct grant (approvers) |
| GET | `/v1/check?app=&action=` | allow/deny check |

Mutating routes require `X-Requested-With` (CSRF).
