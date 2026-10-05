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
	"blockers", "briefing_recap_points", "briefings", "change_ops", "changesets", "entries", "extraction_runs",
	"follow_up_questions", "ideas", "milestones", "observation_links", "observations",
	"pending_observation_links", "profile", "projects", "settings", "task_dependencies", "tasks",
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
			VALUES (1, 1, 1, 'create_task', '{"project_id":1,"title":"Write the schema"}', 'todo: line', 1, 'pending', 1, 1)`,
		`INSERT INTO change_ops (id, changeset_id, position, op_type, payload, status, created_at, updated_at)
			VALUES (2, 2, 1, 'set_task_status', '{"task_id":1,"status":"done"}', 'applied', 1, 1)`,
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
	if version != 13 {
		t.Errorf("schema version %d, want 13", version)
	}
	if got := appTables(t, sqlDB); !slices.Equal(got, coreTables) {
		t.Errorf("tables %v, want %v", got, coreTables)
	}

	seed(t, sqlDB)
	// Not in seed: the rebuild tests seed schemas from before this table.
	exec(t, sqlDB, `INSERT INTO pending_observation_links (id, observation_id, change_op_id, target_type, confidence, created_at)
		VALUES (1, 1, 1, 'task', 1, 1)`)

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
		{"project status", check, `INSERT INTO projects (title, status, created_at, updated_at) VALUES ('x', 'abandoned', 1, 1)`},
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
		{"pending link missing op", fk, `INSERT INTO pending_observation_links (observation_id, change_op_id, target_type, confidence, created_at) VALUES (1, 99, 'task', 0.5, 1)`},
		{"pending link target type", check, `INSERT INTO pending_observation_links (observation_id, change_op_id, target_type, confidence, created_at) VALUES (1, 2, 'entry', 0.5, 1)`},
		{"pending link duplicate", unique, `INSERT INTO pending_observation_links (observation_id, change_op_id, target_type, confidence, created_at) VALUES (1, 1, 'task', 0.5, 1), (1, 1, 'task', 0.5, 1)`},
		{"blocker status", check, `INSERT INTO blockers (project_id, description, status, created_at, updated_at) VALUES (1, 'x', 'stuck', 1, 1)`},
		{"settings second row", check, `INSERT INTO settings (id, active_project_cap, questions_per_entry, created_at, updated_at) VALUES (2, 3, 2, 1, 1)`},
		{"settings cap below 1", check, `INSERT INTO settings (id, active_project_cap, questions_per_entry, created_at, updated_at) VALUES (1, 0, 2, 1, 1)`},
		{"settings questions above 2", check, `INSERT INTO settings (id, active_project_cap, questions_per_entry, created_at, updated_at) VALUES (1, 3, 3, 1, 1)`},
		{"settings questions negative", check, `INSERT INTO settings (id, active_project_cap, questions_per_entry, created_at, updated_at) VALUES (1, 3, -1, 1, 1)`},
		{"settings interface font", check, `INSERT INTO settings (id, active_project_cap, questions_per_entry, interface_font, created_at, updated_at) VALUES (1, 3, 2, 'literata', 1, 1)`},
		{"settings theme", check, `INSERT INTO settings (id, active_project_cap, questions_per_entry, theme, created_at, updated_at) VALUES (1, 3, 2, 'solarized', 1, 1)`},
		{"settings diary font", check, `INSERT INTO settings (id, active_project_cap, questions_per_entry, diary_font, created_at, updated_at) VALUES (1, 3, 2, 'papyrus', 1, 1)`},
		{"profile second row", check, `INSERT INTO profile (id, name, created_at, updated_at) VALUES (2, 'x', 1, 1)`},
		{"profile blank name", check, `UPDATE profile SET name = '  ' WHERE id = 1`},
		{"idea status", check, `INSERT INTO ideas (title, status, created_at, updated_at) VALUES ('x', 'active', 1, 1)`},
		{"extraction changeset without run", check, `INSERT INTO changesets (source, status, created_at, updated_at) VALUES ('extraction', 'pending', 1, 1)`},
		{"manual changeset with run", check, `INSERT INTO changesets (source, extraction_run_id, status, created_at, updated_at) VALUES ('manual', 1, 'pending', 1, 1)`},
		{"changeset missing parent", fk, `INSERT INTO changesets (source, parent_changeset_id, status, created_at, updated_at) VALUES ('manual', 99, 'pending', 1, 1)`},
		{"changeset status", check, `INSERT INTO changesets (source, status, created_at, updated_at) VALUES ('manual', 'done', 1, 1)`},
		{"op type", check, `INSERT INTO change_ops (changeset_id, position, op_type, payload, status, created_at, updated_at) VALUES (1, 2, 'move_project', '{}', 'pending', 1, 1)`},
		{"op invalid JSON", check, `INSERT INTO change_ops (changeset_id, position, op_type, payload, status, created_at, updated_at) VALUES (1, 2, 'park_idea', 'nope', 'pending', 1, 1)`},
		{"op duplicate position", unique, `INSERT INTO change_ops (changeset_id, position, op_type, payload, status, created_at, updated_at) VALUES (1, 1, 'park_idea', '{}', 'pending', 1, 1)`},
		{"op status", check, `INSERT INTO change_ops (changeset_id, position, op_type, payload, status, created_at, updated_at) VALUES (1, 2, 'park_idea', '{}', 'maybe', 1, 1)`},
		{"second briefing for project", unique, `INSERT INTO briefings (project_id, source, generated_at, created_at, updated_at) VALUES (1, 'heuristic', 1, 1, 1)`},
		{"briefing source", check, `INSERT INTO briefings (project_id, source, generated_at, created_at, updated_at) VALUES (1, 'guess', 1, 1, 1)`},
		{"recap point kind", check, `INSERT INTO briefing_recap_points (briefing_id, entry_id, position, kind, text) VALUES (1, 1, 1, 'mood', 'x')`},
		{"recap point missing entry", fk, `INSERT INTO briefing_recap_points (briefing_id, entry_id, position, kind, text) VALUES (1, 99, 1, 'note', 'x')`},
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

// TestChangeOpsRebuild checks that 00002 keeps existing ops, accepts the new
// op types, and that going back down keeps only the ops the old table allows.
func TestChangeOpsRebuild(t *testing.T) {
	ctx := context.Background()
	fsys, err := Migrations()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB := openTemp(t)
	provider, err := goose.NewProvider(goose.DialectSQLite3, sqlDB, fsys)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 1); err != nil {
		t.Fatal(err)
	}
	seed(t, sqlDB)

	if _, err := provider.UpTo(ctx, 2); err != nil {
		t.Fatal(err)
	}
	var payload, rationale string
	if err := sqlDB.QueryRowContext(ctx,
		`SELECT payload, rationale FROM change_ops WHERE id = 1`).Scan(&payload, &rationale); err != nil {
		t.Fatal(err)
	}
	if payload != `{"project_id":1,"title":"Write the schema"}` || rationale != "todo: line" {
		t.Errorf("op 1 after rebuild: payload %s, rationale %q", payload, rationale)
	}
	exec(t, sqlDB, `INSERT INTO change_ops (id, changeset_id, position, op_type, payload, status, created_at, updated_at)
		VALUES (3, 2, 2, 'delete_task', '{"task_id":2}', 'pending', 1, 1)`)

	if _, err := provider.DownTo(ctx, 1); err != nil {
		t.Fatal(err)
	}
	var ids []int64
	rows, err := sqlDB.QueryContext(ctx, `SELECT id FROM change_ops ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ids, []int64{1, 2}) {
		t.Errorf("ops after down %v, want [1 2]", ids)
	}
}

