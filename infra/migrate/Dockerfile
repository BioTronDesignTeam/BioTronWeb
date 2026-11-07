# syntax=docker/dockerfile:1

# One image applies the Prisma migrations of every app, each to its own
# schema: each app's *-migrate service runs it with the app's name and its own
# DATABASE_URL, then makes its grants (grants/<app>.sql). Build it from the
# repository root. Dockerfile.dockerignore beside this file keeps the context
# to this folder and every
# apps/*/prisma schema and migrations, so a new app's are picked up as is.

# npm installs the Prisma CLI and its engines here; the runtime below gets
# only the result.
FROM node:24.22.2-bookworm-slim@sha256:01d566e8d94d8c619591d78652babfccd3c6992a3dffa9ed5ab39e11cf6c70e9 AS install

# Prisma reads the OpenSSL version to pick its engines.
RUN apt-get update     && apt-get install -y --no-install-recommends openssl     && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY infra/migrate/package.json infra/migrate/package-lock.json infra/migrate/
RUN npm ci --prefix infra/migrate --no-audit --no-fund

FROM node:24.22.2-bookworm-slim@sha256:01d566e8d94d8c619591d78652babfccd3c6992a3dffa9ed5ab39e11cf6c70e9

# The OS packages get their security fixes, and Prisma gets OpenSSL. Prisma
# runs on node alone, so the runtime drops the npm, npx, and Corepack that the
# node image bundles: nothing here installs a package.
RUN apt-get update     && apt-get upgrade -y     && apt-get install -y --no-install-recommends openssl     && rm -rf /var/lib/apt/lists/*     && rm -rf /usr/local/lib/node_modules /usr/local/bin/npm /usr/local/bin/npx /usr/local/bin/corepack

# The same layout as the repository, which the migrate script expects.
WORKDIR /app
COPY --from=install /app/infra/migrate/node_modules infra/migrate/node_modules
COPY infra/migrate/package.json infra/migrate/migrate infra/migrate/
COPY infra/migrate/grants infra/migrate/grants
COPY apps apps

# Applying migrations needs no root; the image's built-in node user does it.
USER node

ENTRYPOINT ["/app/infra/migrate/migrate"]
