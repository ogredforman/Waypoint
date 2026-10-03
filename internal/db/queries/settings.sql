-- name: GetSettings :one
SELECT * FROM settings
WHERE id = 1;

-- name: InsertSettingsIfMissing :exec
-- Seeds the settings row; an existing row is left as it is.
INSERT INTO settings (id, active_project_cap, questions_per_entry, interface_font, diary_font, created_at, updated_at)
VALUES (1, sqlc.arg(active_project_cap), sqlc.arg(questions_per_entry), sqlc.arg(interface_font), sqlc.arg(diary_font), sqlc.arg(now), sqlc.arg(now))
ON CONFLICT (id) DO NOTHING;

-- name: UpsertSettings :one
INSERT INTO settings (id, active_project_cap, questions_per_entry, interface_font, diary_font, created_at, updated_at)
VALUES (1, sqlc.arg(active_project_cap), sqlc.arg(questions_per_entry), sqlc.arg(interface_font), sqlc.arg(diary_font), sqlc.arg(now), sqlc.arg(now))
ON CONFLICT (id) DO UPDATE
SET active_project_cap = excluded.active_project_cap,
    questions_per_entry = excluded.questions_per_entry,
    interface_font = excluded.interface_font,
    diary_font = excluded.diary_font,
    updated_at = excluded.updated_at
RETURNING *;
