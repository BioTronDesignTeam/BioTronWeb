BEGIN;

-- Logger participates in the same request/grant catalog as every other tool.
INSERT INTO "apps" ("id", "name", "description") VALUES
  ('logger', 'Logger', 'Internal service health and operational logs')
ON CONFLICT ("id") DO UPDATE SET
  "name" = EXCLUDED."name",
  "description" = EXCLUDED."description";

-- Replace Exo's generic View/Write contract with the capabilities exposed by
-- its UI, and register Logger's read-only viewing capability.
INSERT INTO "permissions" ("app_id", "key", "label", "description") VALUES
  ('exo-gui', 'live', 'Live', 'View live exoskeleton telemetry'),
  ('exo-gui', 'historical', 'Historical', 'View recorded exoskeleton telemetry'),
  ('exo-gui', 'commands', 'Commands', 'Send commands to the exoskeleton'),
  ('logger', 'view', 'View', 'View internal service health and operational logs')
ON CONFLICT ("app_id", "key") DO UPDATE SET
  "label" = EXCLUDED."label",
  "description" = EXCLUDED."description";

-- Existing Exo View grants covered both telemetry modes, while Write maps to
-- Commands. Preserve that access before removing the old catalog entries.
INSERT INTO "grants" ("operator_id", "app_id", "permission_key", "created_at")
SELECT "operator_id", "app_id", 'live', "created_at"
FROM "grants"
WHERE "app_id" = 'exo-gui' AND "permission_key" = 'view'
ON CONFLICT DO NOTHING;

INSERT INTO "grants" ("operator_id", "app_id", "permission_key", "created_at")
SELECT "operator_id", "app_id", 'historical', "created_at"
FROM "grants"
WHERE "app_id" = 'exo-gui' AND "permission_key" = 'view'
ON CONFLICT DO NOTHING;

INSERT INTO "grants" ("operator_id", "app_id", "permission_key", "created_at")
SELECT "operator_id", "app_id", 'commands', "created_at"
FROM "grants"
WHERE "app_id" = 'exo-gui' AND "permission_key" = 'write'
ON CONFLICT DO NOTHING;

-- Keep request history intact. A former View request becomes Live and receives
-- a matching Historical record because the old permission covered both.
INSERT INTO "access_requests" (
  "id", "requester_id", "app_id", "permission_key", "status",
  "reviewed_by", "created_at", "reviewed_at"
)
SELECT
  'migrated-' || md5("id" || ':historical'), "requester_id", "app_id",
  'historical', "status", "reviewed_by", "created_at", "reviewed_at"
FROM "access_requests"
WHERE "app_id" = 'exo-gui' AND "permission_key" = 'view'
ON CONFLICT ("id") DO NOTHING;

UPDATE "access_requests"
SET "permission_key" = 'live'
WHERE "app_id" = 'exo-gui' AND "permission_key" = 'view';

UPDATE "access_requests"
SET "permission_key" = 'commands'
WHERE "app_id" = 'exo-gui' AND "permission_key" = 'write';

DELETE FROM "grants"
WHERE "app_id" = 'exo-gui' AND "permission_key" IN ('view', 'write');

DELETE FROM "permissions"
WHERE "app_id" = 'exo-gui' AND "key" IN ('view', 'write');

COMMIT;
