#!/bin/sh
# nginx's entrypoint runs this before starting. It writes the server blocks for
# every instance this edge serves, so one Nginx can route several instances of
# the platform on one host by hostname:
#
#   EDGE_SITES="staging=uwbiotron.dev prod=uwbiotron.ca"
#
# Each entry is <instance>=<domain>. Requests for <domain> and its app
# subdomains go to that instance's containers, which carry <instance>-<service>
# aliases on the instance's own network. ./launch edge attaches Nginx to each.
# The laptop instance, with no name, is `local`.
set -eu

sites=/etc/nginx/sites
out=/etc/nginx/conf.d
: "${EDGE_SITES:?set EDGE_SITES, for example \"staging=uwbiotron.dev prod=uwbiotron.ca\"}"

fail() { echo "EDGE_SITES: $*" >&2; exit 1; }

policy=$(grep -v '^#' "$sites/csp-report-only.template" | grep -v '^$')
csp_map=$(mktemp)
rm -f "$out/default.conf"
cp "$sites/default.conf" "$out/00-default.conf"

seen=' '
count=0
for entry in $EDGE_SITES; do
  case $entry in *=*) ;; *) fail "'$entry' is not <instance>=<domain>" ;; esac
  instance=${entry%%=*}
  domain=${entry#*=}
  case $instance in ''|[!a-z]*|*[!a-z0-9]*) fail "invalid instance name '$instance'" ;; esac
  case $domain in ''|*[!a-z0-9.-]*|.*|*..*|*.|-*) fail "invalid domain '$domain'" ;; esac
  case $seen in *" $instance "*|*" $domain "*) fail "'$entry' repeats an instance or a domain" ;; esac
  seen="$seen$instance $domain "
  count=$((count + 1))

  INSTANCE=$instance BASE_DOMAIN=$domain envsubst '${INSTANCE} ${BASE_DOMAIN}' \
    < "$sites/site.conf.template" > "$out/site-$instance.conf"
  # A leading dot in a hostnames map matches the domain and every subdomain.
  printf '    .%s "%s";\n' "$domain" "$(printf '%s' "$policy" | BASE_DOMAIN=$domain envsubst '${BASE_DOMAIN}')" >> "$csp_map"
done
[ "$count" -gt 0 ] || fail 'name at least one instance'

{
  printf 'map $host $csp_report_only {\n    hostnames;\n    default "";\n'
  cat "$csp_map"
  printf '}\n'
} > "$out/01-maps.conf"
rm -f "$csp_map"
