# syntax=docker/dockerfile:1

# Built from the repository root (compose sets context: ../..) so the image
# can see packages/style. npm installs only this workspace and the shared
# library, then builds the library before the app that imports it.
FROM node:22-bookworm-slim AS build

WORKDIR /repo

ARG VITE_CALENDAR_API_URL
ARG VITE_CALENDAR_URL
ENV VITE_CALENDAR_API_URL=${VITE_CALENDAR_API_URL}
ENV VITE_CALENDAR_URL=${VITE_CALENDAR_URL}

COPY package.json package-lock.json ./
COPY packages/style/package.json packages/style/
COPY apps/site/frontend/package.json apps/site/frontend/
RUN npm ci --workspace packages/style --workspace apps/site/frontend

COPY packages/style packages/style
COPY apps/site/frontend apps/site/frontend
RUN npm run build -w packages/style && npm run build -w apps/site/frontend

FROM nginx:1.27-alpine

COPY apps/site/frontend/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /repo/apps/site/frontend/dist /usr/share/nginx/html

EXPOSE 80
