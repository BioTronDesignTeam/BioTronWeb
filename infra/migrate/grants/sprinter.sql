-- Runs after every `migrate sprinter` deploy, as the cluster owner (see
-- infra/migrate/migrate). A migration that recreates a table drops the grants
-- on it; this puts them back. infra/postgres/init/05-app-roles.sh creates the
-- roles and the default privileges that cover each new table.
DO $$
BEGIN
  IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'sprinter_app') THEN
    GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA sprinter TO sprinter_app;
    GRANT USAGE, SELECT, UPDATE ON ALL SEQUENCES IN SCHEMA sprinter TO sprinter_app;
    -- Prisma's migration history is the owner's business.
    REVOKE ALL ON sprinter._prisma_migrations FROM sprinter_app;
  END IF;
END
$$;
