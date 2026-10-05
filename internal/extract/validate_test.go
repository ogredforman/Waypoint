package extract

import (
	"encoding/json"
	"errors"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/ogredforman/waypoint/internal/changeset"
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

// body has multi-byte characters: "é" is bytes 3-4 and "☕" is bytes 6-8.
const body = "café ☕ done: ship it"

func input() ExtractInput {
	return ExtractInput{
		Entry: domain.Entry{ID: 5, Body: body},
		Snapshot: StateSnapshot{
			Projects: []SnapshotProject{{
				ID: 1, Title: "Waypoint", Status: domain.ProjectActive,
				Tasks:    []SnapshotTask{{ID: 10, Title: "Ship it", Status: domain.TaskInProgress}},
				Blockers: []SnapshotBlocker{{ID: 20, Description: "CI is red"}},
			}},
			Ideas: []SnapshotIdea{{ID: 30, Title: "Dark mode"}},
		},
	}
}

func opDraft(t domain.OpType, payload string) ChangeOpDraft {
	return ChangeOpDraft{OpType: t, Payload: json.RawMessage(payload)}
}

func obs(start, end int64, links ...LinkDraft) ObservationDraft {
	return ObservationDraft{Kind: domain.ObservationProgress, Summary: "s", SpanStart: start, SpanEnd: end, Links: links}
}

func link(tt domain.TargetType, id int64) LinkDraft {
	return LinkDraft{TargetType: tt, TargetID: changeset.ID(id), Confidence: ptr(0.9)}
}

func TestValidate(t *testing.T) {
	n := int64(len(body))
	for _, tt := range []struct {
		name string
		res  ExtractionResult
		want []string // fields with problems; nil means valid
	}{
		{"empty", ExtractionResult{}, nil},
		{"valid", ExtractionResult{
			Observations: []ObservationDraft{
				obs(0, n, link(domain.TargetProject, 1), link(domain.TargetTask, 10),
					link(domain.TargetBlocker, 20), link(domain.TargetIdea, 30)),
				obs(6, 9), // exactly the "☕"
				obs(n, n),
			},
			Ops: []ChangeOpDraft{
				{OpType: domain.OpSetTaskStatus, Payload: json.RawMessage(`{"task_id":10,"status":"done"}`),
					Rationale: "said done", Confidence: ptr(1.0)},
				opDraft(domain.OpCreateTask, `{"project_id":1,"title":"Deploy"}`),
				opDraft(domain.OpCreateBlocker, `{"project_id":1,"task_id":"$new:2","description":"d"}`),
				opDraft(domain.OpResolveBlocker, `{"blocker_id":20}`),
				opDraft(domain.OpUpdateIdea, `{"idea_id":30,"project_id":1}`),
				opDraft(domain.OpCreateProject, `{"title":"New"}`),
				opDraft(domain.OpParkIdea, `{"project_id":"$new:6","title":"i"}`),
			},
			Questions: []QuestionDraft{{Text: "What's next?", Reason: "no next step"}, {Text: "q", Reason: "r"}},
		}, nil},

		// observations
		{"observation kind and summary", ExtractionResult{Observations: []ObservationDraft{
			{Kind: "mood", Summary: " ", SpanStart: 0, SpanEnd: 1},
		}}, []string{"observations[0].kind", "observations[0].summary"}},
		{"span out of range", ExtractionResult{Observations: []ObservationDraft{
			obs(-1, n+1), obs(n+1, n+2), obs(5, 4),
		}}, []string{
			"observations[0].span_start", "observations[0].span_end",
			"observations[1].span_start", "observations[1].span_end",
			"observations[2].span_end",
		}},
		{"span splits a character", ExtractionResult{Observations: []ObservationDraft{
			obs(4, 7),
		}}, []string{"observations[0].span_start", "observations[0].span_end"}},

		// links
		{"links", ExtractionResult{Observations: []ObservationDraft{obs(0, 1,
			link(domain.TargetTask, 99),
			link(domain.TargetBlocker, 10), // a task ID, not a blocker's
			link(domain.TargetProject, 0),
			link(domain.TargetMilestone, 1),
			link("epic", 1),
			LinkDraft{TargetType: domain.TargetIdea, TargetID: changeset.ID(30), Confidence: ptr(1.5)},
			LinkDraft{TargetType: domain.TargetIdea, TargetID: changeset.ID(30), Confidence: ptr(math.NaN())},
			LinkDraft{TargetType: domain.TargetIdea, TargetID: changeset.ID(30)},
			link(domain.TargetTask, 10),
			link(domain.TargetTask, 10),
		)}}, []string{
			"observations[0].links[0].target_id",
			"observations[0].links[1].target_id",
			"observations[0].links[2].target_id",
			"observations[0].links[3].target_type",
			"observations[0].links[4].target_type",
			"observations[0].links[5].confidence",
			"observations[0].links[6].confidence",
			"observations[0].links[6].target_id", // idea 30 again
			"observations[0].links[7].confidence",
			"observations[0].links[7].target_id", // idea 30 again
			"observations[0].links[9].target_id", // task 10 again
		}},

		{"temp links", ExtractionResult{
			Observations: []ObservationDraft{obs(0, 1,
				LinkDraft{TargetType: domain.TargetTask, TargetID: changeset.Temp(1), Confidence: ptr(1.0)},
				LinkDraft{TargetType: domain.TargetIdea, TargetID: changeset.Temp(2), Confidence: ptr(1.0)},
				LinkDraft{TargetType: domain.TargetTask, TargetID: changeset.Temp(1), Confidence: ptr(0.5)}, // again
				LinkDraft{TargetType: domain.TargetBlocker, TargetID: changeset.Temp(1), Confidence: ptr(1.0)},
				LinkDraft{TargetType: domain.TargetTask, TargetID: changeset.Temp(3), Confidence: ptr(1.0)}, // creates nothing
				LinkDraft{TargetType: domain.TargetTask, TargetID: changeset.Temp(9), Confidence: ptr(1.0)}, // no such op
			)},
			Ops: []ChangeOpDraft{
				opDraft(domain.OpCreateTask, `{"project_id":1,"title":"t"}`),
				opDraft(domain.OpParkIdea, `{"title":"i"}`),
				opDraft(domain.OpSetTaskStatus, `{"task_id":"$new:1","status":"done"}`),
			},
		}, []string{
			"observations[0].links[2].target_id",
			"observations[0].links[3].target_id",
			"observations[0].links[4].target_id",
			"observations[0].links[5].target_id",
		}},

		// ops
		{"op shape and temp refs", ExtractionResult{Ops: []ChangeOpDraft{
			opDraft(domain.OpCreateTask, `{"project_id":1}`),
			opDraft(domain.OpSetTaskStatus, `{"task_id":"$new:3","status":"done"}`),
			opDraft("rename_task", `{}`),
			opDraft(domain.OpResolveBlocker, `{"blocker_id":"$new:1"}`),
		}}, []string{"ops[1].title", "ops[2].task_id", "ops[3].op_type", "ops[4].blocker_id"}},
		{"delete a project", ExtractionResult{Ops: []ChangeOpDraft{
			opDraft(domain.OpDeleteProject, `{"project_id":1}`),
		}}, []string{"ops[1].op_type"}},
		{"milestones", ExtractionResult{Ops: []ChangeOpDraft{
			opDraft(domain.OpCreateMilestone, `{"project_id":1,"title":"MVP"}`),
			opDraft(domain.OpUpdateMilestone, `{"milestone_id":"$new:1","title":"Beta"}`),
			opDraft(domain.OpDeleteMilestone, `{"milestone_id":"$new:1"}`),
			opDraft(domain.OpCreateTask, `{"project_id":1,"title":"t","milestone_id":1}`),
		}}, []string{"ops[1].op_type", "ops[2].op_type", "ops[3].op_type", "ops[4].milestone_id"}},
		{"op refs not in snapshot", ExtractionResult{Ops: []ChangeOpDraft{
			opDraft(domain.OpCreateTask, `{"project_id":2,"title":"t","parent_task_id":11}`),
			opDraft(domain.OpResolveBlocker, `{"blocker_id":10}`),
			opDraft(domain.OpAddDependency, `{"task_id":10,"blocked_by_task_id":12}`),
			opDraft(domain.OpDeleteIdea, `{"idea_id":1}`),
		}}, []string{
			"ops[1].project_id", "ops[1].parent_task_id", "ops[2].blocker_id",
			"ops[3].blocked_by_task_id", "ops[4].idea_id",
		}},
		{"op confidence", ExtractionResult{Ops: []ChangeOpDraft{
			{OpType: domain.OpDeleteTask, Payload: json.RawMessage(`{"task_id":10}`), Confidence: ptr(-0.1)},
		}}, []string{"ops[1].confidence"}},

		// questions
		{"too many questions", ExtractionResult{Questions: []QuestionDraft{
			{Text: "a", Reason: "r"}, {Text: "b", Reason: "r"}, {Text: "c", Reason: ""},
		}}, []string{"questions", "questions[2].reason"}},
		{"blank question", ExtractionResult{Questions: []QuestionDraft{{Text: " ", Reason: "r"}}},
			[]string{"questions[0].text"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := fields(t, Validate(input(), tt.res))
			slices.Sort(got)
			want := slices.Clone(tt.want)
			slices.Sort(want)
			if !slices.Equal(got, want) {
				t.Errorf("problems in %v, want %v", got, tt.want)
			}
		})
	}
}

// TestValidateMalformedLinkRef checks that a target_id that is neither an ID
// nor "$new:N" is reported against the link, as model output would send it.
func TestValidateMalformedLinkRef(t *testing.T) {
	var res ExtractionResult
	raw := `{"observations":[{"kind":"note","summary":"s","span_start":0,"span_end":1,"links":[
		{"target_type":"task","target_id":"$new:x","confidence":1},
		{"target_type":"task","target_id":"$new:1","confidence":1}]}]}`
	if err := json.Unmarshal([]byte(raw), &res); err != nil {
		t.Fatal(err)
	}
	got := fields(t, Validate(input(), res))
	want := []string{"observations[0].links[0].target_id", "observations[0].links[1].target_id"}
	if !slices.Equal(got, want) {
		t.Errorf("problems in %v, want %v", got, want)
	}
}

// TestValidateReportsEverything checks that one call reports problems from
// every section, so they can all be fed back to a model at once.
func TestValidateReportsEverything(t *testing.T) {
	res := ExtractionResult{
		Observations: []ObservationDraft{obs(0, 100)},
		Ops:          []ChangeOpDraft{opDraft(domain.OpDeleteTask, `{"task_id":99}`)},
		Questions:    make([]QuestionDraft, 3),
	}
	got := fields(t, Validate(input(), res))
	for _, prefix := range []string{"observations[0].", "ops[1].", "questions"} {
		if !slices.ContainsFunc(got, func(f string) bool { return strings.HasPrefix(f, prefix) }) {
			t.Errorf("no problem starting with %q in %v", prefix, got)
		}
	}
}

// TestValidateDoesNotEchoText checks that model-supplied strings and entry
// text never end up in the error, which gets logged.
func TestValidateDoesNotEchoText(t *testing.T) {
	const secret = "my-secret-diary-text"
	in := input()
	in.Entry.Body = secret
	res := ExtractionResult{
		Observations: []ObservationDraft{{Kind: secret, Summary: secret, SpanStart: 0, SpanEnd: 99,
			Links: []LinkDraft{{TargetType: secret, TargetID: changeset.ID(1)}}}},
		Ops: []ChangeOpDraft{
			opDraft(secret, `{}`),
			opDraft(domain.OpSetTaskStatus, `{"task_id":"`+secret+`","status":"`+secret+`"}`),
			opDraft(domain.OpCreateTask, `{"project_id":1,"title":"t","`+secret+`":1}`),
		},
	}
	err := Validate(in, res)
	if err == nil {
		t.Fatal("want an error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("error echoes input text: %v", err)
	}
}

func TestSnapshotHas(t *testing.T) {
	s := input().Snapshot
	for _, tt := range []struct {
		kind domain.TargetType
		id   int64
		want bool
	}{
		{domain.TargetProject, 1, true},
		{domain.TargetProject, 10, false},
		{domain.TargetTask, 10, true},
		{domain.TargetTask, 20, false},
		{domain.TargetBlocker, 20, true},
		{domain.TargetIdea, 30, true},
		{domain.TargetIdea, 1, false},
		{domain.TargetMilestone, 1, false},
	} {
		if got := s.Has(tt.kind, tt.id); got != tt.want {
			t.Errorf("Has(%s, %d) = %v, want %v", tt.kind, tt.id, got, tt.want)
		}
	}
}
