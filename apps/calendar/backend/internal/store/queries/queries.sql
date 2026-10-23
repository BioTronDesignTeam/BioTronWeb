-- The store's SQL. sqlc makes it into package queries beside this file; see
-- ../../../sqlc.yaml. Explicit casts on parameters and computed columns only
-- tell sqlc the Go type: PostgreSQL resolves them the same way it did before.

-- name: ListScopes :many
WITH RECURSIVE scope_tree AS (
    SELECT id, parent_id, status = 'ACTIVE' AS effectively_active,
           name AS path, slug AS slug_path, TRUE AS is_root
    FROM calendar_scopes WHERE parent_id IS NULL
    UNION ALL
    -- A child of the team root starts its own path, so "Exo" is not
    -- written "BioTron / Exo"; deeper scopes append to their parent.
    SELECT child.id, child.parent_id,
           parent.effectively_active AND child.status = 'ACTIVE',
           CASE WHEN parent.is_root THEN child.name
                ELSE parent.path || ' · ' || child.name END,
           CASE WHEN parent.is_root THEN child.slug
                ELSE parent.slug_path || '-' || child.slug END,
           FALSE
    FROM calendar_scopes child
    JOIN scope_tree parent ON child.parent_id = parent.id
)
SELECT s.id::text AS id, s.kind::text AS kind, s.name, s.slug,
       visibility.path::text AS path, visibility.slug_path::text AS slug_path, s.status::text AS status,
       COALESCE(s.parent_id::text, '')::text AS parent_id, s.archived_at, s.created_at, s.updated_at,
       (SELECT count(*) FROM calendar_scopes c WHERE c.parent_id = s.id) AS child_count,
       (SELECT count(*) FROM event_series e WHERE e.scope_id = s.id) AS event_count
FROM calendar_scopes s
JOIN scope_tree visibility ON visibility.id = s.id
WHERE sqlc.arg(include_archived)::boolean OR visibility.effectively_active
ORDER BY CASE s.kind WHEN 'TEAM' THEN 0 WHEN 'PROJECT' THEN 1 ELSE 2 END, lower(s.name);

-- name: GetScope :one
WITH RECURSIVE scope_tree AS (
    SELECT id, parent_id, name AS path, slug AS slug_path, TRUE AS is_root
    FROM calendar_scopes WHERE parent_id IS NULL
    UNION ALL
    SELECT child.id, child.parent_id,
           CASE WHEN parent.is_root THEN child.name
                ELSE parent.path || ' · ' || child.name END,
           CASE WHEN parent.is_root THEN child.slug
                ELSE parent.slug_path || '-' || child.slug END,
           FALSE
    FROM calendar_scopes child
    JOIN scope_tree parent ON child.parent_id = parent.id
)
SELECT s.id::text AS id, s.kind::text AS kind, s.name, s.slug,
       tree.path::text AS path, tree.slug_path::text AS slug_path, s.status::text AS status,
       COALESCE(s.parent_id::text, '')::text AS parent_id, s.archived_at, s.created_at, s.updated_at,
       (SELECT count(*) FROM calendar_scopes c WHERE c.parent_id = s.id) AS child_count,
       (SELECT count(*) FROM event_series e WHERE e.scope_id = s.id) AS event_count
FROM calendar_scopes s
JOIN scope_tree tree ON tree.id = s.id
WHERE s.id = sqlc.arg(id);

-- name: CreateScope :exec
INSERT INTO calendar_scopes (id, kind, name, slug, parent_id)
VALUES (sqlc.arg(id), sqlc.arg(kind)::"CalendarScopeKind", sqlc.arg(name), sqlc.arg(slug),
        NULLIF(sqlc.arg(parent_id)::text, '')::uuid);

-- name: RenameScope :execrows
UPDATE calendar_scopes SET name = sqlc.arg(name), slug = sqlc.arg(slug), updated_at = now()
WHERE id = sqlc.arg(id);

-- name: SetScopeStatus :execrows
UPDATE calendar_scopes
SET status = sqlc.arg(status)::"CalendarScopeStatus",
    archived_at = CASE WHEN sqlc.arg(status) = 'ARCHIVED' THEN now() ELSE NULL END,
    updated_at = now()
WHERE id = sqlc.arg(id) AND kind <> 'TEAM';

-- name: DeleteScope :execrows
DELETE FROM calendar_scopes WHERE id = sqlc.arg(id);

