#!/usr/bin/env bash
# Creates the sprinter_reader role: SELECT on a few named tables of the logger
# and oauth schemas, for Sprinter's model tools. The postgres image runs every
# script in /docker-entrypoint-initdb.d/ once, on a fresh cluster; the
# postgres-roles service of infra/docker-compose.yml runs it again on every
# start of an instance, before the migrations. The table grants themselves are
# also made after each migration, by infra/migrate/grants/<app>.sql, because
# on a fresh cluster the tables do not exist yet.
#
# What keeps the role read-only is its grants: SELECT on the tables below and
# nothing else, and no TEMPORARY on the database (05-app-roles.sh revokes it
# from PUBLIC). default_transaction_read_only is only a session default that
# the role could turn off with SET; it catches a mistake, it is not the
# guarantee.
set -euo pipefail

: "${SPRINTER_READER_PASSWORD:?SPRINTER_READER_PASSWORD is not set. Add it to infra/.env and pass it to this container.}"

psql -v ON_ERROR_STOP=1 --no-psqlrc --quiet --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" \
  -v pw="$SPRINTER_READER_PASSWORD" -v owner="$POSTGRES_USER" <<'SQL'
-- Idempotent: the postgres-roles service runs it on every start.
--
-- psql does not expand :'pw' inside a dollar-quoted ($$ ... $$) PL/pgSQL
-- body, so the password can't go through a plain "IF NOT EXISTS" DO block.
-- Building the CREATE ROLE statement with format() and running it through
-- \gexec keeps the substitution at the top level: the SELECT returns one row
-- (the statement text) only when the role is missing, and \gexec executes
-- whatever rows come back, so zero rows means nothing runs.
SELECT format('CREATE ROLE sprinter_reader LOGIN PASSWORD %L', :'pw')
WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'sprinter_reader')
\gexec

-- The password follows SPRINTER_READER_PASSWORD on every run, so a value
-- changed in Infisical reaches the database on the next deploy.
ALTER ROLE sprinter_reader WITH LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE
  NOREPLICATION NOBYPASSRLS PASSWORD :'pw';
ALTER ROLE sprinter_reader SET default_transaction_read_only = on;

-- Apps create their own schema on first migrate, so on a fresh cluster
-- neither schema exists yet. Create them here, owned by the role that
-- migrates them, so the grants below have somewhere to land.
CREATE SCHEMA IF NOT EXISTS logger AUTHORIZATION :"owner";
CREATE SCHEMA IF NOT EXISTS oauth AUTHORIZATION :"owner";

GRANT USAGE ON SCHEMA logger, oauth TO sprinter_reader;

-- Status: health_checks only, granted with the oauth tables below. It is the
-- one table Sprinter's tools read; a new logger table needs a deliberate
-- grant there, as a new oauth one does.
--
-- Earlier versions of this script granted the whole schema, present and
-- future tables. Revoking that first narrows an existing cluster when the
-- script is rerun. The default-privileges rule names the role that owns
-- Status's tables, the role Status migrates as: this cluster's superuser. It is
-- passed in as :"owner" rather than written out, so a cluster whose
-- POSTGRES_USER is not "biotron" still runs.
REVOKE SELECT ON ALL TABLES IN SCHEMA logger FROM sprinter_reader;
ALTER DEFAULT PRIVILEGES FOR ROLE :"owner" IN SCHEMA logger REVOKE SELECT ON TABLES FROM sprinter_reader;

-- Auth: named tables only, no default privileges. guest_keys must never be
-- granted here or later: it holds the daily Exo guest key in plaintext. A new
-- oauth table needs a deliberate line added to this list, not an automatic
-- grant.
--
-- On a fresh cluster none of these tables exist yet (Status and Auth have not
-- migrated), so each grant is guarded with to_regclass and skipped rather
-- than failing the whole script. The migrate image makes the same grants
-- after each Status and Auth migration (infra/migrate/grants/), which also
-- restores one that a migration recreating its table dropped; infra/tests
-- checks that both lists agree.
DO $$
BEGIN
  IF to_regclass('logger.health_checks') IS NOT NULL THEN
    GRANT SELECT ON logger.health_checks TO sprinter_reader;
  END IF;
  IF to_regclass('oauth.operators') IS NOT NULL THEN
    GRANT SELECT ON oauth.operators TO sprinter_reader;
  END IF;
  IF to_regclass('oauth.apps') IS NOT NULL THEN
    GRANT SELECT ON oauth.apps TO sprinter_reader;
  END IF;
  IF to_regclass('oauth.permissions') IS NOT NULL THEN
    GRANT SELECT ON oauth.permissions TO sprinter_reader;
  END IF;
  IF to_regclass('oauth.grants') IS NOT NULL THEN
    GRANT SELECT ON oauth.grants TO sprinter_reader;
  END IF;
END
$$;
SQL
