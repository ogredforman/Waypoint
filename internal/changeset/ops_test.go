package changeset

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/ogredforman/waypoint/internal/domain"
)

func ptr[T any](v T) *T { return &v }

// fields returns the fields named in a ValidationError, or nil for a nil error.
func fields(t *testing.T, err error) []string {
	t.Helper()
	if err == nil {
		return nil
	}
	var ve *domain.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error %v is not a *domain.ValidationError", err)
	}
	var out []string
	for _, p := range ve.Problems {
		out = append(out, p.Field)
	}
	return out
}

func TestDecodePayload(t *testing.T) {
	for _, tt := range []struct {
		name    string
		op      domain.OpType
		payload string
		want    []string // fields with problems; nil means valid
	}{
		// create_task
		{"create task", domain.OpCreateTask,
			`{"project_id":1,"title":"Stub /login","notes":"n","size":"s","energy":"low","parent_task_id":"$new:1"}`, nil},
		{"create task missing fields", domain.OpCreateTask, `{}`, []string{"project_id", "title"}},
		{"create task blank title", domain.OpCreateTask, `{"project_id":1,"title":"  "}`, []string{"title"}},
		{"create task bad enums", domain.OpCreateTask,
			`{"project_id":1,"title":"x","size":"xl","energy":"high"}`, []string{"size", "energy"}},
		{"create task milestone", domain.OpCreateTask, `{"project_id":1,"title":"x","milestone_id":"$new:2"}`, nil},
		{"create task bad milestone", domain.OpCreateTask, `{"project_id":1,"title":"x","milestone_id":0}`, []string{"milestone_id"}},
		{"create task bad refs", domain.OpCreateTask,
			`{"project_id":"$new:x","title":"x","parent_task_id":-2}`, []string{"project_id", "parent_task_id"}},
		{"create task status", domain.OpCreateTask, `{"project_id":1,"title":"x","status":"done"}`, []string{"status"}},

		// update_task
		{"update task", domain.OpUpdateTask,
			`{"task_id":1,"title":"t","notes":"","size":null,"energy":"deep","parent_task_id":null,"position":0}`, nil},
		{"update task parent", domain.OpUpdateTask, `{"task_id":1,"parent_task_id":2}`, nil},
		{"update task milestone", domain.OpUpdateTask, `{"task_id":1,"milestone_id":3}`, nil},
		{"update task clear milestone", domain.OpUpdateTask, `{"task_id":1,"milestone_id":null}`, nil},
		{"update task nothing", domain.OpUpdateTask, `{"task_id":1}`, []string{"payload"}},
		{"update task own parent", domain.OpUpdateTask, `{"task_id":"$new:1","parent_task_id":"$new:1"}`, []string{"parent_task_id"}},
		{"update task bad values", domain.OpUpdateTask,
			`{"task_id":1,"title":"","size":"huge","position":-1}`, []string{"title", "size", "position"}},
		{"update task status", domain.OpUpdateTask, `{"task_id":1,"status":"done"}`, []string{"status"}},

		// set_task_status
		{"set status", domain.OpSetTaskStatus, `{"task_id":1,"status":"done"}`, nil},
		{"set status bad", domain.OpSetTaskStatus, `{"task_id":1,"status":"overdue"}`, []string{"status"}},
		{"set status missing", domain.OpSetTaskStatus, `{}`, []string{"task_id", "status"}},

		// split_task
		{"split", domain.OpSplitTask, `{"task_id":1,"subtasks":[{"title":"a","size":"xs"},{"title":"b"}]}`, nil},
		{"split no subtasks", domain.OpSplitTask, `{"task_id":1,"subtasks":[]}`, []string{"subtasks"}},
		{"split bad subtask", domain.OpSplitTask,
			`{"task_id":1,"subtasks":[{"title":"a"},{"title":"","energy":"max"}]}`,
			[]string{"subtasks[1].title", "subtasks[1].energy"}},

		// add_dependency
		{"dependency", domain.OpAddDependency, `{"task_id":1,"blocked_by_task_id":"$new:2"}`, nil},
		{"dependency self", domain.OpAddDependency, `{"task_id":3,"blocked_by_task_id":3}`, []string{"blocked_by_task_id"}},
		{"dependency missing", domain.OpAddDependency, `{"task_id":3}`, []string{"blocked_by_task_id"}},

		// delete_task
		{"delete task", domain.OpDeleteTask, `{"task_id":1}`, nil},
		{"delete task missing", domain.OpDeleteTask, `{}`, []string{"task_id"}},

		// create_blocker
		{"create blocker", domain.OpCreateBlocker, `{"project_id":1,"task_id":2,"description":"waiting on API key"}`, nil},
		{"create blocker missing", domain.OpCreateBlocker, `{"task_id":2}`, []string{"project_id", "description"}},

		// update_blocker
		{"update blocker", domain.OpUpdateBlocker, `{"blocker_id":1,"description":"d","task_id":null}`, nil},
		{"update blocker nothing", domain.OpUpdateBlocker, `{"blocker_id":1}`, []string{"payload"}},
		{"update blocker blank", domain.OpUpdateBlocker, `{"blocker_id":1,"description":""}`, []string{"description"}},
		{"update blocker status", domain.OpUpdateBlocker, `{"blocker_id":1,"status":"open"}`, []string{"status"}},

		// resolve_blocker, delete_blocker
		{"resolve blocker", domain.OpResolveBlocker, `{"blocker_id":"$new:3"}`, nil},
		{"resolve blocker missing", domain.OpResolveBlocker, `{}`, []string{"blocker_id"}},
		{"delete blocker", domain.OpDeleteBlocker, `{"blocker_id":1}`, nil},
		{"delete blocker bad", domain.OpDeleteBlocker, `{"blocker_id":"one"}`, []string{"blocker_id"}},

		// park_idea
		{"park idea", domain.OpParkIdea, `{"title":"Voice entry","body":"later"}`, nil},
		{"park idea on project", domain.OpParkIdea, `{"project_id":1,"title":"Voice entry"}`, nil},
		{"park idea missing", domain.OpParkIdea, `{"body":"b"}`, []string{"title"}},

		// update_idea
		{"update idea", domain.OpUpdateIdea, `{"idea_id":1,"status":"promoted","project_id":null}`, nil},
		{"update idea nothing", domain.OpUpdateIdea, `{"idea_id":1}`, []string{"payload"}},
		{"update idea bad", domain.OpUpdateIdea, `{"idea_id":1,"title":"","status":"active"}`, []string{"title", "status"}},

		// delete_idea
		{"delete idea", domain.OpDeleteIdea, `{"idea_id":1}`, nil},
		{"delete idea missing", domain.OpDeleteIdea, `{}`, []string{"idea_id"}},

		// create_project
		{"create project", domain.OpCreateProject, `{"title":"Waypoint","why":"follow through"}`, nil},
		{"create project missing", domain.OpCreateProject, `{"why":"w"}`, []string{"title"}},
		{"create project status", domain.OpCreateProject, `{"title":"x","status":"active"}`, []string{"status"}},

		// update_project
		{"update project", domain.OpUpdateProject,
			`{"project_id":"$new:1","title":"t","why":"","definition_of_done":"d","status":"active"}`, nil},
		{"update project nothing", domain.OpUpdateProject, `{"project_id":1}`, []string{"payload"}},
		{"update project bad", domain.OpUpdateProject, `{"project_id":1,"status":"abandoned"}`, []string{"status"}},

		// create_milestone
		{"create milestone", domain.OpCreateMilestone, `{"project_id":"$new:1","title":"MVP","description":"d"}`, nil},
		{"create milestone missing", domain.OpCreateMilestone, `{"title":" "}`, []string{"project_id", "title"}},
		{"create milestone status", domain.OpCreateMilestone, `{"project_id":1,"title":"x","status":"done"}`, []string{"status"}},

		// update_milestone
		{"update milestone", domain.OpUpdateMilestone,
			`{"milestone_id":1,"title":"t","description":"","status":"done","position":0}`, nil},
		{"update milestone nothing", domain.OpUpdateMilestone, `{"milestone_id":1}`, []string{"payload"}},
		{"update milestone bad", domain.OpUpdateMilestone,
			`{"milestone_id":1,"title":"","status":"late","position":-1}`, []string{"title", "status", "position"}},

		// delete_milestone
		{"delete milestone", domain.OpDeleteMilestone, `{"milestone_id":"$new:1"}`, nil},
		{"delete milestone missing", domain.OpDeleteMilestone, `{}`, []string{"milestone_id"}},

		// decoding problems
		{"wrong type", domain.OpCreateTask, `{"project_id":1,"title":5}`, []string{"title"}},
		{"not an object", domain.OpDeleteTask, `[1]`, []string{"payload"}},
		{"null", domain.OpDeleteTask, `null`, []string{"payload"}},
		{"invalid JSON", domain.OpDeleteTask, `{"task_id":`, []string{"payload"}},
		{"trailing data", domain.OpDeleteTask, `{"task_id":1} {}`, []string{"payload"}},
		{"odd unknown field", domain.OpDeleteTask, `{"task_id":1,"I felt awful today":1}`, []string{"payload"}},
		{"unknown op type", domain.OpType("move_project"), `{"project_id":1}`, []string{"op_type"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p, err := DecodePayload(tt.op, json.RawMessage(tt.payload))
			if got := fields(t, err); !slices.Equal(got, tt.want) {
				t.Errorf("problem fields %v, want %v (%v)", got, tt.want, err)
			}
			if tt.want == nil && p.OpType() != tt.op {
				t.Errorf("payload op type %s, want %s", p.OpType(), tt.op)
			}
		})
	}
}

