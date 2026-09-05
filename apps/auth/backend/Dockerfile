# syntax=docker/dockerfile:1

FROM golang:1.27-bookworm AS build

# Builds from the repository root so the shared go/logclient module is in
# the context. apps/auth/docker-compose.yml sets context: ../..
WORKDIR /src
COPY go/logclient/ go/logclient/
COPY apps/auth/backend/go.mod apps/auth/backend/go.sum apps/auth/backend/
WORKDIR /src/apps/auth/backend
RUN go mod download

COPY apps/auth/backend/ .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/oauth-manager .

FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --system --uid 10001 app

COPY --from=build /out/oauth-manager /usr/local/bin/oauth-manager

USER app
EXPOSE 8080

CMD ["oauth-manager"]
