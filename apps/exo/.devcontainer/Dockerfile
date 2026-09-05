# syntax=docker/dockerfile:1

FROM golang:1.27-bookworm AS go

FROM node:22-bookworm-slim

COPY --from=go /usr/local/go /usr/local/go

ENV PATH="/usr/local/go/bin:${PATH}"

RUN ln -s /usr/local/go/bin/go /usr/local/bin/go \
    && ln -s /usr/local/go/bin/gofmt /usr/local/bin/gofmt

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        git \
        openssh-client \
        openssl \
    && rm -rf /var/lib/apt/lists/*

USER node
