-- name: GetExtractionRunEntryID :one
SELECT entry_id FROM extraction_runs
WHERE id = ?;
