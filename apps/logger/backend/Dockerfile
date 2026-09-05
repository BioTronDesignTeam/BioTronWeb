# syntax=docker/dockerfile:1

FROM golang:1.27-bookworm AS build

# Builds from the repository root so the shared go/logclient module is in
# the context. apps/logger/docker-compose.yml sets context: ../..
WORKDIR /src
COPY go/logclient/ go/logclient/
COPY apps/logger/backend/go.mod apps/logger/backend/go.sum apps/logger/backend/
WORKDIR /src/apps/logger/backend
RUN go mod download

COPY apps/logger/backend/ .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/logger-api .

FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --system --uid 10001 app

COPY --from=build /out/logger-api /usr/local/bin/logger-api

USER app
EXPOSE 8080

CMD ["logger-api"]
