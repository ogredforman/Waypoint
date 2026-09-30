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
