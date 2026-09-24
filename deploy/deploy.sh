#!/usr/bin/env bash
# Run on the Docker host as the deploy user. Never builds on the host.
set -euo pipefail
branch=${1:?branch required}
revision=${2:?revision required}
image_tag=${3:?image tag required}
[[ $branch == staging || $branch == main ]] || exit 2
[[ $revision =~ ^[0-9a-f]{40}$ && $image_tag =~ ^[0-9a-f]{40}$ ]] || exit 2
cd /opt/biotron
git fetch origin "$branch"
[[ $(git rev-parse FETCH_HEAD) == "$revision" ]] || { echo 'branch moved during deployment' >&2; exit 1; }
git checkout -B "$branch" "$revision"
[[ -f infra/.env && -f infra/cloudflare-tunnel-token ]] || { echo 'missing deployment environment' >&2; exit 1; }
export IMAGE_TAG="$image_tag" IMAGE_REGISTRY=ghcr.io/biotrondesignteam/
docker compose -f infra/docker-compose.yml --env-file infra/.env config -q
docker compose config -q
docker compose -f infra/docker-compose.yml --env-file infra/.env pull
docker compose -f infra/docker-compose.yml --env-file infra/.env up -d --no-build
docker compose pull
docker compose up -d --no-build --remove-orphans
docker compose ps
