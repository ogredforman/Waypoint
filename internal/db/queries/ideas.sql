-- name: CreateIdea :one
INSERT INTO ideas (project_id, title, body, status, origin_entry_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetIdea :one
SELECT * FROM ideas
WHERE id = ?;

-- name: ListIdeasByStatus :many
SELECT * FROM ideas
WHERE status = ?
ORDER BY created_at, id;

-- name: ListIdeasByProject :many
SELECT * FROM ideas
WHERE project_id = ?
ORDER BY created_at, id;

-- name: UpdateIdea :one
-- origin_entry_id is fixed at creation.
UPDATE ideas
SET project_id = ?, title = ?, body = ?, status = ?, updated_at = ?
WHERE id = ?
RETURNING *;

-- name: DeleteIdea :one
DELETE FROM ideas
WHERE id = ?
RETURNING *;
