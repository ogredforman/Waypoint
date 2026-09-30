package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/ogredforman/waypoint/internal/db"
	"github.com/ogredforman/waypoint/internal/domain"
)

// clock is a settable time source for tests.
type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

var t0 = time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)

// newTestStore returns a Store over a migrated temp database, its raw
// connection for setting up rows the store doesn't write yet, and its clock.
func newTestStore(t *testing.T) (*Store, *sql.DB, *clock) {
	t.Helper()
	ctx := context.Background()
	sqlDB, err := db.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	fsys, err := db.Migrations()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.Migrate(ctx, sqlDB, fsys); err != nil {
		t.Fatal(err)
	}
	c := &clock{t: t0}
	return New(sqlDB, c.now), sqlDB, c
}

// inTx runs fn in a transaction and fails the test if it returns an error.
func inTx(t *testing.T, s *Store, fn func(tx *Tx) error) {
	t.Helper()
	if err := s.InTx(context.Background(), fn); err != nil {
		t.Fatal(err)
	}
}

func exec(t *testing.T, sqlDB *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := sqlDB.ExecContext(context.Background(), query, args...); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

func count(t *testing.T, sqlDB *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := sqlDB.QueryRowContext(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

func ptr[T any](v T) *T { return &v }

// seedProject creates an active project to hang other rows on.
func seedProject(t *testing.T, s *Store) domain.Project {
	t.Helper()
	var p domain.Project
	inTx(t, s, func(tx *Tx) (err error) {
		p, err = tx.CreateProject(context.Background(), domain.Project{Title: "Build Waypoint", Status: domain.ProjectActive})
		return err
	})
	return p
}

func seedTask(t *testing.T, s *Store, task domain.Task) domain.Task {
	t.Helper()
	inTx(t, s, func(tx *Tx) (err error) {
		task, err = tx.CreateTask(context.Background(), task)
		return err
	})
	return task
}

func seedEntry(t *testing.T, s *Store, body string) domain.Entry {
	t.Helper()
	var e domain.Entry
	inTx(t, s, func(tx *Tx) (err error) {
		e, err = tx.CreateEntry(context.Background(), domain.Entry{Body: body})
		return err
	})
	return e
}

// seedRun inserts a succeeded extraction run directly; the store only creates
// pending ones.
func seedRun(t *testing.T, sqlDB *sql.DB, entryID int64) int64 {
	t.Helper()
	res, err := sqlDB.ExecContext(context.Background(),
		`INSERT INTO extraction_runs (entry_id, provider, status, created_at, updated_at)
		 VALUES (?, 'mock', 'succeeded', 0, 0)`, entryID)
	if err != nil {
		t.Fatal(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestInTxRollsBackOnError(t *testing.T) {
	s, sqlDB, _ := newTestStore(t)
	boom := errors.New("boom")
	err := s.InTx(context.Background(), func(tx *Tx) error {
		if _, err := tx.CreateProject(context.Background(), domain.Project{Title: "Doomed", Status: domain.ProjectParked}); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("InTx error = %v, want %v", err, boom)
	}
	if n := count(t, sqlDB, "SELECT COUNT(*) FROM projects"); n != 0 {
		t.Errorf("%d projects after rollback, want 0", n)
	}
}

func TestInTxRollsBackOnPanic(t *testing.T) {
	s, sqlDB, _ := newTestStore(t)
	func() {
		defer func() {
			if recover() == nil {
				t.Error("InTx swallowed the panic")
			}
		}()
		_ = s.InTx(context.Background(), func(tx *Tx) error {
			if _, err := tx.CreateProject(context.Background(), domain.Project{Title: "Doomed", Status: domain.ProjectParked}); err != nil {
				return err
			}
			panic("boom")
		})
	}()
	if n := count(t, sqlDB, "SELECT COUNT(*) FROM projects"); n != 0 {
		t.Errorf("%d projects after rollback, want 0", n)
	}
	// The connection went back to the pool usable.
	seedProject(t, s)
}

func TestInvalidInputWritesNothing(t *testing.T) {
	s, sqlDB, _ := newTestStore(t)
	ctx := context.Background()
	p := seedProject(t, s)

	writes := map[string]func(tx *Tx) error{
		"project": func(tx *Tx) error {
			_, err := tx.CreateProject(ctx, domain.Project{Title: " ", Status: domain.ProjectActive})
			return err
		},
		"task": func(tx *Tx) error {
			_, err := tx.CreateTask(ctx, domain.Task{ProjectID: p.ID, Title: "x", Status: "later"})
			return err
		},
		"blocker": func(tx *Tx) error {
			_, err := tx.CreateBlocker(ctx, domain.Blocker{ProjectID: p.ID, Description: "x", Status: domain.BlockerOpen, ResolvedEntryID: ptr(int64(1))})
			return err
		},
		"idea": func(tx *Tx) error {
			_, err := tx.CreateIdea(ctx, domain.Idea{Title: "", Status: domain.IdeaParked})
			return err
		},
		"entry": func(tx *Tx) error {
			_, err := tx.CreateEntry(ctx, domain.Entry{Body: "\n"})
			return err
		},
		"observation": func(tx *Tx) error {
			_, err := tx.CreateObservation(ctx, domain.Observation{EntryID: 1, ExtractionRunID: 1, Kind: domain.ObservationNote, Summary: "x", SpanStart: 5, SpanEnd: 2})
			return err
		},
		"link": func(tx *Tx) error {
			_, err := tx.CreateObservationLink(ctx, domain.ObservationLink{ObservationID: 1, TargetType: domain.TargetTask, TargetID: 1, Confidence: 2})
			return err
		},
	}
	for name, write := range writes {
		t.Run(name, func(t *testing.T) {
			err := s.InTx(ctx, write)
			var verr *domain.ValidationError
			if !errors.As(err, &verr) {
				t.Errorf("error = %v, want a *domain.ValidationError", err)
			}
		})
	}
	for _, table := range []string{"projects", "tasks", "blockers", "ideas", "entries", "observations", "observation_links"} {
		want := 0
		if table == "projects" {
			want = 1 // the seeded one
		}
		if n := count(t, sqlDB, "SELECT COUNT(*) FROM "+table); n != want {
			t.Errorf("%d rows in %s, want %d", n, table, want)
		}
	}
}

func TestMissingRowsAreNotFound(t *testing.T) {
	s, _, _ := newTestStore(t)
	ctx := context.Background()
	p := seedProject(t, s)

	reads := map[string]func() error{
		"project":     func() error { _, err := s.GetProject(ctx, 999); return err },
		"task":        func() error { _, err := s.GetTask(ctx, 999); return err },
		"blocker":     func() error { _, err := s.GetBlocker(ctx, 999); return err },
		"idea":        func() error { _, err := s.GetIdea(ctx, 999); return err },
		"entry":       func() error { _, err := s.GetEntry(ctx, 999); return err },
		"observation": func() error { _, err := s.GetObservation(ctx, 999); return err },
	}
	writes := map[string]func(tx *Tx) error{
		"update project": func(tx *Tx) error {
			_, err := tx.UpdateProject(ctx, domain.Project{ID: 999, Title: "x", Status: domain.ProjectActive})
			return err
		},
		"update task": func(tx *Tx) error {
			_, err := tx.UpdateTask(ctx, domain.Task{ID: 999, ProjectID: p.ID, Title: "x", Status: domain.TaskTodo})
			return err
		},
		"update blocker": func(tx *Tx) error {
			_, err := tx.UpdateBlocker(ctx, domain.Blocker{ID: 999, ProjectID: p.ID, Description: "x", Status: domain.BlockerOpen})
			return err
		},
		"update idea": func(tx *Tx) error {
			_, err := tx.UpdateIdea(ctx, domain.Idea{ID: 999, Title: "x", Status: domain.IdeaParked})
			return err
		},
		"delete task":    func(tx *Tx) error { _, err := tx.DeleteTask(ctx, 999); return err },
		"delete blocker": func(tx *Tx) error { _, err := tx.DeleteBlocker(ctx, 999); return err },
		"delete idea":    func(tx *Tx) error { _, err := tx.DeleteIdea(ctx, 999); return err },
	}
	for name, read := range reads {
		if err := read(); !errors.Is(err, ErrNotFound) {
			t.Errorf("get %s: error = %v, want ErrNotFound", name, err)
		}
	}
	for name, write := range writes {
		if err := s.InTx(ctx, write); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s: error = %v, want ErrNotFound", name, err)
		}
	}
}

func TestDanglingReferenceFails(t *testing.T) {
	s, _, _ := newTestStore(t)
	err := s.InTx(context.Background(), func(tx *Tx) error {
		_, err := tx.CreateTask(context.Background(), domain.Task{ProjectID: 999, Title: "Orphan", Status: domain.TaskTodo})
		return err
	})
	if err == nil {
		t.Fatal("CreateTask on a missing project succeeded")
	}
}

func TestTimestampsAreUTCMilliseconds(t *testing.T) {
	s, _, c := newTestStore(t)
	c.t = time.Date(2026, 9, 29, 8, 30, 0, 123_456_789, time.FixedZone("CDT", -5*3600))
	p := seedProject(t, s)
	want := time.Date(2026, 9, 29, 13, 30, 0, 123_000_000, time.UTC)
	if !p.CreatedAt.Equal(want) || p.CreatedAt.Location() != time.UTC {
		t.Errorf("CreatedAt = %v, want %v", p.CreatedAt, want)
	}
}
