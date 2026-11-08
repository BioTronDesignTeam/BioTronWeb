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
END
$$;
