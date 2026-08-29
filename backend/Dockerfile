# Live-reload Go API. Source is bind-mounted; air rebuilds on .go changes.
FROM golang:1.23-alpine

RUN apk add --no-cache git \
 && go install github.com/air-verse/air@v1.61.7

WORKDIR /app
EXPOSE 8080

CMD ["air", "-c", ".air.toml"]