-- name: ScopeIsActive :one
WITH RECURSIVE ancestors AS (
    SELECT scope.id, scope.parent_id, scope.status FROM calendar_scopes scope WHERE scope.id = sqlc.arg(id)
    UNION ALL
    SELECT parent.id, parent.parent_id, parent.status
    FROM calendar_scopes parent JOIN ancestors child ON child.parent_id = parent.id
)
SELECT COALESCE(bool_and(status = 'ACTIVE'), false)::boolean AS active FROM ancestors;

-- name: ListSeries :many
WITH RECURSIVE scope_tree AS (
    SELECT id, parent_id, status = 'ACTIVE' AS effectively_active,
           name AS path, slug AS slug_path, TRUE AS is_root
    FROM calendar_scopes WHERE parent_id IS NULL
    UNION ALL
    -- A child of the team root starts its own path, so "Exo" is not
    -- written "BioTron / Exo"; deeper scopes append to their parent.
    SELECT child.id, child.parent_id,
           parent.effectively_active AND child.status = 'ACTIVE',
           CASE WHEN parent.is_root THEN child.name
                ELSE parent.path || ' · ' || child.name END,
           CASE WHEN parent.is_root THEN child.slug
                ELSE parent.slug_path || '-' || child.slug END,
           FALSE
    FROM calendar_scopes child
    JOIN scope_tree parent ON child.parent_id = parent.id
)
SELECT e.id::text AS id, e.uid, e.scope_id::text AS scope_id, s.name AS scope_name,
       tree.path::text AS scope_path, s.kind::text AS scope_kind, e.state::text AS state,
       e.title, e.description, e.location, e.url, e.starts_at_local, e.ends_at_local,
       e.timezone, e.all_day, COALESCE(e.recurrence_until::text, '')::text AS recurrence_until, e.sequence,
       e.published_at, e.cancelled_at, e.created_at, e.updated_at
FROM event_series e
JOIN calendar_scopes s ON s.id = e.scope_id
JOIN scope_tree visibility ON visibility.id = s.id
JOIN scope_tree tree ON tree.id = s.id
WHERE (sqlc.arg(scope_id)::text = '' OR e.scope_id = sqlc.arg(scope_id)::uuid)
  AND (sqlc.arg(include_drafts)::boolean OR e.state IN ('PUBLISHED', 'CANCELLED'))
  AND (sqlc.arg(include_hidden_scopes)::boolean OR visibility.effectively_active)
  -- The first occurrence has to start before the window ends, and the
  -- last one has to end after it begins. For a weekly series the last
  -- occurrence is bounded by the inclusive recurrence end, so that date
  -- plus the occurrence duration is what has to be compared, not the
  -- first occurrence's end.
  -- An occurrence can also be moved outside the master's range. Keep
  -- its series when the patched boundaries overlap the window, before
  -- attachOverrides loads the change and Expand applies exact bounds.
  AND (sqlc.narg(window_from)::timestamp IS NULL OR (
        e.starts_at_local < sqlc.narg(window_to)::timestamp
        AND COALESCE(
            (e.recurrence_until + 1)::timestamp + (e.ends_at_local - e.starts_at_local),
            e.ends_at_local
        ) > sqlc.narg(window_from)::timestamp
      ) OR EXISTS (
        SELECT 1 FROM event_overrides o
        WHERE o.series_id = e.id AND o.state = 'MODIFIED'
          AND (o.patch ? 'starts_at_local' OR o.patch ? 'ends_at_local')
          AND COALESCE((o.patch ->> 'starts_at_local')::timestamp, o.recurrence_id_local) < sqlc.narg(window_to)::timestamp
          AND COALESCE(
              (o.patch ->> 'ends_at_local')::timestamp,
              COALESCE((o.patch ->> 'starts_at_local')::timestamp, o.recurrence_id_local)
                  + (e.ends_at_local - e.starts_at_local)
          ) > sqlc.narg(window_from)::timestamp
      ))
ORDER BY e.starts_at_local, lower(e.title);

-- name: GetSeries :one
WITH RECURSIVE scope_tree AS (
    SELECT id, parent_id, name AS path, TRUE AS is_root
    FROM calendar_scopes WHERE parent_id IS NULL
    UNION ALL
    SELECT child.id, child.parent_id,
           CASE WHEN parent.is_root THEN child.name
                ELSE parent.path || ' · ' || child.name END,
           FALSE
    FROM calendar_scopes child
    JOIN scope_tree parent ON child.parent_id = parent.id
)
SELECT e.id::text AS id, e.uid, e.scope_id::text AS scope_id, s.name AS scope_name,
       tree.path::text AS scope_path, s.kind::text AS scope_kind, e.state::text AS state,
       e.title, e.description, e.location, e.url, e.starts_at_local, e.ends_at_local,
       e.timezone, e.all_day, COALESCE(e.recurrence_until::text, '')::text AS recurrence_until, e.sequence,
       e.published_at, e.cancelled_at, e.created_at, e.updated_at
