-- Daily guest keys are product credentials, not a single credential shared by
-- every BioTron app. Exo GUI is the only product currently opting in.
ALTER TABLE "apps"
  ADD COLUMN "daily_key_enabled" BOOLEAN NOT NULL DEFAULT false;

UPDATE "apps"
SET "daily_key_enabled" = true
WHERE "id" = 'exo-gui';

-- Preserve today's existing key as Exo's key, then make product + day the
-- identity so future products rotate independently.
ALTER TABLE "guest_keys"
  ADD COLUMN "app_id" TEXT;

UPDATE "guest_keys"
SET "app_id" = 'exo-gui';

ALTER TABLE "guest_keys"
  ALTER COLUMN "app_id" SET NOT NULL,
  DROP CONSTRAINT "guest_keys_pkey",
  ADD CONSTRAINT "guest_keys_pkey" PRIMARY KEY ("app_id", "day"),
  ADD CONSTRAINT "guest_keys_app_id_fkey"
    FOREIGN KEY ("app_id") REFERENCES "apps"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- Guest sessions must remember the product whose key created them. Existing
-- global guest sessions are invalidated because they cannot be safely scoped.
ALTER TABLE "sessions"
  ADD COLUMN "guest_app_id" TEXT;

DELETE FROM "sessions"
WHERE "operator_id" = 0;

ALTER TABLE "sessions"
  ADD CONSTRAINT "sessions_guest_app_id_fkey"
    FOREIGN KEY ("guest_app_id") REFERENCES "apps"("id") ON DELETE CASCADE ON UPDATE CASCADE,
  ADD CONSTRAINT "sessions_guest_product_scope_check"
    CHECK (
      ("operator_id" = 0 AND "guest_app_id" IS NOT NULL)
      OR ("operator_id" <> 0 AND "guest_app_id" IS NULL)
    );

CREATE INDEX "sessions_guest_app_id_idx" ON "sessions"("guest_app_id");
