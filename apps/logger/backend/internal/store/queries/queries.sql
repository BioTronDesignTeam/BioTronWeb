-- name: InsertLog :one
-- Stores one event. The level travels as text and is cast here, so the Go
-- side never needs the enum's type.
INSERT INTO logs (service, level, message, payload)
VALUES (sqlc.arg(service), sqlc.arg(level)::text::log_level, sqlc.arg(message), sqlc.narg(payload)::jsonb)
RETURNING id, service, level::text AS level, message, payload, created_at;

-- name: QueryLogs :many
-- One page of the log explorer, newest first. Every filter but the services
-- is optional: a NULL argument switches its clause off. The cursor is the
-- (created_at, id) of the last row of the previous page, both or neither.
-- Ask for one row more than a page to learn whether another page follows.
SELECT id, service, level::text AS level, message, payload, created_at
FROM logs
WHERE service = ANY(sqlc.arg(services)::text[])
	AND (sqlc.narg(levels)::text[] IS NULL OR level::text = ANY(sqlc.narg(levels)::text[]))
	AND (sqlc.narg(created_from)::timestamptz IS NULL OR created_at >= sqlc.narg(created_from))
	AND (sqlc.narg(created_to)::timestamptz IS NULL OR created_at <= sqlc.narg(created_to))
	AND (sqlc.narg(pattern)::text IS NULL
		OR message ILIKE sqlc.narg(pattern) OR COALESCE(payload::text, '') ILIKE sqlc.narg(pattern))
	AND (sqlc.narg(cursor_created_at)::timestamptz IS NULL
		OR (created_at, id) < (sqlc.narg(cursor_created_at), sqlc.narg(cursor_id)::bigint))
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(row_limit);

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
