# syntax=docker/dockerfile:1

# Built from the repository root (compose sets context: ../..) so the image
# can see packages/style. npm installs the root, which declares the build
# toolchain once for every app, plus this workspace and the shared library,
# with scripts off, because the library's prepare step would try to build
# before its sources are copied in; the explicit build below runs once the
# sources are present, library first, then the app that imports it.
FROM node:24-bookworm-slim AS build

WORKDIR /repo

ARG VITE_CALENDAR_API_URL
ARG VITE_CALENDAR_URL
ARG VITE_SITE_URL
ARG VITE_GOOGLE_SITE_VERIFICATION
ENV VITE_CALENDAR_API_URL=${VITE_CALENDAR_API_URL}
ENV VITE_CALENDAR_URL=${VITE_CALENDAR_URL}
ENV VITE_SITE_URL=${VITE_SITE_URL}
ENV VITE_GOOGLE_SITE_VERIFICATION=${VITE_GOOGLE_SITE_VERIFICATION}

COPY package.json package-lock.json ./
COPY packages/style/package.json packages/style/
COPY apps/site/frontend/package.json apps/site/frontend/
RUN npm ci --ignore-scripts --include-workspace-root --workspace packages/style --workspace apps/site/frontend

COPY packages/style packages/style
COPY apps/site/frontend apps/site/frontend
RUN npm run build -w packages/style && npm run build -w apps/site/frontend

FROM nginx:1.27-alpine

COPY deploy/40-domain.sh /docker-entrypoint.d/40-domain.sh
RUN chmod +x /docker-entrypoint.d/40-domain.sh

COPY apps/site/frontend/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /repo/apps/site/frontend/dist /usr/share/nginx/html

EXPOSE 80
