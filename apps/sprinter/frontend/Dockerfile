# syntax=docker/dockerfile:1

# Built from the repository root (compose sets context: ../..) so the image
# can see packages/style. npm installs the root, which declares the build
# toolchain once for every app, plus this workspace and the shared library,
# with scripts off, because the library's prepare step would try to build
# before its sources are copied in; the explicit build below runs once the
# sources are present, library first, then the app that imports it.
FROM node:24.22.2-bookworm-slim@sha256:01d566e8d94d8c619591d78652babfccd3c6992a3dffa9ed5ab39e11cf6c70e9 AS build

WORKDIR /repo

ARG VITE_API_URL
ARG VITE_AUTH_URL
ARG VITE_CALENDAR_URL
ENV VITE_API_URL=${VITE_API_URL}
ENV VITE_AUTH_URL=${VITE_AUTH_URL}
ENV VITE_CALENDAR_URL=${VITE_CALENDAR_URL}

COPY package.json package-lock.json ./
COPY packages/style/package.json packages/style/
COPY apps/sprinter/frontend/package.json apps/sprinter/frontend/
RUN npm ci --ignore-scripts --include-workspace-root --workspace packages/style --workspace apps/sprinter/frontend

COPY packages/style packages/style
COPY apps/sprinter/frontend apps/sprinter/frontend
RUN npm run build -w packages/style && npm run build -w apps/sprinter/frontend

FROM nginx:1.30.6-alpine@sha256:583b7f2fce1cdb8da5ad5c39ebd3c33b98b87d493190e548bdee1fa8c619c311

COPY deploy/40-domain.sh /docker-entrypoint.d/40-domain.sh
RUN chmod +x /docker-entrypoint.d/40-domain.sh

COPY apps/sprinter/frontend/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /repo/apps/sprinter/frontend/dist /usr/share/nginx/html

EXPOSE 80
