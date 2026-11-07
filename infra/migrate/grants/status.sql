-- Runs after every `migrate status` deploy, as the cluster owner (see
-- infra/migrate/migrate). A migration that recreates a table drops the grants
-- on it; this puts them back. infra/postgres/init/05-app-roles.sh creates the
-- roles and the default privileges that cover each new table.
DO $$
BEGIN
  IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'status_app') THEN
    GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA logger TO status_app;
    GRANT USAGE, SELECT, UPDATE ON ALL SEQUENCES IN SCHEMA logger TO status_app;
    -- Prisma's migration history is the owner's business.
    REVOKE ALL ON logger._prisma_migrations FROM status_app;
  END IF;
  -- Sprinter's read-only role: these named tables and no others. Keep this
  -- list the same as infra/postgres/init/10-sprinter-reader.sh's.
  IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'sprinter_reader') THEN
    GRANT SELECT ON logger.health_checks TO sprinter_reader;
  END IF;
END
$$;
