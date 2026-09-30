-- Entries are immutable: insert and read only. Never add an UPDATE or DELETE here.

-- name: CreateEntry :one
INSERT INTO entries (body, project_id, reply_to_question_id, created_at)
VALUES (?, ?, ?, ?)
RETURNING *;

-- name: GetEntry :one
SELECT * FROM entries
WHERE id = ?;

-- name: ListRecentEntries :many
SELECT * FROM entries
ORDER BY created_at DESC, id DESC
LIMIT ?;

-- name: ListRecentEntriesByProject :many
SELECT * FROM entries
WHERE project_id = ?
ORDER BY created_at DESC, id DESC
LIMIT ?;

-- name: ListEntriesBefore :many
-- Entries written before the given one, newest first. IDs grow with created_at.
SELECT * FROM entries
WHERE id < sqlc.arg(before_id)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(limit);

-- name: ListEntriesBeforeByProject :many
SELECT * FROM entries
WHERE project_id = sqlc.arg(project_id) AND id < sqlc.arg(before_id)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(limit);

-- name: ListRecentEntriesForProject :many
-- Entries about a project, newest first: those scoped to it, and unscoped
-- ones with an observation linked to the project or to one of its tasks or
-- blockers.
SELECT e.* FROM entries AS e
WHERE e.project_id = sqlc.arg(project_id)
   OR (e.project_id IS NULL AND EXISTS (
        SELECT 1 FROM observations AS o
        JOIN observation_links AS l ON l.observation_id = o.id
        WHERE o.entry_id = e.id AND (
              (l.target_type = 'project' AND l.target_id = sqlc.arg(project_id))
           OR (l.target_type = 'task' AND l.target_id IN (SELECT t.id FROM tasks AS t WHERE t.project_id = sqlc.arg(project_id)))
           OR (l.target_type = 'blocker' AND l.target_id IN (SELECT b.id FROM blockers AS b WHERE b.project_id = sqlc.arg(project_id)))
        )
   ))
ORDER BY e.created_at DESC, e.id DESC
LIMIT sqlc.arg(limit);