FROM event_series e
JOIN calendar_scopes s ON s.id = e.scope_id
JOIN scope_tree tree ON tree.id = s.id
WHERE e.id = sqlc.arg(id);

-- name: CreateSeries :exec
INSERT INTO event_series (
    id, uid, scope_id, title, description, location, url, starts_at_local,
    ends_at_local, timezone, all_day, recurrence_until
) VALUES (
    sqlc.arg(id), sqlc.arg(uid), sqlc.arg(scope_id), sqlc.arg(title), sqlc.arg(description),
    sqlc.arg(location), sqlc.arg(url), sqlc.arg(starts_at_local), sqlc.arg(ends_at_local),
    sqlc.arg(timezone), sqlc.arg(all_day), sqlc.narg(recurrence_until)
);

-- name: UpdateSeries :execrows
UPDATE event_series SET
    scope_id = sqlc.arg(scope_id), title = sqlc.arg(title), description = sqlc.arg(description),
    location = sqlc.arg(location), url = sqlc.arg(url),
    starts_at_local = sqlc.arg(starts_at_local), ends_at_local = sqlc.arg(ends_at_local),
    timezone = sqlc.arg(timezone), all_day = sqlc.arg(all_day),
    recurrence_until = sqlc.narg(recurrence_until), sequence = sequence + 1, updated_at = now()
WHERE id = sqlc.arg(id) AND sequence = sqlc.arg(expected_sequence);

-- name: PublishSeries :execrows
UPDATE event_series SET state = 'PUBLISHED', published_at = COALESCE(published_at, now()),
    cancelled_at = NULL, sequence = sequence + 1, updated_at = now()
WHERE id = sqlc.arg(id) AND sequence = sqlc.arg(expected_sequence) AND state IN ('DRAFT', 'CANCELLED');

-- name: CancelSeries :execrows
UPDATE event_series SET state = 'CANCELLED', cancelled_at = now(),
    sequence = sequence + 1, updated_at = now()
WHERE id = sqlc.arg(id) AND sequence = sqlc.arg(expected_sequence) AND state = 'PUBLISHED';

-- name: DeleteSeries :one
DELETE FROM event_series WHERE id = sqlc.arg(id) RETURNING title, state::text AS state;

-- name: BumpPublishedSeries :execrows
UPDATE event_series SET sequence = sequence + 1, updated_at = now()
WHERE id = sqlc.arg(id) AND sequence = sqlc.arg(expected_sequence) AND state = 'PUBLISHED';

-- name: UpsertOverride :one
INSERT INTO event_overrides (id, series_id, recurrence_id_local, state, patch, sequence)
VALUES (sqlc.arg(id), sqlc.arg(series_id), sqlc.arg(recurrence_id_local),
        sqlc.arg(state)::"EventOverrideState", sqlc.arg(patch), 1)
ON CONFLICT (series_id, recurrence_id_local) DO UPDATE SET
    state = EXCLUDED.state, patch = EXCLUDED.patch,
    sequence = event_overrides.sequence + 1, updated_at = now()
RETURNING id::text AS id, series_id::text AS series_id, recurrence_id_local, state::text AS state,
          patch, sequence, created_at, updated_at;

-- name: DeleteOverride :execrows
DELETE FROM event_overrides
WHERE series_id = sqlc.arg(series_id) AND recurrence_id_local = sqlc.arg(recurrence_id_local);

-- name: ListOverrides :many
SELECT o.id::text AS id, o.series_id::text AS series_id, o.recurrence_id_local, o.state::text AS state,
       o.patch, o.sequence, o.created_at, o.updated_at
FROM event_overrides o
JOIN event_series e ON e.id = o.series_id
WHERE o.series_id = ANY(sqlc.arg(series_ids)::uuid[])
  AND (
        sqlc.narg(window_from)::timestamp IS NULL
        OR o.patch -> 'starts_at_local' IS NOT NULL
        OR o.patch -> 'ends_at_local' IS NOT NULL
        OR (o.recurrence_id_local < sqlc.narg(window_to)::timestamp
            AND o.recurrence_id_local + (e.ends_at_local - e.starts_at_local) > sqlc.narg(window_from)::timestamp)
      )
ORDER BY o.recurrence_id_local;
