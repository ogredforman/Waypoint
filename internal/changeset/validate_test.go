package changeset

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/ogredforman/waypoint/internal/db"
	"github.com/ogredforman/waypoint/internal/domain"
	"github.com/ogredforman/waypoint/internal/store"
)

func op(pos int64, t domain.OpType, payload string) domain.ChangeOp {
	return domain.ChangeOp{Position: pos, OpType: t, Payload: json.RawMessage(payload)}
}

func TestValidateOps(t *testing.T) {
	for _, tt := range []struct {
		name string
		ops  []domain.ChangeOp
		want []string
	}{
		{"empty", nil, nil},
		{"temp refs to earlier creates", []domain.ChangeOp{
			op(1, domain.OpCreateProject, `{"title":"Waypoint"}`),
			op(2, domain.OpUpdateProject, `{"project_id":"$new:1","status":"active"}`),
			op(3, domain.OpCreateTask, `{"project_id":"$new:1","title":"Write the spec"}`),
			op(4, domain.OpCreateTask, `{"project_id":"$new:1","title":"Draft","parent_task_id":"$new:3"}`),
			op(5, domain.OpSetTaskStatus, `{"task_id":"$new:4","status":"done"}`),
			op(6, domain.OpCreateBlocker, `{"project_id":"$new:1","task_id":"$new:3","description":"d"}`),
			op(7, domain.OpResolveBlocker, `{"blocker_id":"$new:6"}`),
			op(8, domain.OpParkIdea, `{"project_id":"$new:1","title":"i"}`),
			op(9, domain.OpUpdateIdea, `{"idea_id":"$new:8","status":"discarded"}`),
			op(10, domain.OpAddDependency, `{"task_id":"$new:4","blocked_by_task_id":"$new:3"}`),
		}, nil},
		{"forward ref", []domain.ChangeOp{
			op(1, domain.OpSetTaskStatus, `{"task_id":"$new:2","status":"done"}`),
			op(2, domain.OpCreateTask, `{"project_id":1,"title":"x"}`),
		}, []string{"ops[1].task_id"}},
		{"self ref", []domain.ChangeOp{
			op(1, domain.OpCreateTask, `{"project_id":1,"title":"x","parent_task_id":"$new:1"}`),
		}, []string{"ops[1].parent_task_id"}},
		{"wrong kind", []domain.ChangeOp{
			op(1, domain.OpCreateProject, `{"title":"p"}`),
			op(2, domain.OpSetTaskStatus, `{"task_id":"$new:1","status":"done"}`),
		}, []string{"ops[2].task_id"}},
		{"ref to non-creating op", []domain.ChangeOp{
			op(1, domain.OpSplitTask, `{"task_id":1,"subtasks":[{"title":"a"}]}`),
			op(2, domain.OpSetTaskStatus, `{"task_id":"$new:1","status":"done"}`),
		}, []string{"ops[2].task_id"}},
		{"ref to missing position", []domain.ChangeOp{
			op(2, domain.OpCreateTask, `{"project_id":1,"title":"x"}`),
			op(3, domain.OpSetTaskStatus, `{"task_id":"$new:1","status":"done"}`),
		}, []string{"ops[3].task_id"}},
		{"ref to invalid creating op", []domain.ChangeOp{
			op(1, domain.OpCreateTask, `{"project_id":1}`),
			op(2, domain.OpSetTaskStatus, `{"task_id":"$new:1","status":"done"}`),
		}, []string{"ops[1].title"}},
		{"bad positions", []domain.ChangeOp{
			op(0, domain.OpDeleteTask, `{"task_id":1}`),
			op(2, domain.OpDeleteTask, `{"task_id":2}`),
			op(2, domain.OpDeleteTask, `{"task_id":3}`),
		}, []string{"ops[0].position", "ops[2].position"}},
		{"problems from every op", []domain.ChangeOp{
			op(1, domain.OpCreateTask, `{"project_id":1,"title":""}`),
			op(2, domain.OpType("rename_task"), `{}`),
			op(3, domain.OpSetTaskStatus, `{"task_id":1,"status":"late"}`),
		}, []string{"ops[1].title", "ops[2].op_type", "ops[3].status"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			payloads, err := ValidateOps(tt.ops)
			if got := fields(t, err); !slices.Equal(got, tt.want) {
				t.Errorf("problem fields %v, want %v (%v)", got, tt.want, err)
			}
			if len(payloads) != len(tt.ops) {
				t.Errorf("%d payloads for %d ops", len(payloads), len(tt.ops))
			}
		})
	}
}

// fixture is a migrated temp database with two projects:
//
//	project A: tasks a1 > a2 > a3 (parent chain), a4; blocker bA on a1; idea iA
//	project B: task b1; blocker bB
type fixture struct {
	db                 *sql.DB
	s                  *store.Store
	projA, projB       int64
	a1, a2, a3, a4, b1 int64
	bA, bB, iA         int64
}

