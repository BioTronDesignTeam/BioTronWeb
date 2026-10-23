# syntax=docker/dockerfile:1

FROM nginx:1.31.5-alpine

COPY nginx.conf /etc/nginx/nginx.conf
COPY snippets /etc/nginx/snippets
COPY sites /etc/nginx/sites
COPY --chmod=0755 docker-entrypoint.d/40-biotron-sites.sh /docker-entrypoint.d/40-biotron-sites.sh
