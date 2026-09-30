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
    project_id, next_task_id, next_reason, recap, source, generated_at, created_at, updated_at
)
VALUES (
    sqlc.arg(project_id), sqlc.narg(next_task_id), sqlc.arg(next_reason), sqlc.arg(recap),
    sqlc.arg(source), sqlc.arg(generated_at), sqlc.arg(now), sqlc.arg(now)
)
ON CONFLICT (project_id) DO UPDATE
SET next_task_id = excluded.next_task_id,
    next_reason = excluded.next_reason,
    recap = excluded.recap,
    source = excluded.source,
    generated_at = excluded.generated_at,
    updated_at = excluded.updated_at
RETURNING *;