// noFKViolations fails the test if PRAGMA foreign_key_check reports anything.
func noFKViolations(t *testing.T, sqlDB *sql.DB) {
	t.Helper()
	rows, err := sqlDB.QueryContext(context.Background(), "PRAGMA foreign_key_check")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Error("foreign_key_check reports violations")
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}

func ids(t *testing.T, sqlDB *sql.DB, query string) []int64 {
	t.Helper()
	rows, err := sqlDB.QueryContext(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// TestChangesetSplitRebuild checks that 00003 keeps existing changesets and
// their ops, allows a split-off changeset on the same run, and that going back
// down drops split-off changesets and their ops.
func TestChangesetSplitRebuild(t *testing.T) {
	ctx := context.Background()
	fsys, err := Migrations()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB := openTemp(t)
	provider, err := goose.NewProvider(goose.DialectSQLite3, sqlDB, fsys)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 2); err != nil {
		t.Fatal(err)
	}
	seed(t, sqlDB)
	exec(t, sqlDB, `UPDATE changesets SET resolved_at = 5 WHERE id = 2`)

	if _, err := provider.UpTo(ctx, 3); err != nil {
		t.Fatal(err)
	}
	noFKViolations(t, sqlDB)
	var run int64
	var resolved int64
	if err := sqlDB.QueryRowContext(ctx, `SELECT extraction_run_id FROM changesets WHERE id = 1`).Scan(&run); err != nil || run != 1 {
		t.Errorf("changeset 1 run: %d, %v", run, err)
	}
	if err := sqlDB.QueryRowContext(ctx, `SELECT resolved_at FROM changesets WHERE id = 2`).Scan(&resolved); err != nil || resolved != 5 {
		t.Errorf("changeset 2 resolved_at: %d, %v", resolved, err)
	}
	if got := ids(t, sqlDB, `SELECT id FROM change_ops ORDER BY id`); !slices.Equal(got, []int64{1, 2}) {
		t.Errorf("ops after rebuild %v, want [1 2]", got)
	}

	exec(t, sqlDB, `INSERT INTO changesets (id, source, extraction_run_id, parent_changeset_id, status, created_at, updated_at)
		VALUES (3, 'extraction', 1, 1, 'pending', 1, 1)`)
	exec(t, sqlDB, `INSERT INTO changesets (id, source, extraction_run_id, parent_changeset_id, status, created_at, updated_at)
		VALUES (4, 'extraction', 1, 3, 'pending', 1, 1)`)
	exec(t, sqlDB, `UPDATE change_ops SET changeset_id = 4 WHERE id = 1`)

	if _, err := provider.DownTo(ctx, 2); err != nil {
		t.Fatal(err)
	}
	noFKViolations(t, sqlDB)
	if got := ids(t, sqlDB, `SELECT id FROM changesets ORDER BY id`); !slices.Equal(got, []int64{1, 2}) {
		t.Errorf("changesets after down %v, want [1 2]", got)
	}
	if got := ids(t, sqlDB, `SELECT id FROM change_ops ORDER BY id`); !slices.Equal(got, []int64{2}) {
		t.Errorf("ops after down %v, want [2]", got)
	}
}

// TestObservationPlanRebuild checks that 00004 keeps existing observations and
// their links, accepts the plan kind, and that going back down turns plan
// observations into notes, links kept.
func TestObservationPlanRebuild(t *testing.T) {
	ctx := context.Background()
	fsys, err := Migrations()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB := openTemp(t)
	provider, err := goose.NewProvider(goose.DialectSQLite3, sqlDB, fsys)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 3); err != nil {
		t.Fatal(err)
	}
	seed(t, sqlDB)

	if _, err := provider.UpTo(ctx, 4); err != nil {
		t.Fatal(err)
	}
	noFKViolations(t, sqlDB)
	var kind, summary string
	if err := sqlDB.QueryRowContext(ctx, `SELECT kind, summary FROM observations WHERE id = 1`).Scan(&kind, &summary); err != nil {
		t.Fatal(err)
	}
	if kind != "progress" || summary != "Wants to write the schema" {
		t.Errorf("observation 1 after rebuild: %s %q", kind, summary)
	}
	if got := ids(t, sqlDB, `SELECT observation_id FROM observation_links ORDER BY id`); !slices.Equal(got, []int64{1}) {
		t.Errorf("links after rebuild point at %v, want [1]", got)
	}
	exec(t, sqlDB, `INSERT INTO observations (id, entry_id, extraction_run_id, kind, summary, span_start, span_end, created_at)
		VALUES (2, 1, 1, 'plan', 'write the schema', 0, 22, 1)`)
	exec(t, sqlDB, `INSERT INTO observation_links (id, observation_id, target_type, target_id, confidence, created_at)
		VALUES (2, 2, 'project', 1, 1, 1)`)

	if _, err := provider.DownTo(ctx, 3); err != nil {
		t.Fatal(err)
	}
	noFKViolations(t, sqlDB)
	if err := sqlDB.QueryRowContext(ctx, `SELECT kind FROM observations WHERE id = 2`).Scan(&kind); err != nil || kind != "note" {
		t.Errorf("plan observation after down: %q, %v", kind, err)
	}
	if got := ids(t, sqlDB, `SELECT observation_id FROM observation_links ORDER BY id`); !slices.Equal(got, []int64{1, 2}) {
		t.Errorf("links after down point at %v, want [1 2]", got)
	}
}