// TestDecodeRoundTrip checks that a decoded payload marshals back to the same
// JSON, so what the applier records in applied_payload matches the proposal.
func TestDecodeRoundTrip(t *testing.T) {
	for op, payload := range map[domain.OpType]string{
		domain.OpCreateTask:     `{"project_id":"$new:1","parent_task_id":4,"title":"a","notes":"n","size":"m","energy":"low"}`,
		domain.OpUpdateTask:     `{"task_id":1,"size":null,"parent_task_id":"$new:2","milestone_id":null,"position":3}`,
		domain.OpCreateMilestone: `{"project_id":"$new:1","title":"MVP","description":"d"}`,
		domain.OpUpdateMilestone: `{"milestone_id":2,"status":"dropped","position":0}`,
		domain.OpSplitTask:      `{"task_id":1,"subtasks":[{"title":"a"},{"title":"b","size":"xs"}]}`,
		domain.OpUpdateBlocker:  `{"blocker_id":2,"task_id":null}`,
		domain.OpUpdateIdea:     `{"idea_id":3,"project_id":5,"status":"promoted"}`,
		domain.OpUpdateProject:  `{"project_id":1,"status":"active"}`,
		domain.OpResolveBlocker: `{"blocker_id":"$new:7"}`,
	} {
		t.Run(string(op), func(t *testing.T) {
			p, err := DecodePayload(op, json.RawMessage(payload))
			if err != nil {
				t.Fatal(err)
			}
			out, err := json.Marshal(p)
			if err != nil {
				t.Fatal(err)
			}
			if string(out) != payload {
				t.Errorf("round trip %s, want %s", out, payload)
			}
		})
	}
}

