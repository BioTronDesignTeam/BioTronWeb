-- What the agent's tools read through the read-only pool. Every table is
-- named with its schema, because the pool has no search_path of its own.

-- name: Logs :many
-- Newest first. Every filter is optional: a null argument is no filter. The
-- cursor is the keyset position (created_at, id) of the last row a page
-- returned, as Logger's own history pages, so a row written mid-walk cannot
-- shift a page.
SELECT id, service, level::text AS level, message, payload, created_at
FROM logger.logs
WHERE (sqlc.narg(service)::text IS NULL OR service = sqlc.narg(service)::text)
    AND (sqlc.narg(level)::text IS NULL OR level::text = sqlc.narg(level)::text)
    AND (sqlc.narg(search)::text IS NULL OR message ILIKE sqlc.narg(search)::text)
    AND (sqlc.narg(from_time)::timestamptz IS NULL OR created_at >= sqlc.narg(from_time)::timestamptz)
    AND (sqlc.narg(to_time)::timestamptz IS NULL OR created_at <= sqlc.narg(to_time)::timestamptz)
    AND (sqlc.narg(cursor_created_at)::timestamptz IS NULL
        OR (created_at, id) < (sqlc.narg(cursor_created_at)::timestamptz, sqlc.narg(cursor_id)::bigint))
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(row_limit);

-- name: HealthHistory :many
SELECT ok, COALESCE(detail, '')::text AS detail, checked_at
FROM logger.health_checks
WHERE service = sqlc.arg(service) AND checked_at >= sqlc.arg(since)
ORDER BY checked_at DESC, id DESC
LIMIT sqlc.arg(row_limit);

-- name: AppName :one
SELECT name FROM oauth.apps WHERE id = sqlc.arg(id);

-- name: Permissions :many
SELECT key, label, description
FROM oauth.permissions WHERE app_id = sqlc.arg(app_id) ORDER BY key
LIMIT sqlc.arg(row_limit);

-- name: Grants :many
-- The permission row is joined outward: a grant whose catalog entry was
-- deleted still admits the operator, so hiding it would understate access.
SELECT o.login, COALESCE(o.name, '')::text AS name, g.permission_key,
    COALESCE(p.label, '')::text AS label, o.is_manager, o.is_superuser, o.is_banned,
    g.created_at
FROM oauth.grants g
JOIN oauth.operators o ON o.github_id = g.operator_id
LEFT JOIN oauth.permissions p
    ON p.app_id = g.app_id AND p.key = g.permission_key
WHERE g.app_id = sqlc.arg(app_id)
ORDER BY lower(o.login), g.permission_key
LIMIT sqlc.arg(row_limit);

-- name: PrivilegedOperators :many
SELECT login, COALESCE(name, '')::text AS name, is_manager, is_superuser, is_banned
FROM oauth.operators
WHERE is_manager OR is_superuser
ORDER BY lower(login)
LIMIT sqlc.arg(row_limit);
