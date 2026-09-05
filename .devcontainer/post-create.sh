#!/usr/bin/env bash
# Runs once when the devcontainer is created. Afterwards every app can start
# natively (Vite, go run) against the Postgres and Redis beside it.
set -euo pipefail
cd /workspaces/biotron

echo "== npm: every frontend and the shared library"
npm ci

echo "== go: warm the workspace"
go work sync

echo "== env: one .env per app, from its example, where none exists"
for app in apps/*/; do
  if [ -f "${app}.env.example" ] && [ ! -f "${app}.env" ]; then
    cp "${app}.env.example" "${app}.env"
    echo "   created ${app}.env"
  fi
done

echo "== database: every app's migrations against the sidecar Postgres"
for app in auth calendar logger exo sprinter; do
  (
    set -a; . "apps/${app}/.env"; set +a
    npm ci --prefix "apps/${app}/prisma" --no-audit --no-fund >/dev/null
    npm run --prefix "apps/${app}/prisma" deploy
  )
done

echo "== database: read-only sprinter_reader role on the sidecar"
(
  export PGHOST=postgres PGUSER=biotron PGPASSWORD=change-me
  export POSTGRES_USER=biotron POSTGRES_DB=biotron
  export SPRINTER_READER_PASSWORD=change-me-reader
  bash infra/postgres/init/10-sprinter-reader.sh
)

echo "== ready"
