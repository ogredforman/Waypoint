-- name: CreateFollowUpQuestion :one
INSERT INTO follow_up_questions (
    entry_id, extraction_run_id, text, reason, status, answer_entry_id, created_at, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: ListFollowUpQuestionsByEntry :many
SELECT * FROM follow_up_questions
WHERE entry_id = ?
ORDER BY id;

-- name: DismissOpenFollowUpQuestionsByEntry :execrows
UPDATE follow_up_questions
SET status = 'dismissed', updated_at = ?
WHERE entry_id = ? AND status = 'open';

-- name: GetFollowUpQuestion :one
SELECT * FROM follow_up_questions
WHERE id = ?;

-- name: DismissFollowUpQuestion :one
-- Only an open question can be dismissed; no row means it wasn't open.
UPDATE follow_up_questions
SET status = 'dismissed', updated_at = ?
WHERE id = ? AND status = 'open'
RETURNING *;

-- name: AnswerFollowUpQuestion :one
-- Only an open question can be answered; no row means it wasn't open.
UPDATE follow_up_questions
SET status = 'answered', answer_entry_id = ?, updated_at = ?
WHERE id = ? AND status = 'open'
RETURNING *;
