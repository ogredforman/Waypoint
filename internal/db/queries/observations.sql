-- name: CreateObservation :one
INSERT INTO observations (
    entry_id, extraction_run_id, kind, summary, span_start, span_end, created_at
)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetObservation :one
SELECT * FROM observations
WHERE id = ?;

-- name: ListObservationsByEntry :many
SELECT * FROM observations
WHERE entry_id = ?
ORDER BY span_start, id;

-- name: ListProjectObservationsBefore :many
-- The latest-run observations about a project (linked to it or to one of its
-- tasks or blockers) from the limit most recent entries written before the
-- given one. Newest entry first, then in text order; one row per observation,
-- with its strongest link's confidence. IDs grow with created_at.
SELECT o.id, o.entry_id, o.extraction_run_id, o.kind, o.summary, o.span_start, o.span_end, o.created_at,
       e.body AS entry_body, e.created_at AS entry_created_at,
       CAST(MAX(l.confidence) AS REAL) AS confidence
FROM observations AS o
JOIN entries AS e ON e.id = o.entry_id
JOIN observation_links AS l ON l.observation_id = o.id
WHERE (
      (l.target_type = 'project' AND l.target_id = sqlc.arg(project_id))
   OR (l.target_type = 'task' AND l.target_id IN (SELECT t.id FROM tasks AS t WHERE t.project_id = sqlc.arg(project_id)))
   OR (l.target_type = 'blocker' AND l.target_id IN (SELECT b.id FROM blockers AS b WHERE b.project_id = sqlc.arg(project_id)))
  )
  AND o.extraction_run_id = (SELECT MAX(o2.extraction_run_id) FROM observations AS o2 WHERE o2.entry_id = o.entry_id)
  AND o.entry_id IN (
        SELECT o3.entry_id FROM observations AS o3
        JOIN observation_links AS l3 ON l3.observation_id = o3.id
        WHERE o3.entry_id < sqlc.arg(before_id)
          AND o3.extraction_run_id = (SELECT MAX(o4.extraction_run_id) FROM observations AS o4 WHERE o4.entry_id = o3.entry_id)
          AND (
                (l3.target_type = 'project' AND l3.target_id = sqlc.arg(project_id))
             OR (l3.target_type = 'task' AND l3.target_id IN (SELECT t.id FROM tasks AS t WHERE t.project_id = sqlc.arg(project_id)))
             OR (l3.target_type = 'blocker' AND l3.target_id IN (SELECT b.id FROM blockers AS b WHERE b.project_id = sqlc.arg(project_id)))
          )
        GROUP BY o3.entry_id
        ORDER BY o3.entry_id DESC
        LIMIT sqlc.arg(limit)
      )
GROUP BY o.id
ORDER BY e.created_at DESC, e.id DESC, o.span_start, o.id;
