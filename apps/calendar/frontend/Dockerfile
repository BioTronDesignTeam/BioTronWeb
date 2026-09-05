# syntax=docker/dockerfile:1

# Built from the repository root (compose sets context: ../..) so the image
# can see packages/style. npm installs only this workspace and the shared
# library, then builds the library before the app that imports it.
FROM node:24-bookworm-slim AS build

WORKDIR /repo

ARG VITE_API_URL
ARG VITE_AUTH_URL
ARG VITE_SITE_URL
ENV VITE_API_URL=${VITE_API_URL}
ENV VITE_AUTH_URL=${VITE_AUTH_URL}
ENV VITE_SITE_URL=${VITE_SITE_URL}

COPY package.json package-lock.json ./
COPY packages/style/package.json packages/style/
COPY apps/calendar/frontend/package.json apps/calendar/frontend/
RUN npm ci --workspace packages/style --workspace apps/calendar/frontend

COPY packages/style packages/style
COPY apps/calendar/frontend apps/calendar/frontend
RUN npm run build -w packages/style && npm run build -w apps/calendar/frontend

FROM nginx:1.27-alpine

COPY apps/calendar/frontend/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /repo/apps/calendar/frontend/dist /usr/share/nginx/html

EXPOSE 80
