-- name: CreateChangeOp :one
INSERT INTO change_ops (
    changeset_id, position, op_type, payload, rationale, confidence, status,
    applied_payload, created_at, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: ListChangeOpsByChangeset :many
SELECT * FROM change_ops
WHERE changeset_id = ?
ORDER BY position;

-- name: SetChangeOpResult :one
UPDATE change_ops
SET status = ?, applied_payload = ?, updated_at = ?
WHERE id = ?
RETURNING *;

-- name: MoveChangeOp :one
-- A split moves an op to the new changeset, keeping its position. The payload
-- is rewritten with the temp IDs that resolved before the split.
UPDATE change_ops
SET changeset_id = ?, payload = ?, updated_at = ?
WHERE id = ?
RETURNING *;
