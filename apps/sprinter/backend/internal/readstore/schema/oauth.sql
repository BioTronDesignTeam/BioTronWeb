-- Read by sqlc only, never applied. Auth's migrations create its tables
-- unqualified in the oauth schema. These are the four that sprinter_reader
-- holds SELECT on (infra/postgres/init/10-sprinter-reader.sh). guest_keys
-- stays behind on purpose: a query that reads it fails to generate, as it
-- would fail against the role.
CREATE SCHEMA oauth;
ALTER TABLE apps SET SCHEMA oauth;
ALTER TABLE operators SET SCHEMA oauth;
ALTER TABLE permissions SET SCHEMA oauth;
ALTER TABLE grants SET SCHEMA oauth;
