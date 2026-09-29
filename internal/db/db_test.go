package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"slices"
	"testing"
	"testing/fstest"
)

func openTemp(t *testing.T) *sql.DB {
	t.Helper()
	sqlDB, err := Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return sqlDB
}

func TestOpenPragmasOnEveryConnection(t *testing.T) {
	ctx := context.Background()
	sqlDB := openTemp(t)

	// Hold two connections at once so the pool must hand out distinct ones.
	var conns []*sql.Conn
	for range 2 {
		c, err := sqlDB.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = c.Close() })
		conns = append(conns, c)
	}

	for i, c := range conns {
		for _, tt := range []struct {
			pragma string
			want   string
		}{
			{"journal_mode", "wal"},
			{"foreign_keys", "1"},
			{"busy_timeout", "5000"},
			{"synchronous", "1"}, // NORMAL
		} {
			var got string
			if err := c.QueryRowContext(ctx, "PRAGMA "+tt.pragma).Scan(&got); err != nil {
				t.Fatalf("conn %d: PRAGMA %s: %v", i, tt.pragma, err)
			}
			if got != tt.want {
				t.Errorf("conn %d: %s = %q, want %q", i, tt.pragma, got, tt.want)
			}
		}
	}
}

func TestOpenEnforcesForeignKeys(t *testing.T) {
	ctx := context.Background()
	sqlDB := openTemp(t)
	for _, stmt := range []string{
		"CREATE TABLE parent (id INTEGER PRIMARY KEY)",
		"CREATE TABLE child (id INTEGER PRIMARY KEY, parent_id INTEGER NOT NULL REFERENCES parent(id))",
	} {
		if _, err := sqlDB.ExecContext(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := sqlDB.ExecContext(ctx, "INSERT INTO child (parent_id) VALUES (42)"); err == nil {
		t.Error("insert with a missing parent succeeded; foreign keys are not enforced")
	}
}

func TestOpenBadPath(t *testing.T) {
	missingDir := filepath.Join(t.TempDir(), "no", "such", "dir", "x.db")
	if _, err := Open(context.Background(), missingDir); err == nil {
		t.Error("want error opening a DB in a missing directory")
	}
}

func TestOpenRejectsQuestionMark(t *testing.T) {
	if _, err := Open(context.Background(), filepath.Join(t.TempDir(), "a?b.db")); err == nil {
		t.Error("want error for a path containing '?'")
	}
}

func migration(up, down string) *fstest.MapFile {
	return &fstest.MapFile{Data: []byte("-- +goose Up\n" + up + "\n-- +goose Down\n" + down + "\n")}
}

func tableExists(t *testing.T, sqlDB *sql.DB, name string) bool {
	t.Helper()
	var n int
	err := sqlDB.QueryRowContext(context.Background(),
		"SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", name).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n > 0
}

func TestMigrate(t *testing.T) {
	ctx := context.Background()
	sqlDB := openTemp(t)
	fsys := fstest.MapFS{
		"00001_a.sql": migration("CREATE TABLE a (id INTEGER PRIMARY KEY);", "DROP TABLE a;"),
		"00002_b.sql": migration("CREATE TABLE b (id INTEGER PRIMARY KEY);", "DROP TABLE b;"),
		"README.md":   &fstest.MapFile{Data: []byte("ignored")},
	}

	version, applied, err := Migrate(ctx, sqlDB, fsys)
	if err != nil {
		t.Fatal(err)
	}
	if version != 2 || !slices.Equal(applied, []int64{1, 2}) {
		t.Errorf("first run: version %d applied %v, want 2 [1 2]", version, applied)
	}
	if !tableExists(t, sqlDB, "a") || !tableExists(t, sqlDB, "b") {
		t.Error("tables a and b should exist")
	}

	// Second run is a no-op.
	version, applied, err = Migrate(ctx, sqlDB, fsys)
	if err != nil {
		t.Fatal(err)
	}
	if version != 2 || len(applied) != 0 {
		t.Errorf("second run: version %d applied %v, want 2 []", version, applied)
	}

	// A failing migration rolls back entirely and leaves the version alone.
	fsys["00003_broken.sql"] = migration(
		"CREATE TABLE c (id INTEGER PRIMARY KEY);\nINSERT INTO no_such_table VALUES (1);",
		"DROP TABLE c;")
	if _, _, err := Migrate(ctx, sqlDB, fsys); err == nil {
		t.Fatal("want error from broken migration")
	}
	if tableExists(t, sqlDB, "c") {
		t.Error("table c exists; the failed migration was not rolled back")
	}
	delete(fsys, "00003_broken.sql")
	if version, _, err := Migrate(ctx, sqlDB, fsys); err != nil || version != 2 {
		t.Errorf("after failure: version %d err %v, want 2 nil", version, err)
	}

	// With a good and a broken migration pending together, the good one stays
	// applied and is reported.
	fsys["00003_c.sql"] = migration("CREATE TABLE c (id INTEGER PRIMARY KEY);", "DROP TABLE c;")
	fsys["00004_broken.sql"] = migration("INSERT INTO no_such_table VALUES (1);", "")
	_, applied, err = Migrate(ctx, sqlDB, fsys)
	if err == nil {
		t.Fatal("want error from broken migration")
	}
	if !slices.Equal(applied, []int64{3}) {
		t.Errorf("partial failure: applied %v, want [3]", applied)
	}
	if !tableExists(t, sqlDB, "c") {
		t.Error("table c should exist after migration 3 applied")
	}
	delete(fsys, "00004_broken.sql")
	if version, _, err := Migrate(ctx, sqlDB, fsys); err != nil || version != 3 {
		t.Errorf("after partial failure: version %d err %v, want 3 nil", version, err)
	}
}

func TestMigrateNoMigrations(t *testing.T) {
	sqlDB := openTemp(t)
	version, applied, err := Migrate(context.Background(), sqlDB, fstest.MapFS{})
	if err != nil || version != 0 || len(applied) != 0 {
		t.Errorf("got %d %v %v, want 0 [] nil", version, applied, err)
	}
}

func TestMigrateEmbedded(t *testing.T) {
	fsys, err := Migrations()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := Migrate(context.Background(), openTemp(t), fsys); err != nil {
		t.Fatal(err)
	}
}
