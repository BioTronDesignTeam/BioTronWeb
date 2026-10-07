#!/usr/bin/env bash
# Run on the Actions runner; send environment-specific files over Tailscale SSH.
set -euo pipefail
umask 077
: "${DEPLOY_HOST:?}"
: "${DEPLOY_DOMAIN:?}"
[[ $DEPLOY_HOST =~ ^[a-zA-Z0-9.-]+$ ]] || { echo 'invalid DEPLOY_HOST' >&2; exit 1; }
[[ $DEPLOY_DOMAIN =~ ^[a-zA-Z0-9.-]+$ ]] || { echo 'invalid DEPLOY_DOMAIN' >&2; exit 1; }
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/apps" "$tmp/infra"
for app in site calendar logger auth exo sprinter; do
  name="${app^^}_ENV"
  [[ -n ${!name:-} ]] || { echo "missing GitHub Environment secret $name" >&2; exit 1; }
  mkdir -p "$tmp/apps/$app"
  printf '%s\nBASE_DOMAIN=%s\n' "${!name}" "$DEPLOY_DOMAIN" > "$tmp/apps/$app/.env"
done
[[ -n ${INFRA_ENV:-} ]] || { echo 'missing INFRA_ENV' >&2; exit 1; }
[[ -n ${CLOUDFLARE_TUNNEL_TOKEN:-} ]] || { echo 'missing CLOUDFLARE_TUNNEL_TOKEN' >&2; exit 1; }
printf '%s\nBASE_DOMAIN=%s\nCLOUDFLARE_TUNNEL_TOKEN=%s\n' "$INFRA_ENV" "$DEPLOY_DOMAIN" "$CLOUDFLARE_TUNNEL_TOKEN" > "$tmp/infra/.env"
# The remote directory must already exist and be owned by the deploy user.
# tar transfers hidden .env files without putting their contents in command
# arguments or Actions logs.
tar -C "$tmp" -cf - apps infra | tailscale ssh "deploy@$DEPLOY_HOST" \
  'umask 077; cd /opt/biotron && tar -xf - && chmod 600 apps/*/.env infra/.env'
