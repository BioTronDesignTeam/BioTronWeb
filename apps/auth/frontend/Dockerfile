# syntax=docker/dockerfile:1

# Built from the repository root (compose sets context: ../..) so the image
# can see packages/style. npm installs only this workspace and the shared
# library, then builds the library before the app that imports it.
FROM node:22-bookworm-slim AS build

WORKDIR /repo

ARG VITE_API_BASE
ENV VITE_API_BASE=${VITE_API_BASE}

COPY package.json package-lock.json ./
COPY packages/style/package.json packages/style/
COPY apps/auth/frontend/package.json apps/auth/frontend/
RUN npm ci --workspace packages/style --workspace apps/auth/frontend

COPY packages/style packages/style
COPY apps/auth/frontend apps/auth/frontend
RUN npm run build -w packages/style && npm run build -w apps/auth/frontend

FROM nginx:1.27-alpine

COPY apps/auth/frontend/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /repo/apps/auth/frontend/dist /usr/share/nginx/html

EXPOSE 80
