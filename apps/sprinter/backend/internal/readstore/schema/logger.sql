-- Read by sqlc only, never applied. Logger's migrations create its tables
-- unqualified, and Logger runs them with the logger schema on its
-- search_path. This puts them where they land in Postgres, so a query that
-- names logger.logs finds them.
CREATE SCHEMA logger;
ALTER TABLE logs SET SCHEMA logger;
ALTER TABLE health_checks SET SCHEMA logger;
