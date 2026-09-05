# syntax=docker/dockerfile:1

FROM golang:1.27-bookworm AS build

# Builds from the repository root so the shared go/logclient module is in
# the context. apps/exo/docker-compose.yml sets context: ../..
WORKDIR /src
COPY go/logclient/ go/logclient/
COPY apps/exo/backend/go.mod apps/exo/backend/go.sum apps/exo/backend/
WORKDIR /src/apps/exo/backend
RUN go mod download

COPY apps/exo/backend/ .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/exo-api .

FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --system --uid 10001 app

COPY --from=build /out/exo-api /usr/local/bin/exo-api

USER app
EXPOSE 8080

CMD ["exo-api"]
