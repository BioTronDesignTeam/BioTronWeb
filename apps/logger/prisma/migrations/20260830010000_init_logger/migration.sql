-- CreateEnum
CREATE TYPE "log_level" AS ENUM ('debug', 'info', 'warning', 'error');

-- CreateTable
CREATE TABLE "logs" (
    "id" BIGSERIAL NOT NULL,
    "service" TEXT NOT NULL,
    "level" "log_level" NOT NULL,
    "message" TEXT NOT NULL,
    "payload" JSONB,
    "created_at" TIMESTAMPTZ(6) NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "logs_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "health_checks" (
    "id" BIGSERIAL NOT NULL,
    "service" TEXT NOT NULL,
    "ok" BOOLEAN NOT NULL,
    "detail" TEXT,
    "checked_at" TIMESTAMPTZ(6) NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "health_checks_pkey" PRIMARY KEY ("id")
);

-- CreateIndex
CREATE INDEX "logs_service_created_at_idx" ON "logs"("service", "created_at" DESC);

-- CreateIndex
CREATE INDEX "logs_level_created_at_idx" ON "logs"("level", "created_at" DESC);

-- CreateIndex
CREATE INDEX "logs_created_at_idx" ON "logs"("created_at" DESC);

-- CreateIndex
CREATE INDEX "health_checks_service_checked_at_idx" ON "health_checks"("service", "checked_at" DESC);

-- CreateIndex
CREATE INDEX "health_checks_checked_at_idx" ON "health_checks"("checked_at" DESC);
