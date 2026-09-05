# syntax=docker/dockerfile:1

FROM node:24-bookworm-slim

WORKDIR /app

RUN apt-get update \
    && apt-get install -y --no-install-recommends openssl \
    && rm -rf /var/lib/apt/lists/*

COPY package*.json ./
RUN npm ci

COPY . .

# Applying migrations needs no root; the image's built-in node user does it.
USER node

CMD ["npm", "run", "deploy"]
