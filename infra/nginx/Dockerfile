# syntax=docker/dockerfile:1

FROM nginx:1.30.6-alpine@sha256:583b7f2fce1cdb8da5ad5c39ebd3c33b98b87d493190e548bdee1fa8c619c311

# The Alpine packages get their security fixes between base image updates.
RUN apk upgrade --no-cache

COPY nginx.conf /etc/nginx/nginx.conf
COPY snippets /etc/nginx/snippets
COPY sites /etc/nginx/sites
COPY --chmod=0755 docker-entrypoint.d/40-biotron-sites.sh /docker-entrypoint.d/40-biotron-sites.sh
