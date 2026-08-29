-- Role flags
ALTER TABLE "operators" ADD COLUMN IF NOT EXISTS "is_manager" BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE "operators" ADD COLUMN IF NOT EXISTS "is_banned" BOOLEAN NOT NULL DEFAULT false;

-- Drop old grant/request FKs and tables' action-based shape
DELETE FROM "access_requests";
DELETE FROM "grants";

ALTER TABLE "grants" DROP CONSTRAINT IF EXISTS "grants_pkey";
ALTER TABLE "grants" DROP CONSTRAINT IF EXISTS "grants_app_id_fkey";
ALTER TABLE "grants" DROP CONSTRAINT IF EXISTS "grants_operator_id_fkey";
ALTER TABLE "grants" DROP COLUMN IF EXISTS "action";
ALTER TABLE "grants" ADD COLUMN "permission_key" TEXT NOT NULL DEFAULT '';

ALTER TABLE "access_requests" DROP CONSTRAINT IF EXISTS "access_requests_app_id_fkey";
ALTER TABLE "access_requests" DROP CONSTRAINT IF EXISTS "access_requests_requester_id_fkey";
ALTER TABLE "access_requests" DROP CONSTRAINT IF EXISTS "access_requests_reviewed_by_fkey";
ALTER TABLE "access_requests" DROP COLUMN IF EXISTS "action";
ALTER TABLE "access_requests" ADD COLUMN "permission_key" TEXT NOT NULL DEFAULT '';

-- Permission catalog
CREATE TABLE IF NOT EXISTS "permissions" (
    "app_id" TEXT NOT NULL,
    "key" TEXT NOT NULL,
    "label" TEXT NOT NULL,
    "description" TEXT NOT NULL DEFAULT '',
    CONSTRAINT "permissions_pkey" PRIMARY KEY ("app_id", "key")
);

-- Remove ERP; keep calendar + exo-gui
DELETE FROM "apps" WHERE "id" = 'erp';

UPDATE "apps" SET
  "name" = 'Exo GUI',
  "description" = 'Exoskeleton telemetry dashboard — view and control are protected'
WHERE "id" = 'exo-gui';

UPDATE "apps" SET
  "name" = 'Calendar',
  "description" = 'Public calendar — viewing is open; write access is protected'
WHERE "id" = 'calendar';

INSERT INTO "apps" ("id", "name", "description") VALUES
  ('exo-gui', 'Exo GUI', 'Exoskeleton telemetry dashboard — view and control are protected'),
  ('calendar', 'Calendar', 'Public calendar — viewing is open; write access is protected')
ON CONFLICT ("id") DO UPDATE SET
  "name" = EXCLUDED."name",
  "description" = EXCLUDED."description";

INSERT INTO "permissions" ("app_id", "key", "label", "description") VALUES
  ('exo-gui', 'view', 'View', 'View live telemetry and dashboards'),
  ('exo-gui', 'write', 'Write', 'Send control inputs / setpoints'),
  ('calendar', 'write', 'Write', 'Create and edit calendar events')
ON CONFLICT DO NOTHING;

-- Rebuild grants table constraints
ALTER TABLE "grants" ALTER COLUMN "permission_key" DROP DEFAULT;
ALTER TABLE "grants" ADD CONSTRAINT "grants_pkey" PRIMARY KEY ("operator_id", "app_id", "permission_key");
ALTER TABLE "grants" ADD CONSTRAINT "grants_operator_id_fkey" FOREIGN KEY ("operator_id") REFERENCES "operators"("github_id") ON DELETE CASCADE ON UPDATE CASCADE;
ALTER TABLE "grants" ADD CONSTRAINT "grants_permission_fkey" FOREIGN KEY ("app_id", "permission_key") REFERENCES "permissions"("app_id", "key") ON DELETE CASCADE ON UPDATE CASCADE;

ALTER TABLE "access_requests" ALTER COLUMN "permission_key" DROP DEFAULT;
ALTER TABLE "access_requests" ADD CONSTRAINT "access_requests_requester_id_fkey" FOREIGN KEY ("requester_id") REFERENCES "operators"("github_id") ON DELETE CASCADE ON UPDATE CASCADE;
ALTER TABLE "access_requests" ADD CONSTRAINT "access_requests_permission_fkey" FOREIGN KEY ("app_id", "permission_key") REFERENCES "permissions"("app_id", "key") ON DELETE CASCADE ON UPDATE CASCADE;
ALTER TABLE "access_requests" ADD CONSTRAINT "access_requests_reviewed_by_fkey" FOREIGN KEY ("reviewed_by") REFERENCES "operators"("github_id") ON DELETE SET NULL ON UPDATE CASCADE;

ALTER TABLE "permissions" ADD CONSTRAINT "permissions_app_id_fkey" FOREIGN KEY ("app_id") REFERENCES "apps"("id") ON DELETE CASCADE ON UPDATE CASCADE;
