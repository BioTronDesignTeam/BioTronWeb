-- Read by sqlc only, never applied. Status's migrations create its tables
-- unqualified, and Status runs them with the logger schema on its
-- search_path. This puts the one table Sprinter reads where it lands in
-- Postgres, so a query that names logger.health_checks finds it.
CREATE SCHEMA logger;
ALTER TABLE health_checks SET SCHEMA logger;
