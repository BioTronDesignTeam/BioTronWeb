-- logs held the events services stored in Logger before they moved to Grafana
-- and PostHog. Nothing has read or written it since, so it goes, with its
-- indexes and its enum. This deletes those old events for good.

-- DropTable
DROP TABLE "logs";

-- DropEnum
DROP TYPE "log_level";
