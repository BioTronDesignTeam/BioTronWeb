# syntax=docker/dockerfile:1

FROM golang:1.27-bookworm AS go

# Copy official native binaries so ./pr-precheck needs no Docker daemon.
# These are multi-architecture index digests for both amd64 and arm64.
# golangci-lint 2.13.2 is built with Go 1.27; Trivy 0.70.0 matches the
# default installed by trivy-action@v0.36.0 in .github/workflows/ci.yml.
FROM golangci/golangci-lint:v2.13.2@sha256:ba07dffad130794ae79ebaa0056809d18c0168f3f846480ffd3eb6c04578b83d AS golangci
FROM trufflesecurity/trufflehog:3.97.4@sha256:562bc231afa9de3d04de44cfe624252b08207de1fc3cebc5e7ed92bed7f279e4 AS trufflehog
FROM aquasec/trivy:0.70.0@sha256:be1190afcb28352bfddc4ddeb71470835d16462af68d310f9f4bca710961a41e AS trivy

FROM node:24-bookworm-slim

COPY --from=go /usr/local/go /usr/local/go
COPY --from=golangci /usr/bin/golangci-lint /usr/local/bin/golangci-lint
COPY --from=trufflehog /usr/bin/trufflehog /usr/local/bin/trufflehog
COPY --from=trivy /usr/local/bin/trivy /usr/local/bin/trivy
COPY --from=trivy /contrib /contrib

ENV PATH="/usr/local/go/bin:${PATH}"

RUN ln -s /usr/local/go/bin/go /usr/local/bin/go \
    && ln -s /usr/local/go/bin/gofmt /usr/local/bin/gofmt

# Match TruffleHog's archive helpers (ar, cpio, rpm2cpio) as well as Git.
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        bash \
        binutils \
        ca-certificates \
        cpio \
        curl \
        git \
        jq \
        openssh-client \
        openssl \
        postgresql-client \
        rpm2cpio \
    && rm -rf /var/lib/apt/lists/*

# The compose file masks every node_modules with a named volume. A volume
# takes the ownership of the path it covers on first use, so those paths are
# created here owned by node; otherwise they would be root's and npm could
# not write to them.
RUN mkdir -p /workspaces/biotron/apps/auth/frontend/node_modules /workspaces/biotron/apps/auth/prisma/node_modules /workspaces/biotron/apps/calendar/frontend/node_modules /workspaces/biotron/apps/calendar/prisma/node_modules /workspaces/biotron/apps/exo/frontend/node_modules /workspaces/biotron/apps/exo/prisma/node_modules /workspaces/biotron/apps/logger/frontend/node_modules /workspaces/biotron/apps/logger/prisma/node_modules /workspaces/biotron/apps/site/frontend/node_modules /workspaces/biotron/apps/sprinter/frontend/node_modules /workspaces/biotron/apps/sprinter/prisma/node_modules /workspaces/biotron/node_modules /workspaces/biotron/packages/style/node_modules \
    && chown -R node:node /workspaces

USER node

# Fail the image build if a copied binary cannot run as the development user.
RUN go version \
    && node --version \
    && jq --version \
    && golangci-lint version \
    && trufflehog --version \
    && trivy --version
