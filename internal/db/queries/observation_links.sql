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
