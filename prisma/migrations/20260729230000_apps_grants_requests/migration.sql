-- AlterTable
ALTER TABLE "operators" ADD COLUMN "is_superuser" BOOLEAN NOT NULL DEFAULT false;

-- CreateTable
CREATE TABLE "apps" (
    "id" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "description" TEXT NOT NULL DEFAULT '',
    "created_at" TIMESTAMPTZ(6) NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "apps_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "grants" (
    "operator_id" BIGINT NOT NULL,
    "app_id" TEXT NOT NULL,
    "action" TEXT NOT NULL,
    "created_at" TIMESTAMPTZ(6) NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "grants_pkey" PRIMARY KEY ("operator_id","app_id","action")
);

-- CreateTable
CREATE TABLE "access_requests" (
    "id" TEXT NOT NULL,
    "requester_id" BIGINT NOT NULL,
    "app_id" TEXT NOT NULL,
    "action" TEXT NOT NULL,
    "status" TEXT NOT NULL DEFAULT 'pending',
    "reviewed_by" BIGINT,
    "created_at" TIMESTAMPTZ(6) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "reviewed_at" TIMESTAMPTZ(6),

    CONSTRAINT "access_requests_pkey" PRIMARY KEY ("id")
);

-- CreateIndex
CREATE INDEX "grants_app_id_idx" ON "grants"("app_id");

-- CreateIndex
CREATE INDEX "access_requests_status_idx" ON "access_requests"("status");

-- CreateIndex
CREATE INDEX "access_requests_requester_id_idx" ON "access_requests"("requester_id");

-- CreateIndex
CREATE INDEX "access_requests_app_id_idx" ON "access_requests"("app_id");

-- AddForeignKey
ALTER TABLE "grants" ADD CONSTRAINT "grants_operator_id_fkey" FOREIGN KEY ("operator_id") REFERENCES "operators"("github_id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "grants" ADD CONSTRAINT "grants_app_id_fkey" FOREIGN KEY ("app_id") REFERENCES "apps"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "access_requests" ADD CONSTRAINT "access_requests_requester_id_fkey" FOREIGN KEY ("requester_id") REFERENCES "operators"("github_id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "access_requests" ADD CONSTRAINT "access_requests_app_id_fkey" FOREIGN KEY ("app_id") REFERENCES "apps"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "access_requests" ADD CONSTRAINT "access_requests_reviewed_by_fkey" FOREIGN KEY ("reviewed_by") REFERENCES "operators"("github_id") ON DELETE SET NULL ON UPDATE CASCADE;

-- Seed placeholder apps
INSERT INTO "apps" ("id", "name", "description") VALUES
  ('exo-gui', 'Exo GUI', 'Exoskeleton telemetry dashboard'),
  ('erp', 'ERP', 'BioTron ERP / operations tooling'),
  ('calendar', 'Calendar', 'Public / team calendar');
