# syntax=docker/dockerfile:1

FROM golang:1.27.3-bookworm@sha256:972b4110ba0f47539a975c31e52c64765e1ff19f2d74469ae4a595c6676e80a5 AS build

# Builds from the repository root so the shared go/logclient module is in
# the context. apps/auth/docker-compose.yml sets context: ../..
WORKDIR /src
COPY go/logclient/ go/logclient/
COPY apps/auth/backend/go.mod apps/auth/backend/go.sum apps/auth/backend/
WORKDIR /src/apps/auth/backend
RUN go mod download

COPY apps/auth/backend/ .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/oauth-manager .

FROM debian:12.15-slim@sha256:734a72b3d6142a71993b4bc580ca01d8ca687f77a2c422d96a961d3376b179bc

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --system --uid 10001 app

COPY --from=build /out/oauth-manager /usr/local/bin/oauth-manager

USER app
EXPOSE 8080

CMD ["oauth-manager"]
