CREATE TYPE "CalendarScopeKind" AS ENUM ('TEAM', 'PROJECT', 'SUBTEAM');
CREATE TYPE "CalendarScopeStatus" AS ENUM ('ACTIVE', 'ARCHIVED');
CREATE TYPE "EventState" AS ENUM ('DRAFT', 'PUBLISHED', 'CANCELLED');
CREATE TYPE "EventOverrideState" AS ENUM ('MODIFIED', 'CANCELLED');

CREATE TABLE "calendar_scopes" (
  "id" UUID NOT NULL,
  "kind" "CalendarScopeKind" NOT NULL,
  "name" TEXT NOT NULL,
  "slug" TEXT NOT NULL,
  "status" "CalendarScopeStatus" NOT NULL DEFAULT 'ACTIVE',
  "parent_id" UUID,
  "archived_at" TIMESTAMPTZ(6),
  "created_at" TIMESTAMPTZ(6) NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" TIMESTAMPTZ(6) NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT "calendar_scopes_pkey" PRIMARY KEY ("id"),
  CONSTRAINT "calendar_scopes_parent_shape_check" CHECK (
    ("kind" = 'TEAM' AND "parent_id" IS NULL)
    OR ("kind" <> 'TEAM' AND "parent_id" IS NOT NULL)
  )
);

CREATE TABLE "event_series" (
  "id" UUID NOT NULL,
  "uid" TEXT NOT NULL,
  "scope_id" UUID NOT NULL,
  "state" "EventState" NOT NULL DEFAULT 'DRAFT',
  "title" TEXT NOT NULL,
  "description" TEXT NOT NULL DEFAULT '',
  "location" TEXT NOT NULL DEFAULT '',
  "url" TEXT NOT NULL DEFAULT '',
  "starts_at_local" TIMESTAMP(6) NOT NULL,
  "ends_at_local" TIMESTAMP(6) NOT NULL,
  "timezone" TEXT NOT NULL DEFAULT 'America/Toronto',
  "all_day" BOOLEAN NOT NULL DEFAULT false,
  "recurrence_until" DATE,
  "sequence" INTEGER NOT NULL DEFAULT 0,
  "published_at" TIMESTAMPTZ(6),
  "cancelled_at" TIMESTAMPTZ(6),
  "created_at" TIMESTAMPTZ(6) NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" TIMESTAMPTZ(6) NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT "event_series_pkey" PRIMARY KEY ("id"),
  CONSTRAINT "event_series_time_order_check" CHECK ("ends_at_local" > "starts_at_local"),
  CONSTRAINT "event_series_recurrence_end_check" CHECK (
    "recurrence_until" IS NULL OR "recurrence_until" >= "starts_at_local"::date
  )
);

CREATE TABLE "event_overrides" (
  "id" UUID NOT NULL,
  "series_id" UUID NOT NULL,
  "recurrence_id_local" TIMESTAMP(6) NOT NULL,
  "state" "EventOverrideState" NOT NULL DEFAULT 'MODIFIED',
  "patch" JSONB NOT NULL DEFAULT '{}',
  "sequence" INTEGER NOT NULL DEFAULT 0,
  "created_at" TIMESTAMPTZ(6) NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" TIMESTAMPTZ(6) NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT "event_overrides_pkey" PRIMARY KEY ("id"),
  CONSTRAINT "event_overrides_patch_object_check" CHECK (jsonb_typeof("patch") = 'object')
);

CREATE UNIQUE INDEX "calendar_scopes_parent_id_slug_key" ON "calendar_scopes"("parent_id", "slug");
CREATE UNIQUE INDEX "calendar_scopes_single_team_key" ON "calendar_scopes"("kind") WHERE "kind" = 'TEAM';
CREATE INDEX "calendar_scopes_kind_status_idx" ON "calendar_scopes"("kind", "status");
CREATE UNIQUE INDEX "event_series_uid_key" ON "event_series"("uid");
CREATE INDEX "event_series_scope_id_state_idx" ON "event_series"("scope_id", "state");
CREATE INDEX "event_series_starts_at_local_idx" ON "event_series"("starts_at_local");
CREATE UNIQUE INDEX "event_overrides_series_id_recurrence_id_local_key" ON "event_overrides"("series_id", "recurrence_id_local");
CREATE INDEX "event_overrides_series_id_idx" ON "event_overrides"("series_id");

ALTER TABLE "calendar_scopes"
  ADD CONSTRAINT "calendar_scopes_parent_id_fkey"
  FOREIGN KEY ("parent_id") REFERENCES "calendar_scopes"("id") ON DELETE RESTRICT ON UPDATE CASCADE;

ALTER TABLE "event_series"
  ADD CONSTRAINT "event_series_scope_id_fkey"
  FOREIGN KEY ("scope_id") REFERENCES "calendar_scopes"("id") ON DELETE RESTRICT ON UPDATE CASCADE;

ALTER TABLE "event_overrides"
  ADD CONSTRAINT "event_overrides_series_id_fkey"
  FOREIGN KEY ("series_id") REFERENCES "event_series"("id") ON DELETE CASCADE ON UPDATE CASCADE;

INSERT INTO "calendar_scopes" ("id", "kind", "name", "slug") VALUES
  ('00000000-0000-4000-8000-000000000001', 'TEAM', 'BioTron', 'teamwide')
ON CONFLICT ("id") DO NOTHING;
