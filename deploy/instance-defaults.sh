#!/usr/bin/env bash
# Fills an instance's derivable settings, then runs the command it is given:
#
#   infisical run ... -- bash deploy/instance-defaults.sh ./launch all --instance staging ...
#
# Infisical holds what differs between instances and cannot be derived:
# BASE_DOMAIN, the Postgres passwords, the GitHub App and session key, and
# Sprinter's Discord and Gemini keys. Everything below
# follows from those, so it is computed here instead of being copied into
# Infisical, where the copies could drift. A value Infisical does set always
# wins.
set -euo pipefail

: "${BASE_DOMAIN:?set BASE_DOMAIN in this environment in Infisical}"
: "${POSTGRES_PASSWORD:?set POSTGRES_PASSWORD in this environment in Infisical}"
: "${SPRINTER_READER_PASSWORD:?set SPRINTER_READER_PASSWORD in this environment in Infisical}"
case $BASE_DOMAIN in
  *[!a-z0-9.-]*|.*|*..*|*.) echo "invalid BASE_DOMAIN '$BASE_DOMAIN'" >&2; exit 1 ;;
esac
# The passwords go into URLs unescaped. openssl rand -hex 32 makes safe ones.
for name in POSTGRES_PASSWORD SPRINTER_READER_PASSWORD; do
  [[ ${!name} =~ ^[A-Za-z0-9._~-]+$ ]] || { echo "$name may use only letters, digits, and ._~- so it fits in a URL" >&2; exit 1; }
done

d=$BASE_DOMAIN
db="postgres:5432/${POSTGRES_DB:-biotron}"
owner="${POSTGRES_USER:-biotron}:$POSTGRES_PASSWORD"
reader="sprinter_reader:$SPRINTER_READER_PASSWORD"
: "${AUTH_DATABASE_URL:=postgresql://$owner@$db?schema=oauth}"
: "${CALENDAR_DATABASE_URL:=postgresql://$owner@$db?schema=calendar}"
: "${STATUS_DATABASE_URL:=postgresql://$owner@$db?schema=logger}"
: "${SPRINTER_DATABASE_URL:=postgresql://$owner@$db?schema=sprinter}"
: "${SPRINTER_READ_DATABASE_URL:=postgresql://$reader@$db}"

: "${AUTH_FRONTEND_URL:=https://auth.$d}"
: "${AUTH_OAUTH_CALLBACK_URL:=https://auth.$d/api/auth/github/callback}"
: "${AUTH_CORS_ORIGINS:=https://exogui.$d,https://status.$d,https://calendar.$d,https://sprinter.$d}"
: "${AUTH_COOKIE_DOMAIN:=.$d}"
: "${AUTH_COOKIE_SECURE:=true}"
: "${CALENDAR_FRONTEND_URL:=https://calendar.$d}"
: "${CALENDAR_SITE_URL:=https://$d}"
: "${CALENDAR_VITE_API_URL:=/api}"
: "${CALENDAR_PUBLIC_BASE_URL:=https://calendar.$d/api}"
: "${CALENDAR_CORS_ORIGINS:=https://calendar.$d,https://$d}"
: "${CALENDAR_ADMIN_CORS_ORIGINS:=https://calendar.$d}"
: "${EXOGUI_FRONTEND_URL:=https://exogui.$d}"
: "${SPRINTER_FRONTEND_URL:=https://sprinter.$d}"

export AUTH_DATABASE_URL CALENDAR_DATABASE_URL STATUS_DATABASE_URL \
  SPRINTER_DATABASE_URL SPRINTER_READ_DATABASE_URL AUTH_FRONTEND_URL \
  AUTH_OAUTH_CALLBACK_URL AUTH_CORS_ORIGINS AUTH_COOKIE_DOMAIN AUTH_COOKIE_SECURE \
  CALENDAR_FRONTEND_URL CALENDAR_SITE_URL CALENDAR_VITE_API_URL \
  CALENDAR_PUBLIC_BASE_URL CALENDAR_CORS_ORIGINS CALENDAR_ADMIN_CORS_ORIGINS \
  EXOGUI_FRONTEND_URL SPRINTER_FRONTEND_URL
exec "$@"
