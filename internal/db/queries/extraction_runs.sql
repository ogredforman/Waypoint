-- Extraction runs double as the worker's job queue: pending runs wait to be
-- claimed, running ones are being processed.

-- name: CreateExtractionRun :one
INSERT INTO extraction_runs (entry_id, provider, model, prompt_version, status, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetExtractionRun :one
SELECT * FROM extraction_runs
WHERE id = ?;

-- name: GetExtractionRunEntryID :one
SELECT entry_id FROM extraction_runs
WHERE id = ?;

-- name: ListExtractionRunsByEntry :many
SELECT * FROM extraction_runs
WHERE entry_id = ?
ORDER BY id;

-- name: CountActiveExtractionRunsByEntry :one
SELECT count(*) FROM extraction_runs
WHERE entry_id = ? AND status IN ('pending', 'running');

-- name: CountRunningExtractionRunsByEntry :one
SELECT COUNT(*) FROM extraction_runs
WHERE entry_id = ? AND status = 'running';

-- name: ClaimNextExtractionRun :one
-- Marks the oldest pending run running and returns it.
UPDATE extraction_runs
SET status = 'running', updated_at = sqlc.arg(updated_at)
WHERE id = (
    SELECT id FROM extraction_runs
    WHERE status = 'pending'
    ORDER BY id
    LIMIT 1
)
RETURNING *;

-- name: ResetRunningExtractionRuns :execrows
-- At startup no run is really running: the process that ran them is gone.
UPDATE extraction_runs
SET status = 'pending', updated_at = ?
WHERE status = 'running';

-- name: SetExtractionRunSnapshot :one
UPDATE extraction_runs
SET input_snapshot = ?, updated_at = ?
WHERE id = ?
RETURNING *;

-- name: FinishExtractionRun :one
UPDATE extraction_runs
SET status = ?, raw_output = ?, error = ?, latency_ms = ?, updated_at = ?
WHERE id = ? AND status = 'running'
RETURNING *;
