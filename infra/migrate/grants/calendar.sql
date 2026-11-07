-- Runs after every `migrate calendar` deploy, as the cluster owner (see
-- infra/migrate/migrate). A migration that recreates a table drops the grants
-- on it; this puts them back. infra/postgres/init/05-app-roles.sh creates the
-- roles and the default privileges that cover each new table.
DO $$
BEGIN
  IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'calendar_app') THEN
    GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA calendar TO calendar_app;
    GRANT USAGE, SELECT, UPDATE ON ALL SEQUENCES IN SCHEMA calendar TO calendar_app;
    -- Prisma's migration history is the owner's business.
    REVOKE ALL ON calendar._prisma_migrations FROM calendar_app;
  END IF;
END
$$;
