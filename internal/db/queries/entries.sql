-- Entries are immutable: insert and read only. Never add an UPDATE or DELETE here.

-- name: CreateEntry :one
INSERT INTO entries (body, project_id, reply_to_question_id, created_at)
VALUES (?, ?, ?, ?)
RETURNING *;

-- name: GetEntry :one
SELECT * FROM entries
WHERE id = ?;

-- name: ListRecentEntries :many
SELECT * FROM entries
ORDER BY created_at DESC, id DESC
LIMIT ?;

-- name: ListRecentEntriesByProject :many
SELECT * FROM entries
WHERE project_id = ?
ORDER BY created_at DESC, id DESC
LIMIT ?;

-- name: ListEntriesBefore :many
-- Entries written before the given one, newest first. IDs grow with created_at.
SELECT * FROM entries
WHERE id < sqlc.arg(before_id)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(limit);

-- name: ListEntriesBeforeByProject :many
SELECT * FROM entries
WHERE project_id = sqlc.arg(project_id) AND id < sqlc.arg(before_id)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(limit);
