#!/usr/bin/env bash
# Creates the sprinter_reader role: read-only access to the logger and oauth
# schemas, for Sprinter's model tools. The official postgres image runs every
# script in /docker-entrypoint-initdb.d/ once, only when the data directory is
# empty, so this only fires on a fresh cluster. Run it again by hand against
# an existing cluster; see the "Read-only role for Sprinter" section in
# infra/README.md for that command.
#
# The read-only guarantee lives here, in Postgres, not in Sprinter's Go code:
# default_transaction_read_only makes every session sprinter_reader opens
# reject writes, even if the application code has a bug.
set -euo pipefail

: "${SPRINTER_READER_PASSWORD:?SPRINTER_READER_PASSWORD is not set. Add it to infra/.env and pass it to this container.}"

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" \
  -v pw="$SPRINTER_READER_PASSWORD" -v owner="$POSTGRES_USER" <<'SQL'
-- Idempotent: safe to run again on an existing cluster (the README's
-- one-shot command does exactly that after Auth's first migration).
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

ALTER ROLE sprinter_reader SET default_transaction_read_only = on;

-- Apps create their own schema on first migrate, so on a fresh cluster
-- neither schema exists yet. Create them here so the grants below have
-- somewhere to land.
CREATE SCHEMA IF NOT EXISTS logger;
CREATE SCHEMA IF NOT EXISTS oauth;

GRANT USAGE ON SCHEMA logger, oauth TO sprinter_reader;

-- Logger: every table, present and future. Logger owns nothing sensitive, so
-- the whole schema is open, and the default-privileges rule covers tables
-- Logger's own migrations add later without this script needing to change.
GRANT SELECT ON ALL TABLES IN SCHEMA logger TO sprinter_reader;
--
-- The rule names the role that will own the future tables, which is the role
-- Logger migrates as: this cluster's superuser. It is passed in as :"owner"
-- rather than written out, so a cluster whose POSTGRES_USER is not "biotron"
-- still gets the grant instead of an error.
ALTER DEFAULT PRIVILEGES FOR ROLE :"owner" IN SCHEMA logger GRANT SELECT ON TABLES TO sprinter_reader;

-- Auth: named tables only, no default privileges. sessions and guest_keys
-- must never be granted here or later; guest_keys holds the daily Exo guest
-- key in plaintext, and sessions is the login token store. A new oauth table
-- needs a deliberate line added to this list, not an automatic grant.
--
-- On a fresh cluster none of these tables exist yet (Auth has not migrated),
-- so each grant is guarded with to_regclass and skipped rather than failing
-- the whole script. Rerun this script after Auth's first migration to pick
-- the grants up.
DO $$
BEGIN
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
