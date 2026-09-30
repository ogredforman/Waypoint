-- name: CreateChangeset :one
INSERT INTO changesets (
    source, extraction_run_id, parent_changeset_id, status, created_at, updated_at
)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetChangeset :one
SELECT * FROM changesets
WHERE id = ?;

-- name: ResolveChangeset :one
UPDATE changesets
SET status = ?, resolved_at = ?, updated_at = ?
WHERE id = ?
RETURNING *;
