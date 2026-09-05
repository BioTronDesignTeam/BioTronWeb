# syntax=docker/dockerfile:1

FROM node:24-bookworm-slim

WORKDIR /app

COPY package.json package-lock.json ./
RUN npm ci

COPY . .

# Applying migrations needs no root; the image's built-in node user does it.
USER node

CMD ["npm", "run", "deploy"]
