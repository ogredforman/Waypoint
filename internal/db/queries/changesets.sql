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

-- name: ListChangesetsByEntry :many
-- The extraction changesets of an entry, from all of its runs, split-offs
-- included.
SELECT changesets.* FROM changesets
JOIN extraction_runs ON extraction_runs.id = changesets.extraction_run_id
WHERE extraction_runs.entry_id = ?
ORDER BY changesets.id;

-- name: ListChangesetsByStatus :many
SELECT * FROM changesets
WHERE status = ?
ORDER BY id;