func newFixture(t *testing.T) fixture {
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
	f := fixture{db: sqlDB, s: store.New(sqlDB, func() time.Time { return time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC) })}

	err = f.s.InTx(ctx, func(tx *store.Tx) error {
		project := func(title string) int64 {
			p, err := tx.CreateProject(ctx, domain.Project{Title: title, Status: domain.ProjectActive})
			if err != nil {
				t.Fatal(err)
			}
			return p.ID
		}
		task := func(projectID int64, parent *int64) int64 {
			tk, err := tx.CreateTask(ctx, domain.Task{ProjectID: projectID, ParentTaskID: parent, Title: "t", Status: domain.TaskTodo})
			if err != nil {
				t.Fatal(err)
			}
			return tk.ID
		}
		blocker := func(projectID int64, taskID *int64) int64 {
			b, err := tx.CreateBlocker(ctx, domain.Blocker{ProjectID: projectID, TaskID: taskID, Description: "d", Status: domain.BlockerOpen})
			if err != nil {
				t.Fatal(err)
			}
			return b.ID
		}
		f.projA, f.projB = project("A"), project("B")
		f.a1 = task(f.projA, nil)
		f.a2 = task(f.projA, &f.a1)
		f.a3 = task(f.projA, &f.a2)
		f.a4 = task(f.projA, nil)
		f.b1 = task(f.projB, nil)
		f.bA = blocker(f.projA, &f.a1)
		f.bB = blocker(f.projB, nil)
		i, err := tx.CreateIdea(ctx, domain.Idea{ProjectID: &f.projA, Title: "i", Status: domain.IdeaParked})
		f.iA = i.ID
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestCheckRefs(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	const missing = 999
	temps := TempIDs{1: f.a4, 2: f.projB}

	for _, tt := range []struct {
		name string
		p    Payload
		want []string
	}{
		// existence, every kind
		{"all exist", UpdateIdea{IdeaID: ID(f.iA), ProjectID: To(ID(f.projB))}, nil},
		{"missing project", CreateTask{ProjectID: ID(missing), Title: "x"}, []string{"project_id"}},
		{"missing task", SetTaskStatus{TaskID: ID(missing), Status: domain.TaskDone}, []string{"task_id"}},
		{"missing blocker", ResolveBlocker{BlockerID: ID(missing)}, []string{"blocker_id"}},
		{"missing idea", DeleteIdea{IdeaID: ID(missing)}, []string{"idea_id"}},
		{"missing optional", ParkIdea{ProjectID: ptr(ID(missing)), Title: "x"}, []string{"project_id"}},
		{"cleared ref not checked", UpdateBlocker{BlockerID: ID(f.bA), TaskID: Clear[Ref]()}, nil},

		// temp IDs
		{"temp resolves", SetTaskStatus{TaskID: Temp(1), Status: domain.TaskDone}, nil},
		{"temp not applied", SetTaskStatus{TaskID: Temp(3), Status: domain.TaskDone}, []string{"task_id"}},

		// same project
		{"create task parent same project", CreateTask{ProjectID: ID(f.projA), ParentTaskID: ptr(ID(f.a1)), Title: "x"}, nil},
		{"create task parent other project", CreateTask{ProjectID: ID(f.projA), ParentTaskID: ptr(ID(f.b1)), Title: "x"}, []string{"parent_task_id"}},
		{"create task parent via temp project", CreateTask{ProjectID: Temp(2), ParentTaskID: ptr(ID(f.a1)), Title: "x"}, []string{"parent_task_id"}},
		{"create blocker task same project", CreateBlocker{ProjectID: ID(f.projA), TaskID: ptr(ID(f.a2)), Description: "d"}, nil},
		{"create blocker task other project", CreateBlocker{ProjectID: ID(f.projB), TaskID: ptr(ID(f.a2)), Description: "d"}, []string{"task_id"}},
		{"update blocker task same project", UpdateBlocker{BlockerID: ID(f.bA), TaskID: To(ID(f.a4))}, nil},
		{"update blocker task other project", UpdateBlocker{BlockerID: ID(f.bB), TaskID: To(ID(f.a4))}, []string{"task_id"}},
		{"dependency same project", AddDependency{TaskID: ID(f.a1), BlockedByTaskID: ID(f.a4)}, nil},
		{"dependency other project", AddDependency{TaskID: ID(f.a1), BlockedByTaskID: ID(f.b1)}, []string{"blocked_by_task_id"}},
		{"dependency self after resolve", AddDependency{TaskID: ID(f.a4), BlockedByTaskID: Temp(1)}, []string{"blocked_by_task_id"}},
		{"update task parent other project", UpdateTask{TaskID: ID(f.a4), ParentTaskID: To(ID(f.b1))}, []string{"parent_task_id"}},

		// cycles
		{"reparent to sibling", UpdateTask{TaskID: ID(f.a4), ParentTaskID: To(ID(f.a3))}, nil},
		{"reparent to self", UpdateTask{TaskID: ID(f.a2), ParentTaskID: To(ID(f.a2))}, []string{"parent_task_id"}},
		{"reparent to child", UpdateTask{TaskID: ID(f.a2), ParentTaskID: To(ID(f.a3))}, []string{"parent_task_id"}},
		{"reparent to grandchild", UpdateTask{TaskID: ID(f.a1), ParentTaskID: To(ID(f.a3))}, []string{"parent_task_id"}},
		{"reparent under own parent", UpdateTask{TaskID: ID(f.a3), ParentTaskID: To(ID(f.a1))}, nil},
		{"make top-level", UpdateTask{TaskID: ID(f.a3), ParentTaskID: Clear[Ref]()}, nil},
		{"reparent temp task", UpdateTask{TaskID: Temp(1), ParentTaskID: To(ID(f.a3))}, nil},
		{"reparent temp task other project", UpdateTask{TaskID: Temp(1), ParentTaskID: To(ID(f.b1))}, []string{"parent_task_id"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			rerr := f.s.InTx(ctx, func(tx *store.Tx) error {
				err = CheckRefs(ctx, tx, tt.p, temps)
				return nil
			})
			if rerr != nil {
				t.Fatal(rerr)
			}
			if got := fields(t, err); !slices.Equal(got, tt.want) {
				t.Errorf("problem fields %v, want %v (%v)", got, tt.want, err)
			}
		})
	}
}

func TestCheckRefsIdeaStatus(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	ideas := map[domain.IdeaStatus]int64{}
	err := f.s.InTx(ctx, func(tx *store.Tx) error {
		for _, st := range domain.IdeaStatuses() {
			i, err := tx.CreateIdea(ctx, domain.Idea{Title: "i", Status: st})
			if err != nil {
				return err
			}
			ideas[st] = i.ID
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	set := func(from, to domain.IdeaStatus) UpdateIdea {
		return UpdateIdea{IdeaID: ID(ideas[from]), Status: &to}
	}
	title := "renamed"
	for _, tt := range []struct {
		name string
		p    UpdateIdea
		want []string
	}{
		{"promote parked", set(domain.IdeaParked, domain.IdeaPromoted), nil},
		{"discard parked", set(domain.IdeaParked, domain.IdeaDiscarded), nil},
		{"park discarded", set(domain.IdeaDiscarded, domain.IdeaParked), nil},
		{"promote discarded", set(domain.IdeaDiscarded, domain.IdeaPromoted), []string{"status"}},
		{"promote again", set(domain.IdeaPromoted, domain.IdeaPromoted), []string{"status"}},
		{"unpromote", set(domain.IdeaPromoted, domain.IdeaParked), []string{"status"}},
		{"rename promoted", UpdateIdea{IdeaID: ID(ideas[domain.IdeaPromoted]), Title: &title}, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			rerr := f.s.InTx(ctx, func(tx *store.Tx) error {
				err = CheckRefs(ctx, tx, tt.p, nil)
				return nil
			})
			if rerr != nil {
				t.Fatal(rerr)
			}
			if got := fields(t, err); !slices.Equal(got, tt.want) {
				t.Errorf("problem fields %v, want %v (%v)", got, tt.want, err)
			}
		})
	}
}

// TestCheckRefsSeesEarlierOps checks that CheckRefs reads through the apply
// transaction, so rows written by earlier ops count.
func TestCheckRefsSeesEarlierOps(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	err := f.s.InTx(ctx, func(tx *store.Tx) error {
		// An earlier op in the changeset deletes a2 (a3 becomes top-level)...
		if _, err := tx.DeleteTask(ctx, f.a2); err != nil {
			return err
		}
		if err := CheckRefs(ctx, tx, SetTaskStatus{TaskID: ID(f.a2), Status: domain.TaskDone}, nil); !slices.Equal(fields(t, err), []string{"task_id"}) {
			t.Errorf("deleted task: got %v, want a task_id problem", err)
		}
		// ...so putting a1 under a3 no longer makes a cycle.
		if err := CheckRefs(ctx, tx, UpdateTask{TaskID: ID(f.a1), ParentTaskID: To(ID(f.a3))}, nil); err != nil {
			t.Errorf("reparent after delete: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// withCap stores limit as the active-project cap in s's settings and returns
// an Applier over s.
func withCap(t *testing.T, s *store.Store, limit int, after AfterApply) *Applier {
	t.Helper()
	ctx := context.Background()
	err := s.InTx(ctx, func(tx *store.Tx) error {
		set, err := tx.GetSettings(ctx)
		if err != nil {
			return err
		}
		set.ActiveProjectCap = limit
		_, err = tx.UpdateSettings(ctx, set)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return NewApplier(s, after)
}
