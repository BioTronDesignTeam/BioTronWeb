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

# A static binary needs no OS: distroless static carries only CA certificates,
# tzdata, and a nonroot user (65532), so there are no packages to patch.
FROM gcr.io/distroless/static-debian13:nonroot@sha256:25ac8f0c3b2334a9eb05452266bd4697864a8c8ef0ef71e5e6480d7f92868329

COPY --from=build /out/oauth-manager /usr/local/bin/oauth-manager

USER nonroot
EXPOSE 8080

CMD ["oauth-manager"]
