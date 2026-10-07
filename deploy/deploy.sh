#!/usr/bin/env bash
# Run on the Docker host as the deploy user. Never builds on the host.
# main deploys to staging; a v* release tag deploys to production.
set -euo pipefail
ref=${1:?ref required}
revision=${2:?revision required}
case $ref in
  main) remote_ref=refs/heads/main ;;
  v[0-9]*) [[ $ref =~ ^v[0-9A-Za-z.-]+$ ]] || exit 2; remote_ref=refs/tags/$ref ;;
  *) exit 2 ;;
esac
[[ $revision =~ ^[0-9a-f]{40}$ ]] || exit 2
cd /opt/biotron
git fetch origin "$remote_ref"
[[ $(git rev-parse 'FETCH_HEAD^{commit}') == "$revision" ]] || { echo "$ref moved during deployment" >&2; exit 1; }
git checkout --detach "$revision"
[[ -f infra/.env ]] || { echo 'missing deployment environment' >&2; exit 1; }
export IMAGE_TAG="$revision" IMAGE_REGISTRY=ghcr.io/biotrondesignteam/
for app in site calendar logger auth exo sprinter; do export "${app^^}_ENV_FILE=$PWD/apps/$app/.env"; done
docker compose -f infra/docker-compose.yml --env-file infra/.env config -q
docker compose config -q
docker compose -f infra/docker-compose.yml --env-file infra/.env pull
docker compose -f infra/docker-compose.yml --env-file infra/.env up -d --no-build
docker compose pull
docker compose up -d --no-build --remove-orphans
docker compose ps
