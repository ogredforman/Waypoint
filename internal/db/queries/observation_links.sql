-- name: CreateObservationLink :one
INSERT INTO observation_links (observation_id, target_type, target_id, confidence, created_at)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: ListLinksByObservation :many
SELECT * FROM observation_links
WHERE observation_id = ?
ORDER BY id;

-- name: ListLinksByTarget :many
SELECT * FROM observation_links
WHERE target_type = ? AND target_id = ?
ORDER BY created_at, id;

-- name: DeleteLinksByTarget :exec
-- Used when the target is deleted. The observation itself stays.
DELETE FROM observation_links
WHERE target_type = ? AND target_id = ?;

-- name: CreatePendingLink :one
INSERT INTO pending_observation_links (observation_id, change_op_id, target_type, confidence, created_at)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: ListPendingLinksByOp :many
SELECT * FROM pending_observation_links
WHERE change_op_id = ?
ORDER BY id;

-- name: DeletePendingLinksByOp :exec
DELETE FROM pending_observation_links
WHERE change_op_id = ?;
