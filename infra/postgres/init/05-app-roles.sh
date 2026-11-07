#!/usr/bin/env bash
# Creates one login role per app, each with data rights in its own schema
# only:
#
#   auth_app       oauth        AUTH_DB_PASSWORD
#   calendar_app   calendar     CALENDAR_DB_PASSWORD
#   status_app     logger       STATUS_DB_PASSWORD
#   sprinter_app   sprinter     SPRINTER_DB_PASSWORD
#
# Each API connects as its role, which can read and write the rows of its own
# schema and nothing else: no DDL, no other schema, no superuser, and so no
# COPY ... TO PROGRAM. Migrations still run as the cluster owner
# (POSTGRES_USER), which owns every schema and table. Default privileges hand
# each table the owner creates in a schema to that schema's app, so a
# migration needs no grant of its own.
#
# The postgres image runs this once on a fresh cluster, from
# /docker-entrypoint-initdb.d/. The postgres-roles service of
# infra/docker-compose.yml runs it again on every start of an instance, before
# any migration, over the network as the owner (PGHOST, PGPASSWORD), so an
# existing cluster gets the roles too and a password changed in Infisical
# reaches the database on the next deploy. It is idempotent.
#
# A role whose password is not set is skipped, so a laptop whose .env predates
# the roles keeps working with the owner's URL; ./launch --runtime refuses to
# deploy without all four.
set -euo pipefail

owner=${POSTGRES_USER:?POSTGRES_USER is not set}
db=${POSTGRES_DB:-$owner}

for entry in auth:oauth calendar:calendar status:logger sprinter:sprinter; do
  app=${entry%%:*} schema=${entry#*:}
  var=$(printf '%s' "$app" | tr '[:lower:]' '[:upper:]')_DB_PASSWORD
  role=${app}_app
  if [[ -z ${!var:-} ]]; then
    echo "postgres-roles: $var is not set; skipping $role" >&2
    continue
  fi
  psql -v ON_ERROR_STOP=1 --no-psqlrc --quiet --username "$owner" --dbname "$db" \
    -v role="$role" -v schema="$schema" -v owner="$owner" -v pw="${!var}" <<'SQL'
-- psql does not expand variables inside a dollar-quoted body, so the role is
-- created through \gexec: the SELECT returns the statement only when the role
-- is missing, and \gexec runs whatever comes back.
SELECT format('CREATE ROLE %I LOGIN', :'role')
WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = :'role')
\gexec
ALTER ROLE :"role" WITH LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT
  NOREPLICATION NOBYPASSRLS PASSWORD :'pw';

CREATE SCHEMA IF NOT EXISTS :"schema" AUTHORIZATION :"owner";
GRANT USAGE ON SCHEMA :"schema" TO :"role";
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA :"schema" TO :"role";
GRANT USAGE, SELECT, UPDATE ON ALL SEQUENCES IN SCHEMA :"schema" TO :"role";
ALTER DEFAULT PRIVILEGES FOR ROLE :"owner" IN SCHEMA :"schema"
  GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO :"role";
ALTER DEFAULT PRIVILEGES FOR ROLE :"owner" IN SCHEMA :"schema"
  GRANT USAGE, SELECT, UPDATE ON SEQUENCES TO :"role";

-- Prisma's migration history is the owner's business.
SELECT format('REVOKE ALL ON %I._prisma_migrations FROM %I', :'schema', :'role')
WHERE to_regclass(format('%I._prisma_migrations', :'schema')) IS NOT NULL
\gexec
SQL
done

# Temporary tables are a foothold for anything that ever runs SQL it did not
# write; no app uses them. The owner, a superuser, keeps them.
psql -v ON_ERROR_STOP=1 --no-psqlrc --quiet --username "$owner" --dbname "$db" -v db="$db" <<'SQL'
REVOKE TEMPORARY ON DATABASE :"db" FROM PUBLIC;
SQL
