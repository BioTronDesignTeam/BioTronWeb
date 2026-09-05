# syntax=docker/dockerfile:1

FROM golang:1.27-bookworm AS go

FROM node:24-bookworm-slim

COPY --from=go /usr/local/go /usr/local/go

ENV PATH="/usr/local/go/bin:${PATH}"

RUN ln -s /usr/local/go/bin/go /usr/local/bin/go \
    && ln -s /usr/local/go/bin/gofmt /usr/local/bin/gofmt

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        git \
        openssh-client \
        openssl \
        postgresql-client \
    && rm -rf /var/lib/apt/lists/*

# The compose file masks every node_modules with a named volume. A volume
# takes the ownership of the path it covers on first use, so those paths are
# created here owned by node; otherwise they would be root's and npm could
# not write to them.
RUN mkdir -p /workspaces/biotron/apps/auth/frontend/node_modules /workspaces/biotron/apps/auth/prisma/node_modules /workspaces/biotron/apps/calendar/frontend/node_modules /workspaces/biotron/apps/calendar/prisma/node_modules /workspaces/biotron/apps/exo/frontend/node_modules /workspaces/biotron/apps/exo/prisma/node_modules /workspaces/biotron/apps/logger/frontend/node_modules /workspaces/biotron/apps/logger/prisma/node_modules /workspaces/biotron/apps/site/frontend/node_modules /workspaces/biotron/apps/sprinter/frontend/node_modules /workspaces/biotron/apps/sprinter/prisma/node_modules /workspaces/biotron/node_modules /workspaces/biotron/packages/style/node_modules \
    && chown -R node:node /workspaces

USER node
