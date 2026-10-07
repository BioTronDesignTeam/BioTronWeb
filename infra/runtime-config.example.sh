# Public deployment settings only. Source before `infisical run`.
# Copy to /etc/biotron/runtime-config.sh and choose the environment's domain.
# Do not put credentials in this file. VITE_* changes still require a rebuild.
export BASE_DOMAIN=uwbiotron.dev
export AUTH_FRONTEND_URL="https://auth.$BASE_DOMAIN"
export AUTH_OAUTH_CALLBACK_URL="https://auth.$BASE_DOMAIN/api/auth/github/callback"
export AUTH_COOKIE_DOMAIN=".$BASE_DOMAIN"
export AUTH_COOKIE_SECURE=true
export AUTH_VITE_API_BASE=/api
export AUTH_GITHUB_ORG=BioTronDesignTeam
# Set the numeric GitHub user IDs allowed to bootstrap administration.
export AUTH_SUPERUSER_GITHUB_IDS=
export CALENDAR_FRONTEND_URL="https://calendar.$BASE_DOMAIN"
export CALENDAR_SITE_URL="https://$BASE_DOMAIN"
export CALENDAR_VITE_API_URL="https://calendar.$BASE_DOMAIN/api"
export CALENDAR_VITE_AUTH_URL="https://auth.$BASE_DOMAIN/api"
export EXO_FRONTEND_URL="https://exogui.$BASE_DOMAIN"
export EXO_VITE_API_URL=/api
export EXO_VITE_AUTH_URL="https://auth.$BASE_DOMAIN/api"
export LOGGER_VITE_API_URL=/api
export LOGGER_VITE_IDP_URL="https://auth.$BASE_DOMAIN/api"
export SPRINTER_FRONTEND_URL="https://sprinter.$BASE_DOMAIN"
export SPRINTER_VITE_API_URL=/api
export SPRINTER_VITE_AUTH_URL="https://auth.$BASE_DOMAIN/api"
export SPRINTER_VITE_CALENDAR_URL="https://calendar.$BASE_DOMAIN/api"
export SITE_VITE_SITE_URL="https://$BASE_DOMAIN"
export SITE_VITE_CALENDAR_API_URL="https://calendar.$BASE_DOMAIN/api"
export SITE_VITE_CALENDAR_URL="https://calendar.$BASE_DOMAIN"
# Docker bridge default; narrow to the biotron network's subnet on your host.
export CALENDAR_TRUSTED_PROXIES=127.0.0.1,::1,172.16.0.0/12
export SPRINTER_TRUSTED_PROXIES=127.0.0.1,::1,172.16.0.0/12
