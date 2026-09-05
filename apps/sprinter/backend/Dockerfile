# syntax=docker/dockerfile:1

FROM golang:1.27-bookworm AS build

WORKDIR /src

COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/sprinter .

FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --system --uid 10001 app

COPY --from=build /out/sprinter /usr/local/bin/sprinter

USER app
EXPOSE 8080

CMD ["sprinter"]
