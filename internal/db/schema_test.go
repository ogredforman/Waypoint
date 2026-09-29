package db

import (
	"context"
	"database/sql"
	"slices"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"
)

var coreTables = []string{
	"blockers", "briefings", "change_ops", "changesets", "entries", "extraction_runs",
	"follow_up_questions", "ideas", "milestones", "observation_links", "observations",
	"projects", "task_dependencies", "tasks",
}

// migrated returns a temp DB with the embedded migrations applied.
func migrated(t *testing.T) *sql.DB {
	t.Helper()
	fsys, err := Migrations()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB := openTemp(t)
	if _, _, err := Migrate(context.Background(), sqlDB, fsys); err != nil {
		t.Fatal(err)
	}
	return sqlDB
}

func exec(t *testing.T, sqlDB *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := sqlDB.ExecContext(context.Background(), query, args...); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

func appTables(t *testing.T, sqlDB *sql.DB) []string {
	t.Helper()
	rows, err := sqlDB.QueryContext(context.Background(), `
		SELECT name FROM sqlite_master
		WHERE type = 'table' AND name NOT LIKE 'sqlite_%' AND name <> 'goose_db_version'
		ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		names = append(names, n)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return names
}

// seed inserts one row into every core table, linked the way the pipeline and
// applier will link them. Every row has id 1.
func seed(t *testing.T, sqlDB *sql.DB) {
	t.Helper()
	for _, q := range []string{
		`INSERT INTO projects (id, title, status, created_at, updated_at) VALUES (1, 'Build Waypoint', 'active', 1, 1)`,
		`INSERT INTO milestones (id, project_id, title, status, position, created_at, updated_at) VALUES (1, 1, 'MVP', 'open', 0, 1, 1)`,
		`INSERT INTO entries (id, body, project_id, created_at) VALUES (1, 'todo: write the schema', 1, 1)`,
		`INSERT INTO extraction_runs (id, entry_id, provider, input_snapshot, raw_output, status, latency_ms, created_at, updated_at)
			VALUES (1, 1, 'mock', '{}', '{"ops":[]}', 'succeeded', 12, 1, 1)`,
		`INSERT INTO follow_up_questions (id, entry_id, extraction_run_id, text, status, created_at, updated_at)
			VALUES (1, 1, 1, 'What''s the next step?', 'open', 1, 1)`,
		`INSERT INTO entries (id, body, reply_to_question_id, created_at) VALUES (2, 'Write the migration.', 1, 2)`,
		`UPDATE follow_up_questions SET status = 'answered', answer_entry_id = 2 WHERE id = 1`,
		`INSERT INTO tasks (id, project_id, milestone_id, title, status, size, energy, position, origin_entry_id, created_at, updated_at)
			VALUES (1, 1, 1, 'Write the schema', 'todo', 's', 'deep', 0, 1, 1, 1)`,
		`INSERT INTO tasks (id, project_id, parent_task_id, title, status, position, created_at, updated_at)
			VALUES (2, 1, 1, 'Add tests', 'todo', 1, 1, 1)`,
		`INSERT INTO task_dependencies (id, task_id, blocked_by_task_id, created_at) VALUES (1, 2, 1, 1)`,
		`INSERT INTO observations (id, entry_id, extraction_run_id, kind, summary, span_start, span_end, created_at)
			VALUES (1, 1, 1, 'progress', 'Wants to write the schema', 0, 22, 1)`,
		`INSERT INTO observation_links (id, observation_id, target_type, target_id, confidence, created_at)
			VALUES (1, 1, 'task', 1, 0.9, 1)`,
		`INSERT INTO blockers (id, project_id, task_id, description, status, origin_entry_id, created_at, updated_at)
			VALUES (1, 1, 1, 'Unsure about enums', 'open', 1, 1, 1)`,
		`INSERT INTO ideas (id, title, status, created_at, updated_at) VALUES (1, 'Voice entry', 'parked', 1, 1)`,
		`INSERT INTO changesets (id, source, extraction_run_id, status, created_at, updated_at)
			VALUES (1, 'extraction', 1, 'pending', 1, 1)`,
		`INSERT INTO changesets (id, source, status, created_at, updated_at) VALUES (2, 'manual', 'applied', 1, 1)`,
		`INSERT INTO change_ops (id, changeset_id, position, op_type, payload, rationale, confidence, status, created_at, updated_at)
			VALUES (1, 1, 0, 'create_task', '{"project_id":1,"title":"Write the schema"}', 'todo: line', 1, 'pending', 1, 1)`,
		`INSERT INTO change_ops (id, changeset_id, position, op_type, payload, status, created_at, updated_at)
			VALUES (2, 2, 0, 'set_task_status', '{"task_id":1,"status":"done"}', 'applied', 1, 1)`,
		`INSERT INTO briefings (id, project_id, next_task_id, source, generated_at, created_at, updated_at)
			VALUES (1, 1, 1, 'heuristic', 1, 1, 1)`,
	} {
		exec(t, sqlDB, q)
	}
}

func TestCoreSchema(t *testing.T) {
	ctx := context.Background()
	sqlDB := migrated(t)

	var version int64
	if err := sqlDB.QueryRowContext(ctx, "SELECT max(version_id) FROM goose_db_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 1 {
		t.Errorf("schema version %d, want 1", version)
	}
	if got := appTables(t, sqlDB); !slices.Equal(got, coreTables) {
		t.Errorf("tables %v, want %v", got, coreTables)
	}

	seed(t, sqlDB)

	rows, err := sqlDB.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Error("foreign_key_check reports violations after seeding")
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}

func TestCoreSchemaRejects(t *testing.T) {
	const (
		check     = "CHECK constraint failed"
		fk        = "FOREIGN KEY constraint failed"
		unique    = "UNIQUE constraint failed"
		immutable = "entries.body is immutable"
	)
	sqlDB := migrated(t)
	seed(t, sqlDB)

	// Every case is rejected, so the cases can't affect each other.
	for _, tt := range []struct {
		name  string
		want  string // substring of the expected error
		query string
	}{
		{"entry body update", immutable, `UPDATE entries SET body = 'rewritten' WHERE id = 1`},
		{"project status", check, `INSERT INTO projects (title, status, created_at, updated_at) VALUES ('x', 'archived', 1, 1)`},
		{"milestone status", check, `INSERT INTO milestones (project_id, title, status, position, created_at, updated_at) VALUES (1, 'x', 'late', 0, 1, 1)`},
		{"task status", check, `INSERT INTO tasks (project_id, title, status, position, created_at, updated_at) VALUES (1, 'x', 'overdue', 0, 1, 1)`},
		{"task size", check, `INSERT INTO tasks (project_id, title, status, size, position, created_at, updated_at) VALUES (1, 'x', 'todo', 'xl', 0, 1, 1)`},
		{"task energy", check, `INSERT INTO tasks (project_id, title, status, energy, position, created_at, updated_at) VALUES (1, 'x', 'todo', 'high', 0, 1, 1)`},
		{"task missing project", fk, `INSERT INTO tasks (project_id, title, status, position, created_at, updated_at) VALUES (99, 'x', 'todo', 0, 1, 1)`},
		{"task own parent", check, `UPDATE tasks SET parent_task_id = 1 WHERE id = 1`},
		{"self dependency", check, `INSERT INTO task_dependencies (task_id, blocked_by_task_id, created_at) VALUES (1, 1, 1)`},
		{"duplicate dependency", unique, `INSERT INTO task_dependencies (task_id, blocked_by_task_id, created_at) VALUES (2, 1, 1)`},
		{"entry missing question", fk, `INSERT INTO entries (body, reply_to_question_id, created_at) VALUES ('x', 99, 1)`},
		{"run status", check, `INSERT INTO extraction_runs (entry_id, provider, status, created_at, updated_at) VALUES (1, 'mock', 'done', 1, 1)`},
		{"run invalid JSON", check, `INSERT INTO extraction_runs (entry_id, provider, raw_output, status, created_at, updated_at) VALUES (1, 'mock', '{oops', 'failed', 1, 1)`},
		{"question status", check, `INSERT INTO follow_up_questions (entry_id, extraction_run_id, text, status, created_at, updated_at) VALUES (1, 1, 'x', 'ignored', 1, 1)`},
		{"observation kind", check, `INSERT INTO observations (entry_id, extraction_run_id, kind, summary, span_start, span_end, created_at) VALUES (1, 1, 'rant', 'x', 0, 1, 1)`},
		{"observation span reversed", check, `INSERT INTO observations (entry_id, extraction_run_id, kind, summary, span_start, span_end, created_at) VALUES (1, 1, 'note', 'x', 5, 2, 1)`},
		{"link target type", check, `INSERT INTO observation_links (observation_id, target_type, target_id, confidence, created_at) VALUES (1, 'entry', 1, 0.5, 1)`},
		{"link confidence", check, `INSERT INTO observation_links (observation_id, target_type, target_id, confidence, created_at) VALUES (1, 'project', 1, 1.5, 1)`},
		{"blocker status", check, `INSERT INTO blockers (project_id, description, status, created_at, updated_at) VALUES (1, 'x', 'stuck', 1, 1)`},
		{"idea status", check, `INSERT INTO ideas (title, status, created_at, updated_at) VALUES ('x', 'active', 1, 1)`},
		{"extraction changeset without run", check, `INSERT INTO changesets (source, status, created_at, updated_at) VALUES ('extraction', 'pending', 1, 1)`},
		{"manual changeset with run", check, `INSERT INTO changesets (source, extraction_run_id, status, created_at, updated_at) VALUES ('manual', 1, 'pending', 1, 1)`},
		{"second changeset for run", unique, `INSERT INTO changesets (source, extraction_run_id, status, created_at, updated_at) VALUES ('extraction', 1, 'pending', 1, 1)`},
		{"changeset status", check, `INSERT INTO changesets (source, status, created_at, updated_at) VALUES ('manual', 'done', 1, 1)`},
		{"op type", check, `INSERT INTO change_ops (changeset_id, position, op_type, payload, status, created_at, updated_at) VALUES (1, 1, 'delete_task', '{}', 'pending', 1, 1)`},
		{"op invalid JSON", check, `INSERT INTO change_ops (changeset_id, position, op_type, payload, status, created_at, updated_at) VALUES (1, 1, 'park_idea', 'nope', 'pending', 1, 1)`},
		{"op duplicate position", unique, `INSERT INTO change_ops (changeset_id, position, op_type, payload, status, created_at, updated_at) VALUES (1, 0, 'park_idea', '{}', 'pending', 1, 1)`},
		{"op status", check, `INSERT INTO change_ops (changeset_id, position, op_type, payload, status, created_at, updated_at) VALUES (1, 1, 'park_idea', '{}', 'maybe', 1, 1)`},
		{"second briefing for project", unique, `INSERT INTO briefings (project_id, source, generated_at, created_at, updated_at) VALUES (1, 'heuristic', 1, 1, 1)`},
		{"briefing source", check, `INSERT INTO briefings (project_id, source, generated_at, created_at, updated_at) VALUES (1, 'guess', 1, 1, 1)`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := sqlDB.ExecContext(context.Background(), tt.query)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("got error %v, want one containing %q", err, tt.want)
			}
		})
	}
}

func TestCoreSchemaDownAndUp(t *testing.T) {
	ctx := context.Background()
	sqlDB := migrated(t)
	seed(t, sqlDB)

	fsys, err := Migrations()
	if err != nil {
		t.Fatal(err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, sqlDB, fsys)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.DownTo(ctx, 0); err != nil {
		t.Fatal(err)
	}
	if got := appTables(t, sqlDB); len(got) != 0 {
		t.Errorf("tables left after down: %v", got)
	}

	if _, _, err := Migrate(ctx, sqlDB, fsys); err != nil {
		t.Fatal(err)
	}
	if got := appTables(t, sqlDB); !slices.Equal(got, coreTables) {
		t.Errorf("tables after re-up %v, want %v", got, coreTables)
	}
}
