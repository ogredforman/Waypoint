-- name: CreateBlocker :one
INSERT INTO blockers (
    project_id, task_id, description, status, origin_entry_id, resolved_entry_id,
    created_at, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetBlocker :one
SELECT * FROM blockers
WHERE id = ?;

-- name: ListBlockersByProject :many
SELECT * FROM blockers
WHERE project_id = ?
ORDER BY created_at, id;

-- name: UpdateBlocker :one
-- project_id and origin_entry_id are fixed at creation.
UPDATE blockers
SET task_id = ?, description = ?, status = ?, resolved_entry_id = ?, updated_at = ?
WHERE id = ?
RETURNING *;

-- name: DetachBlockersFromTask :exec
-- Blockers on a deleted task stay on its project.
UPDATE blockers
SET task_id = NULL, updated_at = ?
WHERE task_id = ?;

-- name: DeleteBlocker :one
DELETE FROM blockers
WHERE id = ?
RETURNING *;
