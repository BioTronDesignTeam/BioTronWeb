-- The public status page walks ninety days of health history in ascending time
-- order per service. The existing (service, checked_at DESC) index cannot supply
-- that order, so the planner fell back to a sequential scan plus a sort that
-- spilled roughly 9.5 MB to disk on every cache miss.
--
-- This index is ascending and covers `ok`, which is the only other column the
-- walk reads. That lets the history query run as an Index Only Scan with no sort
-- node at all. The DESC index stays: the latest-health lookup and the carry-in
-- DISTINCT ON both want descending order.

-- CreateIndex
CREATE INDEX "health_checks_service_checked_at_ok_idx" ON "health_checks"("service", "checked_at", "ok");
