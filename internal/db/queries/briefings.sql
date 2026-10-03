-- name: ClearBriefingNextTask :exec
-- Used when the task is deleted; the briefing is recomputed afterwards.
UPDATE briefings
SET next_task_id = NULL, updated_at = ?
WHERE next_task_id = ?;

-- name: GetBriefingByProject :one
SELECT * FROM briefings
WHERE project_id = ?;

-- name: UpsertBriefing :one
-- A project has at most one briefing; a refresh replaces it.
INSERT INTO briefings (
    project_id, next_task_id, next_reason, source, generated_at, created_at, updated_at
)
VALUES (
    sqlc.arg(project_id), sqlc.narg(next_task_id), sqlc.arg(next_reason),
    sqlc.arg(source), sqlc.arg(generated_at), sqlc.arg(now), sqlc.arg(now)
)
ON CONFLICT (project_id) DO UPDATE
SET next_task_id = excluded.next_task_id,
    next_reason = excluded.next_reason,
    source = excluded.source,
    generated_at = excluded.generated_at,
    updated_at = excluded.updated_at
RETURNING *;

-- name: DeleteBriefingRecap :exec
-- A refresh replaces the whole recap.
DELETE FROM briefing_recap_points
WHERE briefing_id = ?;

-- name: CreateBriefingRecapPoint :exec
INSERT INTO briefing_recap_points (briefing_id, entry_id, position, kind, text)
VALUES (?, ?, ?, ?, ?);

-- name: ListBriefingRecap :many
-- In recap order, each point with its entry's date.
SELECT p.entry_id, p.kind, p.text, e.created_at AS entry_created_at
FROM briefing_recap_points p
JOIN entries e ON e.id = p.entry_id
WHERE p.briefing_id = ?
ORDER BY p.position;
