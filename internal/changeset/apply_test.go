package changeset

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/ogredforman/waypoint/internal/domain"
	"github.com/ogredforman/waypoint/internal/store"
)

// propose stores a pending changeset with ops, which must be valid. runID
// nil makes it manual.
func propose(t *testing.T, f fixture, runID *int64, ops ...domain.ChangeOp) int64 {
	t.Helper()
	if _, err := ValidateOps(ops); err != nil {
		t.Fatalf("propose: %v", err)
	}
	cs := domain.Changeset{Source: domain.SourceManual, Status: domain.ChangesetPending}
	if runID != nil {
		cs.Source, cs.ExtractionRunID = domain.SourceExtraction, runID
	}
	for i := range ops {
		ops[i].Status = domain.OpPending
	}
	var id int64
	err := f.s.InTx(context.Background(), func(tx *store.Tx) error {
		created, _, err := tx.CreateChangeset(context.Background(), cs, ops)
		id = created.ID
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// newRun stores an entry and a succeeded extraction run over it.
func newRun(t *testing.T, f fixture) (entryID, runID int64) {
	t.Helper()
	ctx := context.Background()
	err := f.s.InTx(ctx, func(tx *store.Tx) error {
		e, err := tx.CreateEntry(ctx, domain.Entry{Body: "todo: something"})
		entryID = e.ID
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	err = f.db.QueryRowContext(ctx, `INSERT INTO extraction_runs (entry_id, provider, status, created_at, updated_at)
		VALUES (?, 'mock', 'succeeded', 1, 1) RETURNING id`, entryID).Scan(&runID)
	if err != nil {
		t.Fatal(err)
	}
	return entryID, runID
}

func acceptAll(ops ...int64) map[int64]Decision {
	d := map[int64]Decision{}
	for _, pos := range ops {
		d[pos] = Decision{Action: Accept}
	}
	return d
}

// jsonf formats a payload with row IDs from the fixture.
func jsonf(format string, ids ...any) string { return fmt.Sprintf(format, ids...) }

// must takes a (value, error) call and returns its value, failing the test on
// an error: must(f.s.GetTask(ctx, id))(t).
func must[T any](v T, err error) func(*testing.T) T {
	return func(t *testing.T) T {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
}

func opStatuses(t *testing.T, f fixture, changesetID int64) []domain.ChangeOpStatus {
	t.Helper()
	ops := must(f.s.ListChangeOps(context.Background(), changesetID))(t)
	out := make([]domain.ChangeOpStatus, len(ops))
	for i, op := range ops {
		out[i] = op.Status
	}
	return out
}

func taskCount(t *testing.T, f fixture, projectID int64) int {
	t.Helper()
	return len(must(f.s.ListTasksByProject(context.Background(), projectID))(t))
}

func TestApplyTempIDs(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	cs := propose(t, f, nil,
		op(1, domain.OpCreateProject, `{"title":"Garden","why":"fresh food"}`),
		op(2, domain.OpCreateTask, `{"project_id":"$new:1","title":"Buy seeds","size":"s"}`),
		op(3, domain.OpCreateTask, `{"project_id":"$new:1","title":"Pick seeds","parent_task_id":"$new:2"}`),
		op(4, domain.OpCreateBlocker, `{"project_id":"$new:1","task_id":"$new:2","description":"Shop closed"}`),
		op(5, domain.OpParkIdea, `{"project_id":"$new:1","title":"Greenhouse"}`),
		op(6, domain.OpAddDependency, `{"task_id":"$new:3","blocked_by_task_id":"$new:2"}`),
	)

	res, err := NewApplier(f.s, 3).Apply(ctx, cs, acceptAll(1, 2, 3, 4, 5, 6))
	if err != nil {
		t.Fatal(err)
	}
	if res.Changeset.Status != domain.ChangesetApplied || res.Changeset.ResolvedAt == nil {
		t.Errorf("changeset %+v, want applied and resolved", res.Changeset)
	}
	if res.SplitOff != nil || res.HeldByCap != nil {
		t.Errorf("split off %v, held %v; want none", res.SplitOff, res.HeldByCap)
	}
	if got := opStatuses(t, f, cs); !slices.Equal(got, slices.Repeat([]domain.ChangeOpStatus{domain.OpApplied}, 6)) {
		t.Errorf("op statuses %v", got)
	}

	proj := must(f.s.GetProject(ctx, res.Created[1]))(t)
	if proj.Title != "Garden" || proj.Why != "fresh food" || proj.Status != domain.ProjectParked {
		t.Errorf("project %+v", proj)
	}
	seeds := must(f.s.GetTask(ctx, res.Created[2]))(t)
	pick := must(f.s.GetTask(ctx, res.Created[3]))(t)
	if seeds.ProjectID != proj.ID || seeds.Status != domain.TaskTodo || seeds.Size == nil || *seeds.Size != domain.SizeS || seeds.OriginEntryID != nil {
		t.Errorf("task 2 %+v", seeds)
	}
	if pick.ParentTaskID == nil || *pick.ParentTaskID != seeds.ID {
		t.Errorf("task 3 parent %v, want %d", pick.ParentTaskID, seeds.ID)
	}
	b := must(f.s.GetBlocker(ctx, res.Created[4]))(t)
	if b.ProjectID != proj.ID || b.TaskID == nil || *b.TaskID != seeds.ID || b.Status != domain.BlockerOpen {
		t.Errorf("blocker %+v", b)
	}
	i := must(f.s.GetIdea(ctx, res.Created[5]))(t)
	if i.ProjectID == nil || *i.ProjectID != proj.ID || i.Status != domain.IdeaParked {
		t.Errorf("idea %+v", i)
	}
	var deps int
	if err := f.db.QueryRowContext(ctx, `SELECT count(*) FROM task_dependencies WHERE task_id = ? AND blocked_by_task_id = ?`,
		pick.ID, seeds.ID).Scan(&deps); err != nil || deps != 1 {
		t.Errorf("dependencies %d, %v; want 1", deps, err)
	}
}

// TestApplyOps applies each op that changes an existing row on its own.
func TestApplyOps(t *testing.T) {
	ctx := context.Background()
	for _, tt := range []struct {
		name    string
		op      func(f fixture) domain.ChangeOp
		check   func(t *testing.T, f fixture)
		deleted string // the "deleted" object expected in the applied payload
	}{
		{"update task", func(f fixture) domain.ChangeOp {
			return op(1, domain.OpUpdateTask, jsonf(`{"task_id":%d,"title":"New","notes":"n","size":"l","energy":"low","parent_task_id":null,"position":7}`, f.a2))
		}, func(t *testing.T, f fixture) {
			tk := must(f.s.GetTask(ctx, f.a2))(t)
			if tk.Title != "New" || tk.Notes != "n" || *tk.Size != domain.SizeL || *tk.Energy != domain.EnergyLow || tk.ParentTaskID != nil || tk.Position != 7 {
				t.Errorf("task %+v", tk)
			}
		}, ""},
		{"update task clears size", func(f fixture) domain.ChangeOp {
			return op(1, domain.OpUpdateTask, jsonf(`{"task_id":%d,"size":null,"parent_task_id":%d}`, f.a4, f.a1))
		}, func(t *testing.T, f fixture) {
			tk := must(f.s.GetTask(ctx, f.a4))(t)
			if tk.Size != nil || tk.ParentTaskID == nil || *tk.ParentTaskID != f.a1 || tk.Title != "t" {
				t.Errorf("task %+v", tk)
			}
		}, ""},
		{"set task status", func(f fixture) domain.ChangeOp {
			return op(1, domain.OpSetTaskStatus, jsonf(`{"task_id":%d,"status":"done"}`, f.a1))
		}, func(t *testing.T, f fixture) {
			if tk := must(f.s.GetTask(ctx, f.a1))(t); tk.Status != domain.TaskDone {
				t.Errorf("status %s", tk.Status)
			}
		}, ""},
		{"split task", func(f fixture) domain.ChangeOp {
			return op(1, domain.OpSplitTask, jsonf(`{"task_id":%d,"subtasks":[{"title":"x","energy":"low"},{"title":"y"}]}`, f.b1))
		}, func(t *testing.T, f fixture) {
			tasks := must(f.s.ListTasksByProject(ctx, f.projB))(t)
			var subs []string
			for _, tk := range tasks {
				if tk.ParentTaskID != nil && *tk.ParentTaskID == f.b1 {
					subs = append(subs, tk.Title)
				}
			}
			if !slices.Equal(subs, []string{"x", "y"}) {
				t.Errorf("subtasks %v", subs)
			}
		}, ""},
		{"add existing dependency", func(f fixture) domain.ChangeOp {
			return op(1, domain.OpAddDependency, jsonf(`{"task_id":%d,"blocked_by_task_id":%d}`, f.a4, f.a1))
		}, func(t *testing.T, f fixture) {
			// The fixture has none; apply a second time to check it's a no-op.
			cs := propose(t, f, nil, op(1, domain.OpAddDependency, jsonf(`{"task_id":%d,"blocked_by_task_id":%d}`, f.a4, f.a1)))
			if _, err := NewApplier(f.s, 3).Apply(ctx, cs, acceptAll(1)); err != nil {
				t.Fatal(err)
			}
			var n int
			if err := f.db.QueryRowContext(ctx, `SELECT count(*) FROM task_dependencies`).Scan(&n); err != nil || n != 1 {
				t.Errorf("dependencies %d, %v; want 1", n, err)
			}
		}, ""},
		{"delete task", func(f fixture) domain.ChangeOp {
			return op(1, domain.OpDeleteTask, jsonf(`{"task_id":%d}`, f.a1))
		}, func(t *testing.T, f fixture) {
			if _, err := f.s.GetTask(ctx, f.a1); !errors.Is(err, store.ErrNotFound) {
				t.Errorf("task still there: %v", err)
			}
		}, `"title":"t"`},
		{"update blocker", func(f fixture) domain.ChangeOp {
			return op(1, domain.OpUpdateBlocker, jsonf(`{"blocker_id":%d,"description":"new","task_id":null}`, f.bA))
		}, func(t *testing.T, f fixture) {
			b := must(f.s.GetBlocker(ctx, f.bA))(t)
			if b.Description != "new" || b.TaskID != nil {
				t.Errorf("blocker %+v", b)
			}
		}, ""},
		{"resolve blocker manually", func(f fixture) domain.ChangeOp {
			return op(1, domain.OpResolveBlocker, jsonf(`{"blocker_id":%d}`, f.bA))
		}, func(t *testing.T, f fixture) {
			b := must(f.s.GetBlocker(ctx, f.bA))(t)
			if b.Status != domain.BlockerResolved || b.ResolvedEntryID != nil {
				t.Errorf("blocker %+v", b)
			}
		}, ""},
		{"delete blocker", func(f fixture) domain.ChangeOp {
			return op(1, domain.OpDeleteBlocker, jsonf(`{"blocker_id":%d}`, f.bB))
		}, func(t *testing.T, f fixture) {
			if _, err := f.s.GetBlocker(ctx, f.bB); !errors.Is(err, store.ErrNotFound) {
				t.Errorf("blocker still there: %v", err)
			}
		}, `"description":"d"`},
		{"update idea", func(f fixture) domain.ChangeOp {
			return op(1, domain.OpUpdateIdea, jsonf(`{"idea_id":%d,"title":"T","body":"B","project_id":null,"status":"promoted"}`, f.iA))
		}, func(t *testing.T, f fixture) {
			i := must(f.s.GetIdea(ctx, f.iA))(t)
			if i.Title != "T" || i.Body != "B" || i.ProjectID != nil || i.Status != domain.IdeaPromoted {
				t.Errorf("idea %+v", i)
			}
		}, ""},
		{"delete idea", func(f fixture) domain.ChangeOp {
			return op(1, domain.OpDeleteIdea, jsonf(`{"idea_id":%d}`, f.iA))
		}, func(t *testing.T, f fixture) {
			if _, err := f.s.GetIdea(ctx, f.iA); !errors.Is(err, store.ErrNotFound) {
				t.Errorf("idea still there: %v", err)
			}
		}, `"status":"parked"`},
		{"update project", func(f fixture) domain.ChangeOp {
			return op(1, domain.OpUpdateProject, jsonf(`{"project_id":%d,"title":"A2","why":"w","definition_of_done":"d","status":"done"}`, f.projA))
		}, func(t *testing.T, f fixture) {
			p := must(f.s.GetProject(ctx, f.projA))(t)
			if p.Title != "A2" || p.Why != "w" || p.DefinitionOfDone != "d" || p.Status != domain.ProjectDone {
				t.Errorf("project %+v", p)
			}
		}, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			cs := propose(t, f, nil, tt.op(f))
			res, err := NewApplier(f.s, 3).Apply(ctx, cs, acceptAll(1))
			if err != nil {
				t.Fatal(err)
			}
			if res.Changeset.Status != domain.ChangesetApplied {
				t.Errorf("changeset status %s", res.Changeset.Status)
			}
			tt.check(t, f)

			applied := must(f.s.ListChangeOps(ctx, cs))(t)[0].AppliedPayload
			switch {
			case tt.deleted == "" && applied != nil:
				t.Errorf("applied payload %s, want none", applied)
			case tt.deleted != "":
				var got struct{ Deleted json.RawMessage }
				if err := json.Unmarshal(applied, &got); err != nil || !json.Valid(got.Deleted) ||
					!strings.Contains(string(got.Deleted), tt.deleted) {
					t.Errorf("applied payload %s, want a deleted row with %s", applied, tt.deleted)
				}
			}
		})
	}
}

func TestApplyExtractionSetsEntries(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	entryID, runID := newRun(t, f)
	cs := propose(t, f, &runID,
		op(1, domain.OpCreateTask, jsonf(`{"project_id":%d,"title":"x"}`, f.projA)),
		op(2, domain.OpSplitTask, `{"task_id":"$new:1","subtasks":[{"title":"y"}]}`),
		op(3, domain.OpCreateBlocker, jsonf(`{"project_id":%d,"description":"d"}`, f.projA)),
		op(4, domain.OpParkIdea, `{"title":"i"}`),
		op(5, domain.OpResolveBlocker, jsonf(`{"blocker_id":%d}`, f.bA)),
	)
	res, err := NewApplier(f.s, 3).Apply(ctx, cs, acceptAll(1, 2, 3, 4, 5))
	if err != nil {
		t.Fatal(err)
	}
	is := func(what string, got *int64) {
		t.Helper()
		if got == nil || *got != entryID {
			t.Errorf("%s = %v, want entry %d", what, got, entryID)
		}
	}
	is("task origin", must(f.s.GetTask(ctx, res.Created[1]))(t).OriginEntryID)
	is("blocker origin", must(f.s.GetBlocker(ctx, res.Created[3]))(t).OriginEntryID)
	is("idea origin", must(f.s.GetIdea(ctx, res.Created[4]))(t).OriginEntryID)
	is("resolved entry", must(f.s.GetBlocker(ctx, f.bA))(t).ResolvedEntryID)
	for _, tk := range must(f.s.ListTasksByProject(ctx, f.projA))(t) {
		if tk.Title == "y" {
			is("subtask origin", tk.OriginEntryID)
		}
	}
}

func TestApplyRejectAndEdit(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	cs := propose(t, f, nil,
		op(1, domain.OpCreateTask, jsonf(`{"project_id":%d,"title":"proposed"}`, f.projA)),
		op(2, domain.OpSetTaskStatus, jsonf(`{"task_id":%d,"status":"done"}`, f.a1)),
		op(3, domain.OpParkIdea, `{"title":"maybe"}`),
	)
	edited := json.RawMessage(jsonf(`{"project_id":%d,"title":"edited"}`, f.projA))
	res, err := NewApplier(f.s, 3).Apply(ctx, cs, map[int64]Decision{
		1: {Action: Edit, Payload: edited},
		2: {Action: Accept},
		3: {Action: Reject},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Changeset.Status != domain.ChangesetPartiallyApplied {
		t.Errorf("status %s, want partially_applied", res.Changeset.Status)
	}
	ops := must(f.s.ListChangeOps(ctx, cs))(t)
	if got := opStatuses(t, f, cs); !slices.Equal(got, []domain.ChangeOpStatus{domain.OpEdited, domain.OpApplied, domain.OpRejected}) {
		t.Errorf("op statuses %v", got)
	}
	if string(ops[0].AppliedPayload) != string(edited) || ops[1].AppliedPayload != nil || ops[2].AppliedPayload != nil {
		t.Errorf("applied payloads %s, %s, %s", ops[0].AppliedPayload, ops[1].AppliedPayload, ops[2].AppliedPayload)
	}
	if tk := must(f.s.GetTask(ctx, res.Created[1]))(t); tk.Title != "edited" {
		t.Errorf("task title %q, want edited", tk.Title)
	}
	if ideas := must(f.s.ListIdeasByStatus(ctx, domain.IdeaParked))(t); len(ideas) != 1 {
		t.Errorf("%d parked ideas, want only the fixture's", len(ideas))
	}
}

func TestApplyAllRejected(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	cs := propose(t, f, nil, op(1, domain.OpDeleteTask, jsonf(`{"task_id":%d}`, f.a1)))
	res, err := NewApplier(f.s, 3).Apply(ctx, cs, map[int64]Decision{1: {Action: Reject}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Changeset.Status != domain.ChangesetRejected || res.Changeset.ResolvedAt == nil {
		t.Errorf("changeset %+v, want rejected and resolved", res.Changeset)
	}
	must(f.s.GetTask(ctx, f.a1))(t)
}

func TestApplySplitsOffUndecided(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	entryID, runID := newRun(t, f)
	cs := propose(t, f, &runID,
		op(1, domain.OpCreateProject, `{"title":"Garden"}`),
		op(2, domain.OpCreateTask, `{"project_id":"$new:1","title":"Dig"}`),
		op(3, domain.OpCreateTask, `{"project_id":"$new:1","title":"Plant","parent_task_id":"$new:2"}`),
		op(4, domain.OpSetTaskStatus, `{"task_id":"$new:3","status":"in_progress"}`),
		op(5, domain.OpSetTaskStatus, jsonf(`{"task_id":%d,"status":"done"}`, f.a1)),
	)
	a := NewApplier(f.s, 3)
	res, err := a.Apply(ctx, cs, map[int64]Decision{1: {Action: Accept}, 2: {Action: Accept}, 5: {Action: Accept}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Changeset.Status != domain.ChangesetApplied {
		t.Errorf("status %s, want applied", res.Changeset.Status)
	}
	if res.SplitOff == nil {
		t.Fatal("no split-off changeset")
	}
	child := *res.SplitOff
	if child.Status != domain.ChangesetPending || child.Source != domain.SourceExtraction ||
		*child.ExtractionRunID != runID || *child.ParentChangesetID != cs {
		t.Errorf("split-off changeset %+v", child)
	}
	if n := len(must(f.s.ListChangeOps(ctx, cs))(t)); n != 3 {
		t.Errorf("%d ops left in the original, want 3", n)
	}
	moved := must(f.s.ListChangeOps(ctx, child.ID))(t)
	var positions []int64
	for _, op := range moved {
		positions = append(positions, op.Position)
	}
	if !slices.Equal(positions, []int64{3, 4}) {
		t.Fatalf("moved positions %v, want [3 4]", positions)
	}
	var got3 map[string]any
	if err := json.Unmarshal(moved[0].Payload, &got3); err != nil {
		t.Fatal(err)
	}
	if got3["project_id"] != float64(res.Created[1]) || got3["parent_task_id"] != float64(res.Created[2]) || got3["title"] != "Plant" {
		t.Errorf("rewritten payload %s", moved[0].Payload)
	}
	if string(moved[1].Payload) != `{"task_id":"$new:3","status":"in_progress"}` {
		t.Errorf("payload of op 4 %s, want its temp ref kept", moved[1].Payload)
	}

	// The split-off changeset applies on its own later, and can split again.
	res2, err := a.Apply(ctx, child.ID, acceptAll(3))
	if err != nil {
		t.Fatal(err)
	}
	if res2.SplitOff == nil || *res2.SplitOff.ParentChangesetID != child.ID {
		t.Fatalf("second apply %+v, want a split-off child of %d", res2, child.ID)
	}
	plant := must(f.s.GetTask(ctx, res2.Created[3]))(t)
	if *plant.ParentTaskID != res.Created[2] || *plant.OriginEntryID != entryID {
		t.Errorf("task %+v", plant)
	}
	grand := must(f.s.ListChangeOps(ctx, res2.SplitOff.ID))(t)
	if len(grand) != 1 || grand[0].Position != 4 || string(grand[0].Payload) != jsonf(`{"status":"in_progress","task_id":%d}`, plant.ID) {
		t.Fatalf("grandchild ops %+v", grand)
	}
	res3, err := a.Apply(ctx, res2.SplitOff.ID, acceptAll(4))
	if err != nil {
		t.Fatal(err)
	}
	if res3.SplitOff != nil || res3.Changeset.Status != domain.ChangesetApplied {
		t.Errorf("third apply %+v", res3)
	}
	if tk := must(f.s.GetTask(ctx, plant.ID))(t); tk.Status != domain.TaskInProgress {
		t.Errorf("task status %s", tk.Status)
	}
}

// TestApplySplitStatus checks the status of the resolved changeset when some
// ops split off.
func TestApplySplitStatus(t *testing.T) {
	ctx := context.Background()
	for _, tt := range []struct {
		name      string
		decisions map[int64]Decision
		want      domain.ChangesetStatus
	}{
		{"accept and reject", map[int64]Decision{1: {Action: Accept}, 2: {Action: Reject}}, domain.ChangesetPartiallyApplied},
		{"reject only", map[int64]Decision{2: {Action: Reject}}, domain.ChangesetRejected},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			cs := propose(t, f, nil,
				op(1, domain.OpSetTaskStatus, jsonf(`{"task_id":%d,"status":"done"}`, f.a1)),
				op(2, domain.OpSetTaskStatus, jsonf(`{"task_id":%d,"status":"done"}`, f.a2)),
				op(3, domain.OpSetTaskStatus, jsonf(`{"task_id":%d,"status":"done"}`, f.a3)),
			)
			res, err := NewApplier(f.s, 3).Apply(ctx, cs, tt.decisions)
			if err != nil {
				t.Fatal(err)
			}
			if res.Changeset.Status != tt.want || res.SplitOff == nil {
				t.Fatalf("result %+v, want %s with a split-off changeset", res, tt.want)
			}
			if n := len(must(f.s.ListChangeOps(ctx, res.SplitOff.ID))(t)); n != 3-len(tt.decisions) {
				t.Errorf("%d ops split off", n)
			}
		})
	}
}

func TestApplyActiveCap(t *testing.T) {
	ctx := context.Background()

	// The fixture has two active projects; the cap is 2, so C can't become active.
	setup := func(t *testing.T) (fixture, int64) {
		f := newFixture(t)
		var projC int64
		err := f.s.InTx(ctx, func(tx *store.Tx) error {
			p, err := tx.CreateProject(ctx, domain.Project{Title: "C", Status: domain.ProjectParked})
			projC = p.ID
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return f, projC
	}
	activate := func(projC int64) domain.ChangeOp {
		return op(1, domain.OpUpdateProject, jsonf(`{"project_id":%d,"status":"active"}`, projC))
	}

	t.Run("extraction holds the op back", func(t *testing.T) {
		f, projC := setup(t)
		_, runID := newRun(t, f)
		a := NewApplier(f.s, 2)
		cs := propose(t, f, &runID, activate(projC), op(2, domain.OpCreateTask, jsonf(`{"project_id":%d,"title":"x"}`, f.projA)))
		res, err := a.Apply(ctx, cs, acceptAll(1, 2))
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(res.HeldByCap, []int64{1}) || res.SplitOff == nil || res.Changeset.Status != domain.ChangesetApplied {
			t.Fatalf("result %+v", res)
		}
		if p := must(f.s.GetProject(ctx, projC))(t); p.Status != domain.ProjectParked {
			t.Errorf("project C is %s", p.Status)
		}
		if taskCount(t, f, f.projA) != 5 {
			t.Errorf("task not created")
		}

		// Still no room: nothing else was decided, so the cap is reported.
		_, err = a.Apply(ctx, res.SplitOff.ID, acceptAll(1))
		var ce *domain.ActiveCapError
		var oe *OpError
		if !errors.As(err, &ce) || !errors.As(err, &oe) || oe.Position != 1 {
			t.Fatalf("got %v, want an OpError at 1 wrapping ActiveCapError", err)
		}
		if cs := must(f.s.GetChangeset(ctx, res.SplitOff.ID))(t); cs.Status != domain.ChangesetPending {
			t.Errorf("split-off changeset is %s, want pending", cs.Status)
		}

		// Park project B, then it applies.
		park := propose(t, f, nil, op(1, domain.OpUpdateProject, jsonf(`{"project_id":%d,"status":"parked"}`, f.projB)))
		if _, err := a.Apply(ctx, park, acceptAll(1)); err != nil {
			t.Fatal(err)
		}
		if _, err := a.Apply(ctx, res.SplitOff.ID, acceptAll(1)); err != nil {
			t.Fatal(err)
		}
		if p := must(f.s.GetProject(ctx, projC))(t); p.Status != domain.ProjectActive {
			t.Errorf("project C is %s, want active", p.Status)
		}
	})

	t.Run("new active project keeps the edit", func(t *testing.T) {
		f, _ := setup(t)
		_, runID := newRun(t, f)
		cs := propose(t, f, &runID,
			op(1, domain.OpCreateProject, `{"title":"Garden"}`),
			op(2, domain.OpUpdateProject, `{"project_id":"$new:1","status":"active"}`),
		)
		res, err := NewApplier(f.s, 2).Apply(ctx, cs, map[int64]Decision{
			1: {Action: Accept},
			2: {Action: Edit, Payload: json.RawMessage(`{"project_id":"$new:1","status":"active","why":"food"}`)},
		})
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(res.HeldByCap, []int64{2}) || res.SplitOff == nil {
			t.Fatalf("result %+v", res)
		}
		held := must(f.s.ListChangeOps(ctx, res.SplitOff.ID))(t)
		want := jsonf(`{"project_id":%d,"status":"active","why":"food"}`, res.Created[1])
		if len(held) != 1 || string(held[0].Payload) != want || held[0].Status != domain.OpPending {
			t.Errorf("held op %+v, want payload %s", held, want)
		}
	})

	t.Run("manual fails", func(t *testing.T) {
		f, projC := setup(t)
		cs := propose(t, f, nil, activate(projC), op(2, domain.OpCreateTask, jsonf(`{"project_id":%d,"title":"x"}`, f.projA)))
		_, err := NewApplier(f.s, 2).Apply(ctx, cs, acceptAll(1, 2))
		var ce *domain.ActiveCapError
		if !errors.As(err, &ce) || ce.Cap != 2 || ce.Active != 2 {
			t.Fatalf("got %v, want ActiveCapError 2 of 2", err)
		}
		if taskCount(t, f, f.projA) != 4 {
			t.Errorf("task created despite the failure")
		}
		if got := must(f.s.GetChangeset(ctx, cs))(t); got.Status != domain.ChangesetPending {
			t.Errorf("changeset is %s, want pending", got.Status)
		}
	})

	t.Run("staying active passes", func(t *testing.T) {
		f, _ := setup(t)
		cs := propose(t, f, nil, op(1, domain.OpUpdateProject, jsonf(`{"project_id":%d,"status":"active","title":"A!"}`, f.projA)))
		if _, err := NewApplier(f.s, 1).Apply(ctx, cs, acceptAll(1)); err != nil {
			t.Fatal(err)
		}
	})
}

func TestApplyRollsBack(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	cs := propose(t, f, nil,
		op(1, domain.OpCreateTask, jsonf(`{"project_id":%d,"title":"x"}`, f.projA)),
		op(2, domain.OpDeleteTask, jsonf(`{"task_id":%d}`, f.a1)),
		op(3, domain.OpSetTaskStatus, `{"task_id":999,"status":"done"}`),
	)
	_, err := NewApplier(f.s, 3).Apply(ctx, cs, acceptAll(1, 2, 3))
	var oe *OpError
	if !errors.As(err, &oe) || oe.Position != 3 || oe.OpType != domain.OpSetTaskStatus {
		t.Fatalf("got %v, want an OpError at 3", err)
	}
	if got := fields(t, err); !slices.Equal(got, []string{"task_id"}) {
		t.Errorf("problem fields %v", got)
	}
	if taskCount(t, f, f.projA) != 4 {
		t.Errorf("tasks changed despite the rollback")
	}
	if got := opStatuses(t, f, cs); !slices.Equal(got, slices.Repeat([]domain.ChangeOpStatus{domain.OpPending}, 3)) {
		t.Errorf("op statuses %v", got)
	}
	if got := must(f.s.GetChangeset(ctx, cs))(t); got.Status != domain.ChangesetPending || got.ResolvedAt != nil {
		t.Errorf("changeset %+v, want pending", got)
	}
}

func TestApplyDecisionProblems(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	cs := propose(t, f, nil,
		op(1, domain.OpCreateProject, `{"title":"P"}`),
		op(2, domain.OpCreateTask, `{"project_id":"$new:1","title":"x"}`),
		op(3, domain.OpSetTaskStatus, jsonf(`{"task_id":%d,"status":"done"}`, f.a1)),
	)
	a := NewApplier(f.s, 3)
	for _, tt := range []struct {
		name      string
		decisions map[int64]Decision
		want      []string
	}{
		{"none", nil, []string{"decisions"}},
		{"unknown position", map[int64]Decision{9: {Action: Accept}}, []string{"decisions[9]"}},
		{"bad action", map[int64]Decision{3: {Action: "maybe"}}, []string{"decisions[3]"}},
		{"edit without payload", map[int64]Decision{3: {Action: Edit}}, []string{"decisions[3]"}},
		{"accept with payload", map[int64]Decision{3: {Action: Accept, Payload: json.RawMessage(`{}`)}}, []string{"decisions[3]"}},
		{"invalid edit", map[int64]Decision{3: {Action: Edit, Payload: json.RawMessage(`{"task_id":1,"status":"late"}`)}}, []string{"ops[3].status"}},
		{"uses undecided", map[int64]Decision{2: {Action: Accept}}, []string{"decisions[2]"}},
		{"uses rejected", map[int64]Decision{1: {Action: Reject}, 2: {Action: Accept}}, []string{"decisions[2]"}},
		{"undecided uses rejected", map[int64]Decision{1: {Action: Reject}}, []string{"decisions[2]"}},
		{"edit adds ref to undecided", map[int64]Decision{3: {Action: Edit, Payload: json.RawMessage(`{"task_id":"$new:2","status":"done"}`)}}, []string{"decisions[3]"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := a.Apply(ctx, cs, tt.decisions)
			if got := fields(t, err); !slices.Equal(got, tt.want) {
				t.Errorf("problem fields %v, want %v (%v)", got, tt.want, err)
			}
		})
	}
	if got := must(f.s.GetChangeset(ctx, cs))(t); got.Status != domain.ChangesetPending {
		t.Errorf("changeset is %s after failed applies", got.Status)
	}

	// Rejecting the dependent op too is fine.
	if _, err := a.Apply(ctx, cs, map[int64]Decision{1: {Action: Reject}, 2: {Action: Reject}}); err != nil {
		t.Errorf("reject both: %v", err)
	}
}

func TestApplyNotPending(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	a := NewApplier(f.s, 3)
	cs := propose(t, f, nil, op(1, domain.OpParkIdea, `{"title":"i"}`))
	if _, err := a.Apply(ctx, cs, acceptAll(1)); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Apply(ctx, cs, acceptAll(1)); !errors.Is(err, ErrNotPending) {
		t.Errorf("second apply: got %v, want ErrNotPending", err)
	}
	if _, err := a.Apply(ctx, 999, acceptAll(1)); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("missing changeset: got %v, want ErrNotFound", err)
	}
}

// dbSnapshot dumps every row of every table, so a test can check that a
// failed apply left nothing behind.
func dbSnapshot(t *testing.T, f fixture) string {
	t.Helper()
	ctx := context.Background()
	var tables []string
	rows, err := f.db.QueryContext(ctx, `SELECT name FROM sqlite_master
		WHERE type = 'table' AND name NOT LIKE 'sqlite_%' AND name NOT LIKE 'goose_%' ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, name)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}

	var b strings.Builder
	for _, table := range tables {
		rows, err := f.db.QueryContext(ctx, `SELECT * FROM `+table+` ORDER BY rowid`)
		if err != nil {
			t.Fatal(err)
		}
		cols, err := rows.Columns()
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&b, "%s %v\n", table, cols)
		for rows.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				t.Fatal(err)
			}
			for i, v := range vals {
				if s, ok := v.([]byte); ok {
					vals[i] = string(s)
				}
			}
			fmt.Fprintln(&b, vals...)
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
	}
	return b.String()
}

// everyOpType is one changeset with an op of every type. Ops use temp IDs
// wherever a ref can take one, including the nullable refs.
func everyOpType(f fixture) []domain.ChangeOp {
	return []domain.ChangeOp{
		op(1, domain.OpCreateProject, `{"title":"Garden"}`),
		op(2, domain.OpUpdateProject, `{"project_id":"$new:1","status":"active","why":"food"}`),
		op(3, domain.OpCreateTask, `{"project_id":"$new:1","title":"Dig"}`),
		op(4, domain.OpCreateTask, `{"project_id":"$new:1","title":"Plant","parent_task_id":"$new:3"}`),
		op(5, domain.OpCreateTask, `{"project_id":"$new:1","title":"Water"}`),
		op(6, domain.OpUpdateTask, `{"task_id":"$new:5","parent_task_id":"$new:3","size":"s"}`),
		op(7, domain.OpSetTaskStatus, `{"task_id":"$new:4","status":"in_progress"}`),
		op(8, domain.OpSplitTask, `{"task_id":"$new:3","subtasks":[{"title":"Find spade"}]}`),
		op(9, domain.OpAddDependency, `{"task_id":"$new:5","blocked_by_task_id":"$new:4"}`),
		op(10, domain.OpCreateBlocker, `{"project_id":"$new:1","description":"No seeds"}`),
		op(11, domain.OpUpdateBlocker, `{"blocker_id":"$new:10","task_id":"$new:4"}`),
		op(12, domain.OpResolveBlocker, `{"blocker_id":"$new:10"}`),
		op(13, domain.OpParkIdea, `{"project_id":"$new:1","title":"Greenhouse"}`),
		op(14, domain.OpUpdateIdea, jsonf(`{"idea_id":%d,"project_id":"$new:1"}`, f.iA)),
		op(15, domain.OpDeleteTask, jsonf(`{"task_id":%d}`, f.a4)),
		op(16, domain.OpDeleteBlocker, jsonf(`{"blocker_id":%d}`, f.bB)),
		op(17, domain.OpDeleteIdea, `{"idea_id":"$new:13"}`),
	}
}

func TestApplyEveryOpType(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	ops := everyOpType(f)
	var positions []int64
	for _, o := range ops {
		positions = append(positions, o.Position)
	}
	for _, want := range domain.OpTypes() {
		if !slices.ContainsFunc(ops, func(o domain.ChangeOp) bool { return o.OpType == want }) {
			t.Fatalf("no %s op; add one to everyOpType", want)
		}
	}

	cs := propose(t, f, nil, ops...)
	res, err := NewApplier(f.s, 3).Apply(ctx, cs, acceptAll(positions...))
	if err != nil {
		t.Fatal(err)
	}
	if res.Changeset.Status != domain.ChangesetApplied || res.SplitOff != nil {
		t.Fatalf("result %+v, want applied with nothing split off", res)
	}
	if got := opStatuses(t, f, cs); !slices.Equal(got, slices.Repeat([]domain.ChangeOpStatus{domain.OpApplied}, len(ops))) {
		t.Errorf("op statuses %v", got)
	}

	garden := must(f.s.GetProject(ctx, res.Created[1]))(t)
	if garden.Status != domain.ProjectActive || garden.Why != "food" {
		t.Errorf("project %+v", garden)
	}
	dig, plant, water := res.Created[3], res.Created[4], res.Created[5]
	if tk := must(f.s.GetTask(ctx, plant))(t); tk.ProjectID != garden.ID || tk.ParentTaskID == nil || *tk.ParentTaskID != dig || tk.Status != domain.TaskInProgress {
		t.Errorf("plant %+v", tk)
	}
	if tk := must(f.s.GetTask(ctx, water))(t); tk.ParentTaskID == nil || *tk.ParentTaskID != dig || tk.Size == nil || *tk.Size != domain.SizeS {
		t.Errorf("water %+v", tk)
	}
	var subtasks []string
	for _, tk := range must(f.s.ListTasksByProject(ctx, garden.ID))(t) {
		if tk.ParentTaskID != nil && *tk.ParentTaskID == dig {
			subtasks = append(subtasks, tk.Title)
		}
	}
	if slices.Sort(subtasks); !slices.Equal(subtasks, []string{"Find spade", "Plant", "Water"}) {
		t.Errorf("subtasks of dig %v", subtasks)
	}
	var deps int
	if err := f.db.QueryRowContext(ctx, `SELECT count(*) FROM task_dependencies WHERE task_id = ? AND blocked_by_task_id = ?`,
		water, plant).Scan(&deps); err != nil || deps != 1 {
		t.Errorf("dependencies %d, %v; want 1", deps, err)
	}
	if b := must(f.s.GetBlocker(ctx, res.Created[10]))(t); b.TaskID == nil || *b.TaskID != plant || b.Status != domain.BlockerResolved {
		t.Errorf("blocker %+v", b)
	}
	if i := must(f.s.GetIdea(ctx, f.iA))(t); i.ProjectID == nil || *i.ProjectID != garden.ID {
		t.Errorf("idea %+v", i)
	}
	if _, err := f.s.GetTask(ctx, f.a4); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("task a4: %v, want deleted", err)
	}
	if _, err := f.s.GetBlocker(ctx, f.bB); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("blocker bB: %v, want deleted", err)
	}
	if _, err := f.s.GetIdea(ctx, res.Created[13]); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("parked idea: %v, want deleted", err)
	}
}

// TestApplyRollsBackEveryOpType fails the last op of a changeset holding
// every op type and checks that the database is left exactly as it was.
func TestApplyRollsBackEveryOpType(t *testing.T) {
	ctx := context.Background()
	for _, tt := range []struct {
		name       string
		extraction bool
		cap        int // 2 holds op 2 (Garden becoming active) back on an extraction changeset
	}{
		{"manual with an edit", false, 3},
		{"extraction with an op held by the cap", true, 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// prepare proposes every op type, plus failing op 18 if fail is
			// set, and returns it with decisions accepting all but op 3, which
			// is edited.
			prepare := func(f fixture, fail bool) (int64, map[int64]Decision) {
				var runID *int64
				if tt.extraction {
					_, id := newRun(t, f)
					runID = &id
				}
				ops := everyOpType(f)
				if fail {
					ops = append(ops, op(18, domain.OpSetTaskStatus, `{"task_id":999,"status":"done"}`))
				}
				cs := propose(t, f, runID, ops...)
				decisions := map[int64]Decision{}
				for _, o := range ops {
					decisions[o.Position] = Decision{Action: Accept}
				}
				decisions[3] = Decision{Action: Edit, Payload: json.RawMessage(`{"project_id":"$new:1","title":"Dig deep"}`)}
				return cs, decisions
			}

			// Without the failing op, the apply goes through, holding op 2
			// back on an extraction changeset.
			ok := newFixture(t)
			cs, decisions := prepare(ok, false)
			res, err := NewApplier(ok.s, tt.cap).Apply(ctx, cs, decisions)
			if err != nil {
				t.Fatal(err)
			}
			var wantHeld []int64
			if tt.extraction {
				wantHeld = []int64{2}
			}
			if !slices.Equal(res.HeldByCap, wantHeld) {
				t.Fatalf("held by cap %v, want %v", res.HeldByCap, wantHeld)
			}

			f := newFixture(t)
			cs, decisions = prepare(f, true)
			before := dbSnapshot(t, f)
			_, err = NewApplier(f.s, tt.cap).Apply(ctx, cs, decisions)
			var oe *OpError
			if !errors.As(err, &oe) || oe.Position != 18 {
				t.Fatalf("got %v, want an OpError at 18", err)
			}
			if after := dbSnapshot(t, f); after != before {
				t.Errorf("database changed despite the rollback:\nbefore:\n%s\nafter:\n%s", before, after)
			}
		})
	}
}

// TestApplySplitOffRewritesEveryRefKind leaves ops undecided that use rows
// created by accepted ops, through required, optional, and nullable refs, and
// checks that the split-off ops point at the real rows and still apply.
func TestApplySplitOffRewritesEveryRefKind(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	cs := propose(t, f, nil,
		op(1, domain.OpCreateProject, `{"title":"Garden"}`),
		op(2, domain.OpCreateTask, `{"project_id":"$new:1","title":"Dig"}`),
		op(3, domain.OpCreateTask, `{"project_id":"$new:1","title":"Water"}`),
		op(4, domain.OpUpdateTask, `{"task_id":"$new:3","parent_task_id":"$new:2"}`),
		op(5, domain.OpCreateBlocker, `{"project_id":"$new:1","task_id":"$new:2","description":"d"}`),
		op(6, domain.OpUpdateBlocker, `{"blocker_id":"$new:5","task_id":"$new:3"}`),
		op(7, domain.OpParkIdea, `{"project_id":"$new:1","title":"i"}`),
		op(8, domain.OpUpdateIdea, jsonf(`{"idea_id":%d,"project_id":"$new:1"}`, f.iA)),
		op(9, domain.OpAddDependency, `{"task_id":"$new:3","blocked_by_task_id":"$new:2"}`),
	)
	a := NewApplier(f.s, 3)
	res, err := a.Apply(ctx, cs, acceptAll(1, 2, 3))
	if err != nil {
		t.Fatal(err)
	}
	if res.SplitOff == nil {
		t.Fatal("no split-off changeset")
	}
	garden, dig, water := res.Created[1], res.Created[2], res.Created[3]

	moved := must(f.s.ListChangeOps(ctx, res.SplitOff.ID))(t)
	payloads, err := ValidateOps(moved)
	if err != nil {
		t.Fatalf("split-off ops don't validate: %v", err)
	}
	want := []Payload{
		UpdateTask{TaskID: ID(water), ParentTaskID: To(ID(dig))},
		CreateBlocker{ProjectID: ID(garden), TaskID: ptr(ID(dig)), Description: "d"},
		UpdateBlocker{BlockerID: Temp(5), TaskID: To(ID(water))}, // op 5 moved too, so its temp ref stays
		ParkIdea{ProjectID: ptr(ID(garden)), Title: "i"},
		UpdateIdea{IdeaID: ID(f.iA), ProjectID: To(ID(garden))},
		AddDependency{TaskID: ID(water), BlockedByTaskID: ID(dig)},
	}
	if !reflect.DeepEqual(payloads, want) {
		t.Errorf("split-off payloads\n%+v\nwant\n%+v", payloads, want)
	}

	res2, err := a.Apply(ctx, res.SplitOff.ID, acceptAll(4, 5, 6, 7, 8, 9))
	if err != nil {
		t.Fatal(err)
	}
	if tk := must(f.s.GetTask(ctx, water))(t); tk.ParentTaskID == nil || *tk.ParentTaskID != dig {
		t.Errorf("water %+v, want under dig", tk)
	}
	if b := must(f.s.GetBlocker(ctx, res2.Created[5]))(t); b.ProjectID != garden || b.TaskID == nil || *b.TaskID != water {
		t.Errorf("blocker %+v, want on water", b)
	}
	if i := must(f.s.GetIdea(ctx, res2.Created[7]))(t); i.ProjectID == nil || *i.ProjectID != garden {
		t.Errorf("idea %+v", i)
	}
	if i := must(f.s.GetIdea(ctx, f.iA))(t); i.ProjectID == nil || *i.ProjectID != garden {
		t.Errorf("idea iA %+v", i)
	}
}
