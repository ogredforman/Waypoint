package changeset

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/ogredforman/waypoint/internal/domain"
)

func changesetCount(t *testing.T, f fixture) int {
	t.Helper()
	var n int
	if err := f.db.QueryRowContext(context.Background(), `SELECT count(*) FROM changesets`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestApplyManual(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	res, err := NewApplier(f.s, 3).ApplyManual(ctx,
		UpdateTask{TaskID: ID(f.a4), Title: ptr("Write the intro"), Size: To(domain.SizeS)},
		SetTaskStatus{TaskID: ID(f.a4), Status: domain.TaskInProgress},
		CreateTask{ProjectID: ID(f.projA), Title: "Outline"},
		ResolveBlocker{BlockerID: ID(f.bA)},
	)
	if err != nil {
		t.Fatal(err)
	}
	cs := res.Changeset
	if cs.Source != domain.SourceManual || cs.ExtractionRunID != nil || cs.Status != domain.ChangesetApplied || cs.ResolvedAt == nil {
		t.Errorf("changeset %+v, want manual, applied, and resolved", cs)
	}
	if res.SplitOff != nil || res.HeldByCap != nil {
		t.Errorf("split off %v, held %v; want none", res.SplitOff, res.HeldByCap)
	}
	ops := must(f.s.ListChangeOps(ctx, cs.ID))(t)
	var types []domain.OpType
	for _, op := range ops {
		types = append(types, op.OpType)
	}
	if want := []domain.OpType{domain.OpUpdateTask, domain.OpSetTaskStatus, domain.OpCreateTask, domain.OpResolveBlocker}; !slices.Equal(types, want) {
		t.Errorf("op types %v, want %v", types, want)
	}
	if got := opStatuses(t, f, cs.ID); !slices.Equal(got, slices.Repeat([]domain.ChangeOpStatus{domain.OpApplied}, 4)) {
		t.Errorf("op statuses %v", got)
	}

	tk := must(f.s.GetTask(ctx, f.a4))(t)
	if tk.Title != "Write the intro" || tk.Size == nil || *tk.Size != domain.SizeS || tk.Status != domain.TaskInProgress {
		t.Errorf("task %+v", tk)
	}
	created := must(f.s.GetTask(ctx, res.Created[3]))(t)
	if created.Title != "Outline" || created.OriginEntryID != nil {
		t.Errorf("created task %+v, want no origin entry", created)
	}
	b := must(f.s.GetBlocker(ctx, f.bA))(t)
	if b.Status != domain.BlockerResolved || b.ResolvedEntryID != nil {
		t.Errorf("blocker %+v, want resolved with no entry", b)
	}
}

func TestApplyManualCreatesActiveProject(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	res, err := NewApplier(f.s, 3).ApplyManual(ctx,
		CreateProject{Title: "Garden"},
		UpdateProject{ProjectID: Temp(1), Status: ptr(domain.ProjectActive)},
	)
	if err != nil {
		t.Fatal(err)
	}
	if p := must(f.s.GetProject(ctx, res.Created[1]))(t); p.Title != "Garden" || p.Status != domain.ProjectActive {
		t.Errorf("project %+v, want active", p)
	}
}

func TestApplyManualDeleteRecordsRow(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	res, err := NewApplier(f.s, 3).ApplyManual(ctx, DeleteTask{TaskID: ID(f.a4)})
	if err != nil {
		t.Fatal(err)
	}
	applied := must(f.s.ListChangeOps(ctx, res.Changeset.ID))(t)[0].AppliedPayload
	var got struct {
		Deleted struct {
			ID    int64
			Title string
		}
	}
	if err := json.Unmarshal(applied, &got); err != nil || got.Deleted.ID != f.a4 || got.Deleted.Title != "t" {
		t.Errorf("applied payload %s, want the deleted task", applied)
	}
}

// TestApplyManualStoresNothingOnFailure checks that a failed manual edit
// leaves neither changed rows nor a pending changeset behind.
func TestApplyManualStoresNothingOnFailure(t *testing.T) {
	ctx := context.Background()
	for _, tt := range []struct {
		name  string
		cap   int
		ops   []Payload
		check func(t *testing.T, err error)
	}{
		{"no ops", 3, nil, func(t *testing.T, err error) {
			if got := fields(t, err); !slices.Equal(got, []string{"ops"}) {
				t.Errorf("problem fields %v", got)
			}
		}},
		{"nil op", 3, []Payload{CreateTask{ProjectID: ID(1), Title: "x"}, nil}, func(t *testing.T, err error) {
			if got := fields(t, err); !slices.Equal(got, []string{"ops[2]"}) {
				t.Errorf("problem fields %v", got)
			}
		}},
		{"invalid payload", 3, []Payload{
			CreateTask{ProjectID: ID(1), Title: "x"},
			UpdateTask{TaskID: ID(1), Title: ptr(" ")},
		}, func(t *testing.T, err error) {
			if got := fields(t, err); !slices.Equal(got, []string{"ops[2].title"}) {
				t.Errorf("problem fields %v", got)
			}
		}},
		{"missing row", 3, []Payload{
			CreateTask{ProjectID: ID(1), Title: "x"},
			SetTaskStatus{TaskID: ID(999), Status: domain.TaskDone},
		}, func(t *testing.T, err error) {
			var oe *OpError
			if !errors.As(err, &oe) || oe.Position != 2 {
				t.Errorf("got %v, want an OpError at 2", err)
			}
		}},
		{"over the cap", 2, []Payload{
			CreateTask{ProjectID: ID(1), Title: "x"},
			CreateProject{Title: "C"},
			UpdateProject{ProjectID: Temp(2), Status: ptr(domain.ProjectActive)},
		}, func(t *testing.T, err error) {
			var ce *domain.ActiveCapError
			if !errors.As(err, &ce) || ce.Cap != 2 || ce.Active != 2 {
				t.Errorf("got %v, want ActiveCapError 2 of 2", err)
			}
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			// The fixture's project A is the first project, so ID(1) points at it.
			if f.projA != 1 {
				t.Fatalf("project A has ID %d, want 1", f.projA)
			}
			_, err := NewApplier(f.s, tt.cap).ApplyManual(ctx, tt.ops...)
			if err == nil {
				t.Fatal("got no error")
			}
			tt.check(t, err)
			if n := changesetCount(t, f); n != 0 {
				t.Errorf("%d changesets stored, want 0", n)
			}
			if n := taskCount(t, f, f.projA); n != 4 {
				t.Errorf("project A has %d tasks, want 4", n)
			}
			if ps := must(f.s.ListProjects(ctx))(t); len(ps) != 2 {
				t.Errorf("%d projects, want 2", len(ps))
			}
		})
	}
}