// A settings row stored before the font columns gets the default fonts, and
// going down keeps the rest of it.
func TestSettingsFontsMigration(t *testing.T) {
	ctx := context.Background()
	fsys, err := Migrations()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB := openTemp(t)
	provider, err := goose.NewProvider(goose.DialectSQLite3, sqlDB, fsys)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 7); err != nil {
		t.Fatal(err)
	}
	exec(t, sqlDB, `INSERT INTO settings (id, active_project_cap, questions_per_entry, created_at, updated_at) VALUES (1, 4, 1, 1, 1)`)

	if _, err := provider.UpTo(ctx, 8); err != nil {
		t.Fatal(err)
	}
	var ui, diary string
	var limit int
	if err := sqlDB.QueryRowContext(ctx, `SELECT active_project_cap, interface_font, diary_font FROM settings`).Scan(&limit, &ui, &diary); err != nil {
		t.Fatal(err)
	}
	if limit != 4 || ui != "inter" || diary != "literata" {
		t.Errorf("after up: cap %d, fonts %q and %q; want 4, inter and literata", limit, ui, diary)
	}

	if _, err := provider.DownTo(ctx, 7); err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.QueryRowContext(ctx, `SELECT active_project_cap FROM settings`).Scan(&limit); err != nil || limit != 4 {
		t.Errorf("after down: cap %d, %v; want 4", limit, err)
	}
}

