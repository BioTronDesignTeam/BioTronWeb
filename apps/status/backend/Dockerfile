# syntax=docker/dockerfile:1

FROM golang:1.27.3-bookworm@sha256:972b4110ba0f47539a975c31e52c64765e1ff19f2d74469ae4a595c6676e80a5 AS build

# Builds from the repository root so the shared go/logclient module is in
# the context. apps/status/docker-compose.yml sets context: ../..
WORKDIR /src
COPY go/logclient/ go/logclient/
COPY apps/status/backend/go.mod apps/status/backend/go.sum apps/status/backend/
WORKDIR /src/apps/status/backend
RUN go mod download

COPY apps/status/backend/ .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/status-api .

FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --system --uid 10001 app

COPY --from=build /out/status-api /usr/local/bin/status-api

USER app
EXPOSE 8080

CMD ["status-api"]
