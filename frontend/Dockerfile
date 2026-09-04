# Production image for the Biotron static site.
#
# Build stage uses Debian slim (glibc) so npm / Vite / esbuild optional
# binaries install without musl friction. Runtime is nginx on Alpine —
# this site is 100% static HTML/JS/CSS, so Alpine is the lighter and
# better fit (~40MB vs ~80MB+ for nginx:bookworm-slim).
FROM node:22-bookworm-slim AS build

WORKDIR /app

ARG VITE_CALENDAR_API_URL
ARG VITE_CALENDAR_URL
ENV VITE_CALENDAR_API_URL=${VITE_CALENDAR_API_URL}
ENV VITE_CALENDAR_URL=${VITE_CALENDAR_URL}

COPY package.json package-lock.json ./
RUN npm ci

COPY index.html tsconfig.json vite.config.ts ./
COPY public ./public
COPY src ./src

RUN npm run build

# Pinned to an exact patch release, not a floating `1.31-alpine`. A minor tag
# is mutable, so two builds of the same commit could ship different nginx
# binaries. Bump this line deliberately when a newer patch is wanted.
FROM nginx:1.31.5-alpine

RUN apk add --no-cache wget

COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html

EXPOSE 80

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- http://127.0.0.1/ >/dev/null || exit 1
