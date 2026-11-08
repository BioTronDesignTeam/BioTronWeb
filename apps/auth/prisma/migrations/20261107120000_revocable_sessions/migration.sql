-- Sessions are signed cookies with no table, so revoking one takes a record of
-- its own. sessions_valid_after refuses every session of an operator signed
-- before it, for "sign out everywhere" and a ban. revoked_sessions refuses one
-- session by its id (the cookie's jti) until it would have expired anyway; the
-- hourly prune deletes it after that.

-- AlterTable
ALTER TABLE "operators" ADD COLUMN "sessions_valid_after" TIMESTAMPTZ(6) NOT NULL DEFAULT '1970-01-01 00:00:00+00'::timestamptz;

-- CreateTable
CREATE TABLE "revoked_sessions" (
    "jti" TEXT NOT NULL,
    "expires_at" TIMESTAMPTZ(6) NOT NULL,

    CONSTRAINT "revoked_sessions_pkey" PRIMARY KEY ("jti")
);

-- CreateIndex
CREATE INDEX "revoked_sessions_expires_at_idx" ON "revoked_sessions"("expires_at");
