BEGIN;

-- Sprinter's admin UI is gated by Auth like every other tool.
INSERT INTO "apps" ("id", "name", "description") VALUES
  ('sprinter', 'Sprinter', 'Discord bot: agent commands, announcements, and lead nudges')
ON CONFLICT ("id") DO UPDATE SET
  "name" = EXCLUDED."name",
  "description" = EXCLUDED."description";

INSERT INTO "permissions" ("app_id", "key", "label", "description") VALUES
  ('sprinter', 'admin', 'Admin', 'Configure the Discord bot: guards and automations')
ON CONFLICT ("app_id", "key") DO UPDATE SET
  "label" = EXCLUDED."label",
  "description" = EXCLUDED."description";

COMMIT;
