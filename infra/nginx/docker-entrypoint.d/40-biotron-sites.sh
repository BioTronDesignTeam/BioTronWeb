#!/bin/sh
# nginx's entrypoint runs this before starting. It writes the server blocks for
# every instance this edge serves, so one Nginx can route several instances of
# the platform on one host by hostname:
#
#   EDGE_SITES="staging=uwbiotron.dev:noindex prod=uwbiotron.ca"
#
# Each entry is <instance>=<domain>[:noindex]. Requests for <domain> and its app
# subdomains go to that instance's containers, which carry <instance>-<service>
# aliases on the instance's own network. ./launch edge attaches Nginx to each.
# The laptop instance, with no name, is `local`.
#
# :noindex asks search engines to stay away from that instance: every response
# carries X-Robots-Tag: noindex, nofollow, and robots.txt disallows everything.
# Mark staging with it, never production. It hides nothing from people.
# The single-quoted $names below are meant for envsubst and nginx, not sh.
# shellcheck disable=SC2016
set -eu

sites=/etc/nginx/sites
out=/etc/nginx/conf.d
: "${EDGE_SITES:?set EDGE_SITES, for example \"staging=uwbiotron.dev:noindex prod=uwbiotron.ca\"}"

fail() { echo "EDGE_SITES: $*" >&2; exit 1; }

policy=$(grep -v '^#' "$sites/csp-report-only.template" | grep -v '^$')
csp_map=$(mktemp)
robots_map=$(mktemp)
rm -f "$out/default.conf"
cp "$sites/default.conf" "$out/00-default.conf"

seen=' '
count=0
for entry in $EDGE_SITES; do
  case $entry in *=*) ;; *) fail "'$entry' is not <instance>=<domain>[:noindex]" ;; esac
  instance=${entry%%=*}
  domain=${entry#*=}
  flag=
  case $domain in *:*) flag=${domain#*:}; domain=${domain%%:*} ;; esac
  case $instance in ''|[!a-z]*|*[!a-z0-9]*) fail "invalid instance name '$instance'" ;; esac
  case $domain in ''|*[!a-z0-9.-]*|.*|*..*|*.|-*) fail "invalid domain '$domain'" ;; esac
  case $seen in *" $instance "*|*" $domain "*) fail "'$entry' repeats an instance or a domain" ;; esac
  seen="$seen$instance $domain "
  count=$((count + 1))

  # A leading dot in a hostnames map matches the domain and every subdomain.
  case $flag in
    '') robots='' ;;
    noindex)
      robots='include /etc/nginx/snippets/robots-noindex.conf;'
      printf '    .%s "noindex, nofollow";\n' "$domain" >> "$robots_map" ;;
    *) fail "unknown flag ':$flag' in '$entry'; the only flag is :noindex" ;;
  esac

  INSTANCE=$instance BASE_DOMAIN=$domain ROBOTS=$robots envsubst '${INSTANCE} ${BASE_DOMAIN} ${ROBOTS}' \
    < "$sites/site.conf.template" > "$out/site-$instance.conf"
  printf '    .%s "%s";\n' "$domain" "$(printf '%s' "$policy" | BASE_DOMAIN=$domain envsubst '${BASE_DOMAIN}')" >> "$csp_map"
done
[ "$count" -gt 0 ] || fail 'name at least one instance'

{
  printf 'map $host $csp_report_only {\n    hostnames;\n    default "";\n'
  cat "$csp_map"
  printf '}\n\nmap $host $robots_tag {\n    hostnames;\n    default "";\n'
  cat "$robots_map"
  printf '}\n'
} > "$out/01-maps.conf"
rm -f "$csp_map" "$robots_map"
