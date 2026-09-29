-- name: CreateProject :one
INSERT INTO projects (title, why, definition_of_done, status, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetProject :one
SELECT * FROM projects
WHERE id = ?;

-- name: ListProjects :many
SELECT * FROM projects
ORDER BY created_at, id;

-- name: UpdateProject :one
UPDATE projects
SET title = ?, why = ?, definition_of_done = ?, status = ?, updated_at = ?
WHERE id = ?
RETURNING *;
