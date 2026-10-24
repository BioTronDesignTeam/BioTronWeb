-- name: UpsertOperator :exec
INSERT INTO operators (github_id, login, name, avatar_url, is_superuser, last_login_at)
VALUES ($1, $2, $3, $4, $5, now())
ON CONFLICT (github_id) DO UPDATE
SET login = EXCLUDED.login,
    name = EXCLUDED.name,
    avatar_url = EXCLUDED.avatar_url,
    last_login_at = now(),
    is_superuser = EXCLUDED.is_superuser;

-- name: ReconcileSuperusers :execrows
-- SUPERUSER_GITHUB_IDS decides who is a superuser on every request; the
-- column mirrors it for readers of the table, such as Sprinter, and is set
-- from it at startup.
UPDATE operators SET is_superuser = (github_id = ANY(sqlc.arg(ids)::bigint[]))
WHERE is_superuser IS DISTINCT FROM (github_id = ANY(sqlc.arg(ids)::bigint[]));

-- name: GetOperator :one
SELECT github_id, login, COALESCE(name, '') AS name, COALESCE(avatar_url, '') AS avatar_url,
       is_superuser, is_manager, is_banned, last_login_at
FROM operators WHERE github_id = $1;

-- name: ListOrgMembers :many
SELECT github_id, login, COALESCE(name, '') AS name, COALESCE(avatar_url, '') AS avatar_url,
       is_superuser, is_manager, is_banned, last_login_at
FROM operators
WHERE github_id <> $1
ORDER BY login;

-- name: SetBanned :execrows
UPDATE operators SET is_banned = $2 WHERE github_id = $1;

-- name: SetManager :execrows
UPDATE operators SET is_manager = $2 WHERE github_id = $1;

-- name: ListApps :many
SELECT id, name, description, daily_key_enabled FROM apps ORDER BY name;

-- name: ListPermissions :many
SELECT app_id, key, label, description FROM permissions ORDER BY app_id, key;

-- name: GetPermission :one
SELECT app_id, key, label, description FROM permissions WHERE app_id = $1 AND key = $2;

-- name: CreateApp :exec
INSERT INTO apps (id, name, description) VALUES ($1, $2, $3);

-- name: ListGrantsForOperator :many
SELECT operator_id, app_id, permission_key, created_at
FROM grants WHERE operator_id = $1
ORDER BY app_id, permission_key;

-- name: CreateGrant :exec
INSERT INTO grants (operator_id, app_id, permission_key)
VALUES ($1, $2, $3)
ON CONFLICT DO NOTHING;

-- name: DeleteGrant :execrows
DELETE FROM grants WHERE operator_id = $1 AND app_id = $2 AND permission_key = $3;

-- name: DeleteAllGrants :exec
DELETE FROM grants WHERE operator_id = $1;

-- name: EnsureProductDailyKey :one
-- Only explicitly enabled apps can have daily keys. The day arrives as text,
-- YYYY-MM-DD in Eastern time, and goes back out as text.
INSERT INTO guest_keys (app_id, day, key)
SELECT id, sqlc.arg(day)::text::date, sqlc.arg(key)::text FROM apps
WHERE id = sqlc.arg(app_id) AND daily_key_enabled = true
ON CONFLICT (app_id, day) DO UPDATE SET key = guest_keys.key
RETURNING app_id, day::text AS day, key;

-- name: ListDailyKeyApps :many
SELECT id, name, description, daily_key_enabled
FROM apps WHERE daily_key_enabled = true ORDER BY name;

-- name: DeleteOldProductDailyKeys :execrows
-- Product keys for past Eastern days.
DELETE FROM guest_keys WHERE day < (now() AT TIME ZONE 'America/Toronto')::date;
