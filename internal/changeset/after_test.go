package changeset

import (
	"context"
	"slices"
	"testing"

	"github.com/ogredforman/waypoint/internal/domain"
)

// recorder is an AfterApply that keeps what it was told, and whether its
// context could be cancelled.
type recorder struct {
	got         []Result
	cancellable bool
}

func (r *recorder) Applied(ctx context.Context, res Result) {
	r.got = append(r.got, res)
	r.cancellable = r.cancellable || ctx.Done() != nil
}

func TestResultProjects(t *testing.T) {
	tests := []struct {
		name string
		ops  func(f fixture) []Payload
		want func(f fixture) []int64
	}{
		{"create task", func(f fixture) []Payload { return []Payload{CreateTask{ProjectID: ID(f.projB), Title: "x"}} },
			func(f fixture) []int64 { return []int64{f.projB} }},
		{"update task", func(f fixture) []Payload { return []Payload{UpdateTask{TaskID: ID(f.a1), Title: ptr("y")}} },
			func(f fixture) []int64 { return []int64{f.projA} }},
		{"set task status", func(f fixture) []Payload {
			return []Payload{SetTaskStatus{TaskID: ID(f.b1), Status: domain.TaskDone}}
		}, func(f fixture) []int64 { return []int64{f.projB} }},
		{"split task", func(f fixture) []Payload {
			return []Payload{SplitTask{TaskID: ID(f.a4), Subtasks: []Subtask{{Title: "s"}}}}
		}, func(f fixture) []int64 { return []int64{f.projA} }},
		{"add dependency", func(f fixture) []Payload {
			return []Payload{AddDependency{TaskID: ID(f.a4), BlockedByTaskID: ID(f.a1)}}
		}, func(f fixture) []int64 { return []int64{f.projA} }},
		{"delete task", func(f fixture) []Payload { return []Payload{DeleteTask{TaskID: ID(f.b1)}} },
			func(f fixture) []int64 { return []int64{f.projB} }},
		{"blockers", func(f fixture) []Payload {
			return []Payload{
				CreateBlocker{ProjectID: ID(f.projB), Description: "d"},
				UpdateBlocker{BlockerID: ID(f.bA), Description: ptr("e")},
			}
		}, func(f fixture) []int64 { return []int64{f.projA, f.projB} }},
		{"resolve blocker", func(f fixture) []Payload { return []Payload{ResolveBlocker{BlockerID: ID(f.bB)}} },
			func(f fixture) []int64 { return []int64{f.projB} }},
		{"delete blocker", func(f fixture) []Payload { return []Payload{DeleteBlocker{BlockerID: ID(f.bA)}} },
			func(f fixture) []int64 { return []int64{f.projA} }},
		{"projects", func(f fixture) []Payload {
			return []Payload{
				CreateProject{Title: "New"},
				UpdateProject{ProjectID: ID(f.projB), Why: ptr("because")},
			}
		}, func(f fixture) []int64 { return []int64{f.projB, f.projB + 1} }},
		{"ideas don't count", func(f fixture) []Payload {
			return []Payload{
				ParkIdea{ProjectID: ptr(ID(f.projA)), Title: "t"},
				UpdateIdea{IdeaID: ID(f.iA), Title: ptr("u")},
				DeleteIdea{IdeaID: ID(f.iA)},
			}
		}, func(fixture) []int64 { return nil }},
		{"each project once", func(f fixture) []Payload {
			return []Payload{
				UpdateTask{TaskID: ID(f.a1), Title: ptr("y")},
				UpdateTask{TaskID: ID(f.a2), Title: ptr("z")},
			}
		}, func(f fixture) []int64 { return []int64{f.projA} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			res, err := NewApplier(f.s, 3, nil).ApplyManual(context.Background(), tt.ops(f)...)
			if err != nil {
				t.Fatal(err)
			}
			if want := tt.want(f); !slices.Equal(res.Projects, want) {
				t.Errorf("Projects = %v, want %v", res.Projects, want)
			}
		})
	}
}

func TestAfterApply(t *testing.T) {
	ctx := context.Background()

	t.Run("called after a manual apply", func(t *testing.T) {
		f := newFixture(t)
		var r recorder
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		res, err := NewApplier(f.s, 3, &r).ApplyManual(ctx, SetTaskStatus{TaskID: ID(f.a1), Status: domain.TaskDone})
		if err != nil {
			t.Fatal(err)
		}
		if len(r.got) != 1 || r.got[0].Changeset.ID != res.Changeset.ID {
			t.Errorf("hook got %+v, want the one result", r.got)
		}
		if r.cancellable {
			t.Error("the hook's context can be cancelled: a caller leaving after the commit would cut it short")
		}
		// It runs after commit: the change is visible outside the transaction.
		if tk := must(f.s.GetTask(ctx, f.a1))(t); tk.Status != domain.TaskDone {
			t.Errorf("task status %s when the hook ran", tk.Status)
		}
	})

	t.Run("called after Apply, without undecided ops' projects", func(t *testing.T) {
		f := newFixture(t)
		_, run := newRun(t, f)
		cs := propose(t, f, &run,
			op(1, domain.OpSetTaskStatus, jsonf(`{"task_id":%d,"status":"done"}`, f.a1)),
			op(2, domain.OpSetTaskStatus, jsonf(`{"task_id":%d,"status":"done"}`, f.b1)),
		)
		var r recorder
		if _, err := NewApplier(f.s, 3, &r).Apply(ctx, cs, acceptAll(1)); err != nil {
			t.Fatal(err)
		}
		if len(r.got) != 1 || !slices.Equal(r.got[0].Projects, []int64{f.projA}) {
			t.Errorf("hook got %+v, want one result for project %d", r.got, f.projA)
		}
	})

	t.Run("not called when the apply fails", func(t *testing.T) {
		f := newFixture(t)
		var r recorder
		a := NewApplier(f.s, 3, &r)
		if _, err := a.ApplyManual(ctx, SetTaskStatus{TaskID: ID(999), Status: domain.TaskDone}); err == nil {
			t.Fatal("apply of a missing task succeeded")
		}
		_, run := newRun(t, f)
		cs := propose(t, f, &run, op(1, domain.OpSetTaskStatus, jsonf(`{"task_id":%d,"status":"done"}`, f.a1)))
		if _, err := a.Apply(ctx, cs, map[int64]Decision{}); err == nil {
			t.Fatal("apply with no decisions succeeded")
		}
		if len(r.got) != 0 {
			t.Errorf("hook called %d times", len(r.got))
		}
	})
}

func TestAfterApplies(t *testing.T) {
	var a, b recorder
	res := Result{Projects: []int64{4}}
	AfterApplies{&a, &b}.Applied(context.Background(), res)
	if len(a.got) != 1 || len(b.got) != 1 || !slices.Equal(b.got[0].Projects, []int64{4}) {
		t.Errorf("hooks got %+v and %+v, want the result once each", a.got, b.got)
	}
}
