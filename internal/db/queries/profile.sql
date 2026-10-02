-- name: GetProfile :one
SELECT * FROM profile
WHERE id = 1;

-- name: SetProfileName :one
UPDATE profile
SET name = sqlc.arg(name),
    updated_at = sqlc.arg(now)
WHERE id = 1
RETURNING *;