// A settings row stored before the theme column gets Midnight, the theme the
// app had before, and going down keeps the rest of it.
func TestSettingsThemeMigration(t *testing.T) {
	ctx := context.Background()
	fsys, err := Migrations()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB := openTemp(t)
	provider, err := goose.NewProvider(goose.DialectSQLite3, sqlDB, fsys)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 8); err != nil {
		t.Fatal(err)
	}
	exec(t, sqlDB, `INSERT INTO settings (id, active_project_cap, questions_per_entry, diary_font, created_at, updated_at) VALUES (1, 4, 1, 'geist', 1, 1)`)

	if _, err := provider.UpTo(ctx, 9); err != nil {
		t.Fatal(err)
	}
	var theme, diary string
	if err := sqlDB.QueryRowContext(ctx, `SELECT theme, diary_font FROM settings`).Scan(&theme, &diary); err != nil {
		t.Fatal(err)
	}
	if theme != "midnight" || diary != "geist" {
		t.Errorf("after up: theme %q, diary font %q; want midnight and geist", theme, diary)
	}

	if _, err := provider.DownTo(ctx, 8); err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.QueryRowContext(ctx, `SELECT diary_font FROM settings`).Scan(&diary); err != nil || diary != "geist" {
		t.Errorf("after down: diary font %q, %v; want geist", diary, err)
	}
}

