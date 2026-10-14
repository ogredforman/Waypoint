package store

import (
	"context"
	"slices"
	"testing"

	"github.com/ogredforman/waypoint/internal/domain"
)

func TestExportTablesCoverSchema(t *testing.T) {
	ctx := context.Background()
	_, sqlDB, _ := newTestStore(t)

	rows, err := sqlDB.QueryContext(ctx, `SELECT name FROM sqlite_master
		WHERE type = 'table' AND name NOT LIKE 'sqlite_%' AND name <> 'goose_db_version'`)
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, name)
	}
	_ = rows.Close()
	listed := ExportTables()
	if got, want := slices.Sorted(slices.Values(listed)), slices.Sorted(slices.Values(tables)); !slices.Equal(got, want) {
		t.Fatalf("ExportTables %v, schema has %v", got, want)
	}

	// Each table comes after the tables it references, so an import can
	// insert them in order; replies to questions are the one cycle.
	for i, table := range listed {
		refs, err := sqlDB.QueryContext(ctx, `SELECT "table" FROM pragma_foreign_key_list(?1)
			WHERE NOT (?1 = 'entries' AND "from" = 'reply_to_question_id')`, table)
		if err != nil {
			t.Fatal(err)
		}
		for refs.Next() {
			var parent string
			if err := refs.Scan(&parent); err != nil {
				t.Fatal(err)
			}
			if parent != table && !slices.Contains(listed[:i], parent) {
				t.Errorf("%s references %s, which isn't listed before it", table, parent)
			}
		}
		_ = refs.Close()
	}
}

func TestExportTable(t *testing.T) {
	ctx := context.Background()
	s, _, _ := newTestStore(t)
	p := seedProject(t, s)
	seedTask(t, s, domain.Task{ProjectID: p.ID, Title: "Write the export", Status: domain.TaskTodo})
	seedEntry(t, s, "Started on the export.")

	err := s.InReadTx(ctx, func(tx *ReadTx) error {
		tasks, err := tx.ExportTable(ctx, "tasks")
		if err != nil {
			return err
		}
		if len(tasks) != 1 {
			t.Fatalf("tasks: %d rows, want 1", len(tasks))
		}
		task := tasks[0]
		if task["title"] != "Write the export" || task["project_id"] != p.ID || task["status"] != "todo" {
			t.Errorf("task row %v", task)
		}
		if v, ok := task["milestone_id"]; !ok || v != nil {
			t.Errorf("milestone_id = %v (present %v), want a nil value", v, ok)
		}
		if task["created_at"] != t0.UnixMilli() {
			t.Errorf("created_at = %v, want %d", task["created_at"], t0.UnixMilli())
		}

		entries, err := tx.ExportTable(ctx, "entries")
		if err != nil {
			return err
		}
		if len(entries) != 1 || entries[0]["body"] != "Started on the export." {
			t.Errorf("entries %v", entries)
		}

		ideas, err := tx.ExportTable(ctx, "ideas")
		if err != nil {
			return err
		}
		if ideas == nil || len(ideas) != 0 {
			t.Errorf("ideas = %#v, want an empty, non-nil list", ideas)
		}

		if _, err := tx.ExportTable(ctx, "goose_db_version"); err == nil {
			t.Error("exporting an unlisted table: no error")
		}

		v, err := tx.SchemaVersion(ctx)
		if err != nil {
			return err
		}
		if v < 13 {
			t.Errorf("schema version %d, want at least 13", v)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
