-- Entries are immutable: their text is never updated. An entry can only be
-- deleted whole, with everything derived from it (ADR 0007, the queries at the
-- end); the only other UPDATE clears a reply link to a deleted question.

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

-- name: ListEntriesBeforeForProject :many
-- Entries about a project (as in ListRecentEntriesForProject) written before
-- the given one, newest first.
SELECT e.* FROM entries AS e
WHERE e.id < sqlc.arg(before_id) AND (
      e.project_id = sqlc.arg(project_id)
   OR (e.project_id IS NULL AND EXISTS (
        SELECT 1 FROM observations AS o
        JOIN observation_links AS l ON l.observation_id = o.id
        WHERE o.entry_id = e.id AND (
              (l.target_type = 'project' AND l.target_id = sqlc.arg(project_id))
           OR (l.target_type = 'task' AND l.target_id IN (SELECT t.id FROM tasks AS t WHERE t.project_id = sqlc.arg(project_id)))
           OR (l.target_type = 'blocker' AND l.target_id IN (SELECT b.id FROM blockers AS b WHERE b.project_id = sqlc.arg(project_id)))
        )
   ))
)
ORDER BY e.created_at DESC, e.id DESC
LIMIT sqlc.arg(limit);

-- Deleting an entry (ADR 0007). Store.DeleteEntry runs these in order, in one
-- transaction, so every foreign key stays valid.

-- name: DeleteEntryPendingLinks :exec
-- Pending links from the entry's observations or on its changesets' ops.
DELETE FROM pending_observation_links
WHERE observation_id IN (SELECT observations.id FROM observations WHERE observations.entry_id = sqlc.arg(entry_id))
   OR change_op_id IN (
       SELECT change_ops.id FROM change_ops
       JOIN changesets ON changesets.id = change_ops.changeset_id
       JOIN extraction_runs ON extraction_runs.id = changesets.extraction_run_id
       WHERE extraction_runs.entry_id = sqlc.arg(entry_id));

-- name: DeleteEntryObservationLinks :exec
DELETE FROM observation_links
WHERE observation_id IN (SELECT id FROM observations WHERE entry_id = ?);

-- name: DeleteEntryObservations :exec
DELETE FROM observations
WHERE entry_id = ?;

-- name: DeleteEntryChangeOps :exec
-- The ops of every changeset from the entry's runs, whatever its status.
DELETE FROM change_ops
WHERE changeset_id IN (
    SELECT changesets.id FROM changesets
    JOIN extraction_runs ON extraction_runs.id = changesets.extraction_run_id
    WHERE extraction_runs.entry_id = ?);

-- name: DeleteEntryChangesets :exec
-- Split-offs go in the same statement as their parents, so the
-- parent_changeset_id references are only checked once all are gone.
DELETE FROM changesets
WHERE extraction_run_id IN (SELECT id FROM extraction_runs WHERE entry_id = ?);

-- name: ClearRepliesToEntryQuestions :exec
-- Entries replying to the deleted entry's questions keep their text and lose
-- the reply link.
UPDATE entries
SET reply_to_question_id = NULL
WHERE reply_to_question_id IN (SELECT id FROM follow_up_questions WHERE entry_id = ?);

-- name: DismissQuestionsAnsweredByEntry :exec
-- A question the deleted entry answered counts as dismissed.
UPDATE follow_up_questions
SET status = 'dismissed', answer_entry_id = NULL, updated_at = ?
WHERE answer_entry_id = ?;

-- name: DeleteEntryQuestions :exec
DELETE FROM follow_up_questions
WHERE entry_id = ?;

-- name: ClearTaskOrigins :exec
-- Rows created from the deleted entry stay, without their origin.
UPDATE tasks
SET origin_entry_id = NULL
WHERE origin_entry_id = ?;

-- name: ClearBlockerEntries :exec
UPDATE blockers
SET origin_entry_id   = CASE WHEN origin_entry_id = sqlc.arg(entry_id) THEN NULL ELSE origin_entry_id END,
    resolved_entry_id = CASE WHEN resolved_entry_id = sqlc.arg(entry_id) THEN NULL ELSE resolved_entry_id END
WHERE origin_entry_id = sqlc.arg(entry_id) OR resolved_entry_id = sqlc.arg(entry_id);

-- name: ClearIdeaOrigins :exec
UPDATE ideas
SET origin_entry_id = NULL
WHERE origin_entry_id = ?;

-- name: DeleteEntryRuns :exec
DELETE FROM extraction_runs
WHERE entry_id = ?;

-- name: DeleteEntry :exec
-- Its briefing recap points go with it (ON DELETE CASCADE).
DELETE FROM entries
WHERE id = ?;
