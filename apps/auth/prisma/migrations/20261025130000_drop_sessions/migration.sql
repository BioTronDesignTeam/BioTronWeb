-- sessions held the opaque login tokens from before Auth signed its session
-- cookies. Those tokens no longer validate, and nothing has read or written
-- the table since, so it goes, with its indexes and foreign keys.

-- DropTable
DROP TABLE "sessions";