func TestBriefingRecapMigration(t *testing.T) {
	ctx := context.Background()
	fsys, err := Migrations()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB := openTemp(t)
	provider, err := goose.NewProvider(goose.DialectSQLite3, sqlDB, fsys)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 9); err != nil {
		t.Fatal(err)
	}
	exec(t, sqlDB, `INSERT INTO projects (id, title, status, created_at, updated_at) VALUES (1, 'Build Waypoint', 'active', 1, 1)`)
	exec(t, sqlDB, `INSERT INTO briefings (project_id, recap, source, generated_at, created_at, updated_at) VALUES (1, 'Most recently: started.', 'heuristic', 1, 1, 1)`)

	// The cached briefings go; each is regenerated when next read.
	if _, err := provider.UpTo(ctx, 10); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := sqlDB.QueryRowContext(ctx, `SELECT count(*) FROM briefings`).Scan(&n); err != nil || n != 0 {
		t.Errorf("briefings after up: %d, %v; want 0", n, err)
	}
	if _, err := sqlDB.ExecContext(ctx, `SELECT recap FROM briefings`); err == nil {
		t.Error("briefings.recap still there after up")
	}
	exec(t, sqlDB, `INSERT INTO briefings (id, project_id, source, generated_at, created_at, updated_at) VALUES (1, 1, 'heuristic', 1, 1, 1)`)
	exec(t, sqlDB, `INSERT INTO entries (id, body, created_at) VALUES (1, 'words', 1)`)
	exec(t, sqlDB, `INSERT INTO briefing_recap_points (briefing_id, entry_id, position, kind, text) VALUES (1, 1, 1, 'progress', 'Started')`)

	if _, err := provider.DownTo(ctx, 9); err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.QueryRowContext(ctx, `SELECT count(*) FROM briefings`).Scan(&n); err != nil || n != 0 {
		t.Errorf("briefings after down: %d, %v; want 0", n, err)
	}
	exec(t, sqlDB, `INSERT INTO briefings (project_id, recap, source, generated_at, created_at, updated_at) VALUES (1, 'x', 'heuristic', 1, 1, 1)`)
}

// Rebuilding projects for the archived status keeps every row and every
// reference to them; going down turns archived projects into parked ones.
func TestProjectArchivedRebuild(t *testing.T) {
	ctx := context.Background()
	fsys, err := Migrations()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB := openTemp(t)
	provider, err := goose.NewProvider(goose.DialectSQLite3, sqlDB, fsys)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 10); err != nil {
		t.Fatal(err)
	}
	seed(t, sqlDB)
	exec(t, sqlDB, `UPDATE projects SET why = 'Follow through' WHERE id = 1`)

	if _, err := provider.UpTo(ctx, 11); err != nil {
		t.Fatal(err)
	}
	noFKViolations(t, sqlDB)
	var title, why, status string
	if err := sqlDB.QueryRowContext(ctx, `SELECT title, why, status FROM projects WHERE id = 1`).Scan(&title, &why, &status); err != nil {
		t.Fatal(err)
	}
	if title != "Build Waypoint" || why != "Follow through" || status != "active" {
		t.Errorf("project 1 after rebuild: %q %q %s", title, why, status)
	}
	if got := ids(t, sqlDB, `SELECT project_id FROM tasks ORDER BY id`); !slices.Equal(got, []int64{1, 1}) {
		t.Errorf("tasks after rebuild point at %v, want [1 1]", got)
	}
	exec(t, sqlDB, `INSERT INTO projects (id, title, status, created_at, updated_at) VALUES (2, 'Old idea', 'archived', 1, 1)`)
	exec(t, sqlDB, `INSERT INTO entries (id, body, project_id, created_at) VALUES (3, 'about the old idea', 2, 3)`)

	if _, err := provider.DownTo(ctx, 10); err != nil {
		t.Fatal(err)
	}
	noFKViolations(t, sqlDB)
	if err := sqlDB.QueryRowContext(ctx, `SELECT status FROM projects WHERE id = 2`).Scan(&status); err != nil || status != "parked" {
		t.Errorf("archived project after down: %q, %v", status, err)
	}
	if got := ids(t, sqlDB, `SELECT project_id FROM entries WHERE project_id IS NOT NULL ORDER BY id`); !slices.Equal(got, []int64{1, 2}) {
		t.Errorf("entries after down point at %v, want [1 2]", got)
	}
}

