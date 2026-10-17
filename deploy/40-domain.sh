#!/bin/sh
# nginx's entrypoint runs this before starting. Images carry a neutral domain
# token so staging and production can run the same image digest.
set -eu

: "${BASE_DOMAIN:?set BASE_DOMAIN for deployed frontend images}"
case "$BASE_DOMAIN" in
  *[!a-zA-Z0-9.-]*|.*|*..*|*.) echo 'invalid BASE_DOMAIN' >&2; exit 1 ;;
esac

find /usr/share/nginx/html -type f \( -name '*.html' -o -name '*.js' -o -name '*.css' -o -name '*.xml' -o -name '*.txt' \) -exec sed -i "s/__BASE_DOMAIN__/$BASE_DOMAIN/g" {} +
