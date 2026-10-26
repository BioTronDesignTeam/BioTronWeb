BEGIN;

-- Logger is a public status page now. Its log explorer, the one thing
-- logger/view gated, is gone, so the permission and every grant of it go
-- too (grants cascade from the permission).
DELETE FROM "permissions" WHERE "app_id" = 'logger' AND "key" = 'view';

-- Without a permission the app is an empty row in the catalog. Remove it,
-- unless some other permission was added to it by hand.
DELETE FROM "apps"
WHERE "id" = 'logger'
  AND NOT EXISTS (SELECT 1 FROM "permissions" WHERE "app_id" = 'logger');

COMMIT;