// TestChangeOpsDeleteProjectRebuild checks that 00012 keeps existing ops and
// the pending links on them, accepts delete_project, and that going back down
// drops delete_project ops with their pending links.
func TestChangeOpsDeleteProjectRebuild(t *testing.T) {
	ctx := context.Background()
	fsys, err := Migrations()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB := openTemp(t)
	provider, err := goose.NewProvider(goose.DialectSQLite3, sqlDB, fsys)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 11); err != nil {
		t.Fatal(err)
	}
	seed(t, sqlDB)
	exec(t, sqlDB, `INSERT INTO pending_observation_links (id, observation_id, change_op_id, target_type, confidence, created_at)
		VALUES (1, 1, 1, 'task', 1, 1)`)

	if _, err := provider.UpTo(ctx, 12); err != nil {
		t.Fatal(err)
	}
	noFKViolations(t, sqlDB)
	if got := ids(t, sqlDB, `SELECT id FROM change_ops ORDER BY id`); !slices.Equal(got, []int64{1, 2}) {
		t.Errorf("ops after rebuild %v, want [1 2]", got)
	}
	var payload string
	if err := sqlDB.QueryRowContext(ctx, `SELECT payload FROM change_ops WHERE id = 1`).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if payload != `{"project_id":1,"title":"Write the schema"}` {
		t.Errorf("op 1 after rebuild: payload %s", payload)
	}
	exec(t, sqlDB, `INSERT INTO change_ops (id, changeset_id, position, op_type, payload, status, created_at, updated_at)
		VALUES (3, 2, 2, 'delete_project', '{"project_id":1}', 'applied', 1, 1)`)
	exec(t, sqlDB, `INSERT INTO pending_observation_links (id, observation_id, change_op_id, target_type, confidence, created_at)
		VALUES (2, 1, 3, 'task', 1, 1)`)

	if _, err := provider.DownTo(ctx, 11); err != nil {
		t.Fatal(err)
	}
	noFKViolations(t, sqlDB)
	if got := ids(t, sqlDB, `SELECT id FROM change_ops ORDER BY id`); !slices.Equal(got, []int64{1, 2}) {
		t.Errorf("ops after down %v, want [1 2]", got)
	}
	if got := ids(t, sqlDB, `SELECT id FROM pending_observation_links ORDER BY id`); !slices.Equal(got, []int64{1}) {
		t.Errorf("pending links after down %v, want [1]", got)
	}
}

// TestChangeOpsMilestoneRebuild checks that 00013 keeps existing ops and the
// pending links on them, accepts the milestone ops, and that going back down
// drops milestone ops with their pending links.
func TestChangeOpsMilestoneRebuild(t *testing.T) {
	ctx := context.Background()
	fsys, err := Migrations()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB := openTemp(t)
	provider, err := goose.NewProvider(goose.DialectSQLite3, sqlDB, fsys)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 12); err != nil {
		t.Fatal(err)
	}
	seed(t, sqlDB)
	exec(t, sqlDB, `INSERT INTO pending_observation_links (id, observation_id, change_op_id, target_type, confidence, created_at)
		VALUES (1, 1, 1, 'task', 1, 1)`)

	if _, err := provider.UpTo(ctx, 13); err != nil {
		t.Fatal(err)
	}
	noFKViolations(t, sqlDB)
	if got := ids(t, sqlDB, `SELECT id FROM change_ops ORDER BY id`); !slices.Equal(got, []int64{1, 2}) {
		t.Errorf("ops after rebuild %v, want [1 2]", got)
	}
	var payload string
	if err := sqlDB.QueryRowContext(ctx, `SELECT payload FROM change_ops WHERE id = 1`).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if payload != `{"project_id":1,"title":"Write the schema"}` {
		t.Errorf("op 1 after rebuild: payload %s", payload)
	}
	exec(t, sqlDB, `INSERT INTO change_ops (id, changeset_id, position, op_type, payload, status, created_at, updated_at)
		VALUES (3, 2, 2, 'create_milestone', '{"project_id":1,"title":"MVP"}', 'applied', 1, 1),
		       (4, 2, 3, 'update_milestone', '{"milestone_id":1,"title":"Beta"}', 'applied', 1, 1),
		       (5, 2, 4, 'delete_milestone', '{"milestone_id":1}', 'applied', 1, 1)`)
	exec(t, sqlDB, `INSERT INTO pending_observation_links (id, observation_id, change_op_id, target_type, confidence, created_at)
		VALUES (2, 1, 3, 'milestone', 1, 1)`)

	if _, err := provider.DownTo(ctx, 12); err != nil {
		t.Fatal(err)
	}
	noFKViolations(t, sqlDB)
	if got := ids(t, sqlDB, `SELECT id FROM change_ops ORDER BY id`); !slices.Equal(got, []int64{1, 2}) {
		t.Errorf("ops after down %v, want [1 2]", got)
	}
	if got := ids(t, sqlDB, `SELECT id FROM pending_observation_links ORDER BY id`); !slices.Equal(got, []int64{1}) {
		t.Errorf("pending links after down %v, want [1]", got)
	}
}
