-- name: CreateMilestone :one
-- A new milestone goes to the end of its project.
INSERT INTO milestones (project_id, title, description, status, position, created_at, updated_at)
VALUES (
    sqlc.arg(project_id), sqlc.arg(title), sqlc.arg(description), sqlc.arg(status),
    (SELECT COALESCE(MAX(m.position), 0) + 1 FROM milestones AS m WHERE m.project_id = sqlc.arg(project_id)),
    sqlc.arg(created_at), sqlc.arg(updated_at)
)
RETURNING *;

-- name: GetMilestone :one
SELECT * FROM milestones
WHERE id = ?;

-- name: ListMilestonesByProject :many
SELECT * FROM milestones
WHERE project_id = ?
ORDER BY position, id;

-- name: UpdateMilestone :one
-- project_id is fixed at creation.
UPDATE milestones
SET title = ?, description = ?, status = ?, position = ?, updated_at = ?
WHERE id = ?
RETURNING *;

-- name: DetachTasksFromMilestone :exec
-- Tasks in a deleted milestone stay in its project, with no milestone.
UPDATE tasks
SET milestone_id = NULL, updated_at = ?
WHERE milestone_id = ?;

-- name: DeleteMilestone :one
DELETE FROM milestones
WHERE id = ?
RETURNING *;
