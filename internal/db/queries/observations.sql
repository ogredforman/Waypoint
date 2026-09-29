-- name: CreateObservation :one
INSERT INTO observations (
    entry_id, extraction_run_id, kind, summary, span_start, span_end, created_at
)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetObservation :one
SELECT * FROM observations
WHERE id = ?;

-- name: ListObservationsByEntry :many
SELECT * FROM observations
WHERE entry_id = ?
ORDER BY span_start, id;
