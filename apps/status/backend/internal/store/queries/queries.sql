-- name: InsertHealth :exec
INSERT INTO health_checks (service, ok, detail, checked_at)
VALUES (sqlc.arg(service), sqlc.arg(ok), sqlc.narg(detail), sqlc.arg(checked_at));

-- name: LatestHealth :many
-- The most recent observation of each service.
SELECT DISTINCT ON (service) service, ok, COALESCE(detail, '')::text AS detail, checked_at
FROM health_checks
WHERE service = ANY(sqlc.arg(services)::text[])
ORDER BY service, checked_at DESC;

-- name: HealthHistory :many
-- The observations needed to walk uptime across (window_start, window_end],
-- collapsed. Store.HealthHistory explains the collapse and the continuous
-- flag; gap_tolerance must be the walk's maximum gap.
WITH window_rows AS (
	SELECT service, ok, checked_at,
		lag(ok) OVER w AS previous_ok,
		lag(checked_at) OVER w AS previous_at,
		lead(checked_at) OVER w AS next_at
	FROM health_checks
	WHERE service = ANY(sqlc.arg(services)::text[])
		AND checked_at > sqlc.arg(window_start)::timestamptz AND checked_at <= sqlc.arg(window_end)::timestamptz
	WINDOW w AS (PARTITION BY service ORDER BY checked_at)
)
(
	SELECT DISTINCT ON (service) service, ok, checked_at, false AS continuous
	FROM health_checks
	WHERE service = ANY(sqlc.arg(services)::text[]) AND checked_at <= sqlc.arg(window_start)::timestamptz
	ORDER BY service, checked_at DESC
)
UNION ALL
(
	SELECT service, ok, checked_at,
		next_at IS NOT NULL AND next_at - checked_at <= sqlc.arg(gap_tolerance)::interval AS continuous
	FROM window_rows
	WHERE previous_ok IS DISTINCT FROM ok
		OR checked_at - previous_at > sqlc.arg(gap_tolerance)::interval
		OR next_at - checked_at > sqlc.arg(gap_tolerance)::interval
		OR next_at IS NULL
)
ORDER BY service, checked_at;

-- name: DeleteHealthBefore :execrows
-- Retention: drops the rows older than every window the status page shows.
-- health_checks_checked_at_idx serves the range.
DELETE FROM health_checks
WHERE checked_at < sqlc.arg(cutoff)::timestamptz;
