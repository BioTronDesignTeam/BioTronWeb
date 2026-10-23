-- Sprinter's own schema. sqlc turns each statement into a method of package
-- queries (sqlc.yaml); package store maps the rows onto its types.

-- --- Guards -----------------------------------------------------------------

-- name: ListGuards :many
SELECT subject, guild_id, role_ids, channel_ids, updated_at
FROM guards ORDER BY subject;

-- name: GetGuard :one
SELECT subject, guild_id, role_ids, channel_ids, updated_at
FROM guards WHERE subject = sqlc.arg(subject);

-- name: UpsertGuard :one
INSERT INTO guards (subject, guild_id, role_ids, channel_ids, updated_at)
VALUES (sqlc.arg(subject), sqlc.arg(guild_id), sqlc.arg(role_ids)::text[], sqlc.arg(channel_ids)::text[], now())
ON CONFLICT (subject) DO UPDATE SET
    guild_id = EXCLUDED.guild_id,
    role_ids = EXCLUDED.role_ids,
    channel_ids = EXCLUDED.channel_ids,
    updated_at = now()
RETURNING subject, guild_id, role_ids, channel_ids, updated_at;

-- name: DeleteGuard :execrows
DELETE FROM guards WHERE subject = sqlc.arg(subject);

-- --- Automations ------------------------------------------------------------

-- name: ListAutomations :many
SELECT id, kind::text AS kind, name, scope_id, channel_id, lead_user_id,
    lead_hours, lookback_hours, any_author, post_hour, deliver::text AS deliver,
    enabled, created_at, updated_at
FROM automations ORDER BY kind, lower(name);

-- name: ListEnabledAutomations :many
-- Disabled automations are skipped entirely, not merely left alone once ticked.
SELECT id, kind::text AS kind, name, scope_id, channel_id, lead_user_id,
    lead_hours, lookback_hours, any_author, post_hour, deliver::text AS deliver,
    enabled, created_at, updated_at
FROM automations WHERE enabled ORDER BY kind, lower(name);

-- name: GetAutomation :one
SELECT id, kind::text AS kind, name, scope_id, channel_id, lead_user_id,
    lead_hours, lookback_hours, any_author, post_hour, deliver::text AS deliver,
    enabled, created_at, updated_at
FROM automations WHERE id = sqlc.arg(id)::uuid;

-- name: CreateAutomation :exec
INSERT INTO automations (
    id, kind, name, scope_id, channel_id, lead_user_id, lead_hours,
    lookback_hours, any_author, post_hour, deliver, enabled, updated_at
) VALUES (
    sqlc.arg(id)::uuid, sqlc.arg(kind)::text::"AutomationKind", sqlc.arg(name),
    sqlc.arg(scope_id)::uuid, sqlc.arg(channel_id), sqlc.narg(lead_user_id),
    sqlc.arg(lead_hours), sqlc.arg(lookback_hours), sqlc.arg(any_author),
    sqlc.narg(post_hour), sqlc.arg(deliver)::text::"NudgeDelivery", sqlc.arg(enabled), now()
);

-- name: UpdateAutomation :execrows
UPDATE automations SET
    kind = sqlc.arg(kind)::text::"AutomationKind", name = sqlc.arg(name),
    scope_id = sqlc.arg(scope_id)::uuid, channel_id = sqlc.arg(channel_id),
    lead_user_id = sqlc.narg(lead_user_id), lead_hours = sqlc.arg(lead_hours),
    lookback_hours = sqlc.arg(lookback_hours), any_author = sqlc.arg(any_author),
    post_hour = sqlc.narg(post_hour), deliver = sqlc.arg(deliver)::text::"NudgeDelivery",
    enabled = sqlc.arg(enabled), updated_at = now()
WHERE id = sqlc.arg(id)::uuid;

-- name: DeleteAutomation :execrows
DELETE FROM automations WHERE id = sqlc.arg(id)::uuid;

-- --- Threads and transcripts ------------------------------------------------

-- name: CreateThread :one
INSERT INTO agent_threads (
    thread_id, guild_id, channel_id, opener_id, model, turn_count, last_active_at
) VALUES (
    sqlc.arg(thread_id), sqlc.arg(guild_id), sqlc.arg(channel_id),
    sqlc.arg(opener_id), sqlc.arg(model), sqlc.arg(turn_count), now()
)
RETURNING thread_id, guild_id, channel_id, opener_id, model, turn_count,
    created_at, last_active_at;

-- name: GetThread :one
SELECT thread_id, guild_id, channel_id, opener_id, model, turn_count,
    created_at, last_active_at
FROM agent_threads WHERE thread_id = sqlc.arg(thread_id);

