# syntax=docker/dockerfile:1

# One image applies the Prisma migrations of every app, each to its own
# schema: each app's *-migrate service runs it with the app's name and its own
# DATABASE_URL. Build it from the repository root. Dockerfile.dockerignore
# beside this file keeps the context to this folder and every
# apps/*/prisma schema and migrations, so a new app's are picked up as is.
FROM node:24-bookworm-slim

# Prisma reads the OpenSSL version to pick its engines.
RUN apt-get update \
    && apt-get install -y --no-install-recommends openssl \
    && rm -rf /var/lib/apt/lists/*

# The same layout as the repository, which the migrate script expects.
WORKDIR /app
COPY infra/migrate/package.json infra/migrate/package-lock.json infra/migrate/
RUN npm ci --prefix infra/migrate --no-audit --no-fund \
    && npm cache clean --force
COPY infra/migrate/migrate infra/migrate/
COPY apps apps

# Applying migrations needs no root; the image's built-in node user does it.
USER node

ENTRYPOINT ["/app/infra/migrate/migrate"]
