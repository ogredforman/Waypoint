-- name: CreateProject :one
INSERT INTO projects (title, why, definition_of_done, status, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetProject :one
SELECT * FROM projects
WHERE id = ?;

-- name: ListProjects :many
SELECT * FROM projects
ORDER BY created_at, id;

-- Last activity: the latest of the project's own edits, changes to its tasks,
-- blockers, and ideas, and its newest diary entry. Never earlier than
-- updated_at, so every project has one.

-- name: ListProjectActivity :many
SELECT p.id AS project_id,
       CAST(MAX(p.updated_at,
                COALESCE((SELECT MAX(t.updated_at) FROM tasks t WHERE t.project_id = p.id), 0),
                COALESCE((SELECT MAX(b.updated_at) FROM blockers b WHERE b.project_id = p.id), 0),
                COALESCE((SELECT MAX(i.updated_at) FROM ideas i WHERE i.project_id = p.id), 0),
                COALESCE((SELECT MAX(e.created_at) FROM entries e WHERE e.project_id = p.id), 0)) AS INTEGER) AS last_activity
FROM projects p;

-- name: GetProjectActivity :one
SELECT CAST(MAX(p.updated_at,
                COALESCE((SELECT MAX(t.updated_at) FROM tasks t WHERE t.project_id = p.id), 0),
                COALESCE((SELECT MAX(b.updated_at) FROM blockers b WHERE b.project_id = p.id), 0),
                COALESCE((SELECT MAX(i.updated_at) FROM ideas i WHERE i.project_id = p.id), 0),
                COALESCE((SELECT MAX(e.created_at) FROM entries e WHERE e.project_id = p.id), 0)) AS INTEGER) AS last_activity
FROM projects p
WHERE p.id = ?;

-- name: UpdateProject :one
UPDATE projects
SET title = ?, why = ?, definition_of_done = ?, status = ?, updated_at = ?
WHERE id = ?
RETURNING *;

-- name: CountProjectsByStatus :one
SELECT COUNT(*) FROM projects
WHERE status = ?;

-- Deleting a project (ADR 0008). Store.DeleteProject runs these in order, in
-- one transaction, so every foreign key stays valid.

-- name: CountProjectTasks :one
SELECT COUNT(*) FROM tasks WHERE project_id = ?;

-- name: CountProjectBlockers :one
SELECT COUNT(*) FROM blockers WHERE project_id = ?;

-- name: CountProjectIdeas :one
SELECT COUNT(*) FROM ideas WHERE project_id = ?;

-- name: CountProjectMilestones :one
SELECT COUNT(*) FROM milestones WHERE project_id = ?;

-- name: ListProjectEntryIDs :many
-- The entries scoped to the project, oldest first.
SELECT id FROM entries
WHERE project_id = ?
ORDER BY created_at, id;

-- name: CountRunningProjectRuns :one
SELECT COUNT(*) FROM extraction_runs
JOIN entries ON entries.id = extraction_runs.entry_id
WHERE entries.project_id = sqlc.arg(project_id) AND extraction_runs.status = 'running';

-- name: DeleteProjectLinks :exec
-- Observation links to the project or to anything in it. The observations
-- themselves stay.
DELETE FROM observation_links
WHERE (target_type = 'project' AND target_id = sqlc.arg(project_id))
   OR (target_type = 'task' AND target_id IN (SELECT tasks.id FROM tasks WHERE tasks.project_id = sqlc.arg(project_id)))
   OR (target_type = 'blocker' AND target_id IN (SELECT blockers.id FROM blockers WHERE blockers.project_id = sqlc.arg(project_id)))
   OR (target_type = 'idea' AND target_id IN (SELECT ideas.id FROM ideas WHERE ideas.project_id = sqlc.arg(project_id)))
   OR (target_type = 'milestone' AND target_id IN (SELECT milestones.id FROM milestones WHERE milestones.project_id = sqlc.arg(project_id)));

-- name: DeleteProjectBriefing :exec
-- Its recap points go with it (ON DELETE CASCADE).
DELETE FROM briefings WHERE project_id = ?;

-- name: DeleteProjectDependencies :exec
-- A dependency's tasks are always in the same project.
DELETE FROM task_dependencies
WHERE task_id IN (SELECT tasks.id FROM tasks WHERE tasks.project_id = sqlc.arg(project_id))
   OR blocked_by_task_id IN (SELECT tasks.id FROM tasks WHERE tasks.project_id = sqlc.arg(project_id));

-- name: DeleteProjectBlockers :exec
DELETE FROM blockers WHERE project_id = ?;

-- name: DeleteProjectTasks :exec
-- Subtasks go in the same statement as their parents, so the parent_task_id
-- references are only checked once all are gone.
DELETE FROM tasks WHERE project_id = ?;

-- name: DeleteProjectIdeas :exec
DELETE FROM ideas WHERE project_id = ?;

-- name: DeleteProjectMilestones :exec
DELETE FROM milestones WHERE project_id = ?;

-- name: UnscopeProjectEntries :exec
-- Entries kept when their project goes lose the scope; the text is untouched.
UPDATE entries SET project_id = NULL WHERE project_id = ?;

-- name: DeleteProject :one
DELETE FROM projects WHERE id = ?
RETURNING *;