func TestEveryOpTypeHasPayload(t *testing.T) {
	for _, op := range domain.OpTypes() {
		p, err := DecodePayload(op, json.RawMessage(`{}`))
		// An empty payload is invalid for most ops, but still decodes.
		if slices.Contains(fields(t, err), "op_type") || p == nil {
			t.Errorf("%s: no payload type", op)
			continue
		}
		if p.OpType() != op {
			t.Errorf("%s decodes to a %s payload", op, p.OpType())
		}
	}
}

func TestCreates(t *testing.T) {
	for _, tt := range []struct {
		p    Payload
		want domain.TargetType
	}{
		{CreateTask{}, domain.TargetTask},
		{CreateBlocker{}, domain.TargetBlocker},
		{ParkIdea{}, domain.TargetIdea},
		{CreateProject{}, domain.TargetProject},
		{CreateMilestone{}, domain.TargetMilestone},
		{UpdateMilestone{}, ""},
		{SplitTask{}, ""},
		{UpdateTask{}, ""},
	} {
		got, ok := tt.p.Creates()
		if got != tt.want || ok != (tt.want != "") {
			t.Errorf("%T creates %q (%v), want %q", tt.p, got, ok, tt.want)
		}
	}
}

func TestRefs(t *testing.T) {
	p := CreateTask{ProjectID: Temp(1), ParentTaskID: ptr(ID(7)), Title: "t"}
	want := []FieldRef{
		{Field: "project_id", Kind: domain.TargetProject, Ref: Temp(1)},
		{Field: "parent_task_id", Kind: domain.TargetTask, Ref: ID(7)},
	}
	if got := Refs(p); !slices.Equal(got, want) {
		t.Errorf("Refs = %v, want %v", got, want)
	}
	if got := Refs(CreateProject{Title: "t"}); len(got) != 0 {
		t.Errorf("Refs(CreateProject) = %v, want none", got)
	}
}
