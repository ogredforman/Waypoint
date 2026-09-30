-- name: CreateTask :one
-- A new task goes to the end of its project.
INSERT INTO tasks (
    project_id, milestone_id, parent_task_id, title, notes, status, size, energy,
    position, origin_entry_id, created_at, updated_at
)
VALUES (
    sqlc.arg(project_id), sqlc.narg(milestone_id), sqlc.narg(parent_task_id), sqlc.arg(title),
    sqlc.arg(notes), sqlc.arg(status), sqlc.narg(size), sqlc.narg(energy),
    (SELECT COALESCE(MAX(t.position), 0) + 1 FROM tasks AS t WHERE t.project_id = sqlc.arg(project_id)),
    sqlc.narg(origin_entry_id), sqlc.arg(created_at), sqlc.arg(updated_at)
)
RETURNING *;

-- name: GetTask :one
SELECT * FROM tasks
WHERE id = ?;

-- name: ListTasksByProject :many
SELECT * FROM tasks
WHERE project_id = ?
ORDER BY position, id;

-- name: UpdateTask :one
-- project_id and origin_entry_id are fixed at creation.
UPDATE tasks
SET milestone_id = ?, parent_task_id = ?, title = ?, notes = ?, status = ?,
    size = ?, energy = ?, position = ?, updated_at = ?
WHERE id = ?
RETURNING *;

-- name: DetachSubtasks :exec
-- Subtasks of a deleted task become top-level tasks.
UPDATE tasks
SET parent_task_id = NULL, updated_at = ?
WHERE parent_task_id = ?;

-- name: DeleteTaskDependencies :exec
DELETE FROM task_dependencies
WHERE task_id = sqlc.arg(task_id) OR blocked_by_task_id = sqlc.arg(task_id);

-- name: DeleteTask :one
DELETE FROM tasks
WHERE id = ?
RETURNING *;

-- name: AddTaskDependency :exec
-- Adding a dependency that already exists changes nothing.
INSERT INTO task_dependencies (task_id, blocked_by_task_id, created_at)
VALUES (?, ?, ?)
ON CONFLICT (task_id, blocked_by_task_id) DO NOTHING;

-- name: ListTaskDependenciesByProject :many
-- Dependencies whose dependent task is in the project. Both tasks are in the
-- same project (SPEC 6.2).
SELECT d.id, d.task_id, d.blocked_by_task_id, d.created_at FROM task_dependencies AS d
JOIN tasks AS t ON t.id = d.task_id
WHERE t.project_id = ?
ORDER BY d.id;
