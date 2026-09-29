-- name: ClearBriefingNextTask :exec
-- Used when the task is deleted; the briefing is recomputed afterwards.
UPDATE briefings
SET next_task_id = NULL, updated_at = ?
WHERE next_task_id = ?;
