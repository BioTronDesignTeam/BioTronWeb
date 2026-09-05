# syntax=docker/dockerfile:1

# Built from the repository root (compose sets context: ../..) so the image
# can see packages/style. npm installs only this workspace and the shared
# library with scripts off, because the library's prepare step would try to
# build before its sources are copied in; the explicit build below runs
# once the sources are present, library first, then the app that imports it.
FROM node:24-bookworm-slim AS build

WORKDIR /repo

ARG VITE_API_URL
ARG VITE_IDP_URL
ENV VITE_API_URL=${VITE_API_URL}
ENV VITE_IDP_URL=${VITE_IDP_URL}

COPY package.json package-lock.json ./
COPY packages/style/package.json packages/style/
COPY apps/logger/frontend/package.json apps/logger/frontend/
RUN npm ci --ignore-scripts --workspace packages/style --workspace apps/logger/frontend

COPY packages/style packages/style
COPY apps/logger/frontend apps/logger/frontend
RUN npm run build -w packages/style && npm run build -w apps/logger/frontend

FROM nginx:1.27-alpine

COPY apps/logger/frontend/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /repo/apps/logger/frontend/dist /usr/share/nginx/html

EXPOSE 80
