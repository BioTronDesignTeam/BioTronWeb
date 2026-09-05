-- Introduces guard, automation, and agent-thread/transcript tables for the
-- sprinter schema: bot feature access control, scheduled Calendar
-- announcements, lead nudges, and /agent-thread conversation history.

-- CreateSchema
CREATE SCHEMA IF NOT EXISTS "sprinter";

-- CreateEnum
CREATE TYPE "AutomationKind" AS ENUM ('ANNOUNCE', 'NUDGE');

-- CreateEnum
CREATE TYPE "NudgeDelivery" AS ENUM ('DM', 'CHANNEL');

-- CreateEnum
CREATE TYPE "NudgeTrigger" AS ENUM ('UPCOMING', 'CANCELLED');

-- CreateEnum
CREATE TYPE "NudgeStatus" AS ENUM ('SATISFIED', 'SENT');

-- CreateTable
CREATE TABLE "guards" (
    "subject" TEXT NOT NULL,
    "guild_id" TEXT NOT NULL,
    "role_ids" TEXT[],
    "channel_ids" TEXT[],
    "updated_at" TIMESTAMPTZ(6) NOT NULL,

    CONSTRAINT "guards_pkey" PRIMARY KEY ("subject")
);

-- CreateTable
CREATE TABLE "automations" (
    "id" UUID NOT NULL,
    "kind" "AutomationKind" NOT NULL,
    "name" TEXT NOT NULL,
    "scope_id" UUID NOT NULL,
    "channel_id" TEXT NOT NULL,
    "lead_user_id" TEXT,
    "lead_hours" INTEGER NOT NULL DEFAULT 24,
    "lookback_hours" INTEGER NOT NULL DEFAULT 24,
    "any_author" BOOLEAN NOT NULL DEFAULT false,
    "post_hour" INTEGER,
    "deliver" "NudgeDelivery" NOT NULL DEFAULT 'DM',
    "enabled" BOOLEAN NOT NULL DEFAULT true,
    "created_at" TIMESTAMPTZ(6) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMPTZ(6) NOT NULL,

    CONSTRAINT "automations_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "agent_threads" (
    "thread_id" TEXT NOT NULL,
    "guild_id" TEXT NOT NULL,
    "channel_id" TEXT NOT NULL,
    "opener_id" TEXT NOT NULL,
    "model" TEXT NOT NULL,
    "turn_count" INTEGER NOT NULL DEFAULT 0,
    "created_at" TIMESTAMPTZ(6) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "last_active_at" TIMESTAMPTZ(6) NOT NULL,

    CONSTRAINT "agent_threads_pkey" PRIMARY KEY ("thread_id")
);

-- CreateTable
CREATE TABLE "agent_messages" (
    "id" BIGSERIAL NOT NULL,
    "thread_id" TEXT NOT NULL,
    "sequence" INTEGER NOT NULL,
    "role" TEXT NOT NULL,
    "content" JSONB NOT NULL,
    "created_at" TIMESTAMPTZ(6) NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "agent_messages_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "seen_occurrences" (
    "automation_id" UUID NOT NULL,
    "uid" TEXT NOT NULL,
    "recurrence_id_local" TEXT NOT NULL,
    "sequence" INTEGER NOT NULL,
    "starts_at" TIMESTAMPTZ(6) NOT NULL,
    "ends_at" TIMESTAMPTZ(6) NOT NULL,
    "title" TEXT NOT NULL,
    "scope_id" UUID NOT NULL,
    "first_seen_at" TIMESTAMPTZ(6) NOT NULL,
    "last_seen_at" TIMESTAMPTZ(6) NOT NULL,

    CONSTRAINT "seen_occurrences_pkey" PRIMARY KEY ("automation_id","uid","recurrence_id_local")
);

-- CreateTable
CREATE TABLE "posted_occurrences" (
    "automation_id" UUID NOT NULL,
    "uid" TEXT NOT NULL,
    "recurrence_id_local" TEXT NOT NULL,
    "sequence" INTEGER NOT NULL,
    "channel_id" TEXT NOT NULL,
    "message_id" TEXT NOT NULL,
    "posted_at" TIMESTAMPTZ(6) NOT NULL,

    CONSTRAINT "posted_occurrences_pkey" PRIMARY KEY ("automation_id","uid","recurrence_id_local")
);

-- CreateTable
CREATE TABLE "nudges" (
    "automation_id" UUID NOT NULL,
    "uid" TEXT NOT NULL,
    "recurrence_id_local" TEXT NOT NULL,
    "sequence" INTEGER NOT NULL,
    "trigger" "NudgeTrigger" NOT NULL,
    "status" "NudgeStatus" NOT NULL,
    "message_id" TEXT,
    "created_at" TIMESTAMPTZ(6) NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "nudges_pkey" PRIMARY KEY ("automation_id","uid","recurrence_id_local","sequence","trigger")
);

-- CreateIndex
CREATE UNIQUE INDEX "agent_messages_thread_id_sequence_key" ON "agent_messages"("thread_id", "sequence");

-- AddForeignKey
ALTER TABLE "agent_messages" ADD CONSTRAINT "agent_messages_thread_id_fkey" FOREIGN KEY ("thread_id") REFERENCES "agent_threads"("thread_id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "seen_occurrences" ADD CONSTRAINT "seen_occurrences_automation_id_fkey" FOREIGN KEY ("automation_id") REFERENCES "automations"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "posted_occurrences" ADD CONSTRAINT "posted_occurrences_automation_id_fkey" FOREIGN KEY ("automation_id") REFERENCES "automations"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "nudges" ADD CONSTRAINT "nudges_automation_id_fkey" FOREIGN KEY ("automation_id") REFERENCES "automations"("id") ON DELETE CASCADE ON UPDATE CASCADE;