-- name: TouchThread :one
UPDATE agent_threads
SET turn_count = turn_count + 1, last_active_at = now()
WHERE thread_id = sqlc.arg(thread_id)
RETURNING thread_id, guild_id, channel_id, opener_id, model, turn_count,
    created_at, last_active_at;

-- name: AppendMessage :one
INSERT INTO agent_messages (thread_id, sequence, role, content)
VALUES (sqlc.arg(thread_id), sqlc.arg(sequence), sqlc.arg(role), sqlc.arg(content))
RETURNING id, thread_id, sequence, role, content, created_at;

-- name: ListMessages :many
SELECT id, thread_id, sequence, role, content, created_at
FROM agent_messages WHERE thread_id = sqlc.arg(thread_id) ORDER BY sequence;

-- --- Scheduler --------------------------------------------------------------

-- name: UpsertSeen :one
INSERT INTO seen_occurrences (
    automation_id, uid, recurrence_id_local, sequence, starts_at,
    ends_at, title, scope_id, first_seen_at, last_seen_at
) VALUES (
    sqlc.arg(automation_id)::uuid, sqlc.arg(uid), sqlc.arg(recurrence_id_local),
    sqlc.arg(sequence), sqlc.arg(starts_at), sqlc.arg(ends_at), sqlc.arg(title),
    sqlc.arg(scope_id)::uuid, now(), now()
)
ON CONFLICT (automation_id, uid, recurrence_id_local) DO UPDATE SET
    sequence = EXCLUDED.sequence,
    starts_at = EXCLUDED.starts_at,
    ends_at = EXCLUDED.ends_at,
    title = EXCLUDED.title,
    scope_id = EXCLUDED.scope_id,
    last_seen_at = now()
RETURNING automation_id, uid, recurrence_id_local, sequence,
    starts_at, ends_at, title, scope_id, first_seen_at, last_seen_at;

-- name: ListSeenFuture :many
SELECT automation_id, uid, recurrence_id_local, sequence,
    starts_at, ends_at, title, scope_id, first_seen_at, last_seen_at
FROM seen_occurrences
WHERE automation_id = sqlc.arg(automation_id)::uuid AND starts_at > sqlc.arg(now)
ORDER BY starts_at;

-- name: DeleteSeen :execrows
DELETE FROM seen_occurrences
WHERE automation_id = sqlc.arg(automation_id)::uuid AND uid = sqlc.arg(uid)
    AND recurrence_id_local = sqlc.arg(recurrence_id_local);

-- name: GetPosted :one
SELECT automation_id, uid, recurrence_id_local, sequence,
    channel_id, message_id, posted_at
FROM posted_occurrences
WHERE automation_id = sqlc.arg(automation_id)::uuid AND uid = sqlc.arg(uid)
    AND recurrence_id_local = sqlc.arg(recurrence_id_local);

-- name: UpsertPosted :one
INSERT INTO posted_occurrences (
    automation_id, uid, recurrence_id_local, sequence, channel_id,
    message_id, posted_at
) VALUES (
    sqlc.arg(automation_id)::uuid, sqlc.arg(uid), sqlc.arg(recurrence_id_local),
    sqlc.arg(sequence), sqlc.arg(channel_id), sqlc.arg(message_id), now()
)
ON CONFLICT (automation_id, uid, recurrence_id_local) DO UPDATE SET
    sequence = EXCLUDED.sequence,
    channel_id = EXCLUDED.channel_id,
    message_id = EXCLUDED.message_id,
    posted_at = now()
RETURNING automation_id, uid, recurrence_id_local, sequence,
    channel_id, message_id, posted_at;

-- name: HasNudge :one
SELECT EXISTS(
    SELECT 1 FROM nudges
    WHERE automation_id = sqlc.arg(automation_id)::uuid AND uid = sqlc.arg(uid)
        AND recurrence_id_local = sqlc.arg(recurrence_id_local)
        AND sequence = sqlc.arg(sequence)
        AND trigger = sqlc.arg(trigger)::text::"NudgeTrigger"
);

-- name: InsertNudge :one
INSERT INTO nudges (
    automation_id, uid, recurrence_id_local, sequence, trigger,
    status, message_id
) VALUES (
    sqlc.arg(automation_id)::uuid, sqlc.arg(uid), sqlc.arg(recurrence_id_local),
    sqlc.arg(sequence), sqlc.arg(trigger)::text::"NudgeTrigger",
    sqlc.arg(status)::text::"NudgeStatus", sqlc.narg(message_id)
)
RETURNING automation_id, uid, recurrence_id_local, sequence,
    trigger::text AS trigger, status::text AS status, message_id, created_at;
