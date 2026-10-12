package eval

import (
	"encoding/json"
	"math"
	"slices"
	"testing"

	"github.com/ogredforman/waypoint/internal/changeset"
	"github.com/ogredforman/waypoint/internal/domain"
	"github.com/ogredforman/waypoint/internal/extract"
)

func op(t domain.OpType, payload string) extract.ChangeOpDraft {
	return extract.ChangeOpDraft{OpType: t, Payload: json.RawMessage(payload)}
}

func linked(refs ...any) extract.ObservationDraft {
	o := extract.ObservationDraft{Kind: domain.ObservationProgress, Summary: "s"}
	for i := 0; i < len(refs); i += 2 {
		c := 0.9
		o.Links = append(o.Links, extract.LinkDraft{TargetType: refs[i].(domain.TargetType), TargetID: refs[i+1].(changeset.Ref), Confidence: &c})
	}
	return o
}

func TestScoreResult(t *testing.T) {
	f := Fixture{
		Name: "x",
		Expected: extract.ExtractionResult{
			Observations: []extract.ObservationDraft{
				linked(domain.TargetTask, changeset.ID(10)),
				linked(domain.TargetProject, changeset.ID(1), domain.TargetTask, changeset.Temp(2)),
			},
			Ops: []extract.ChangeOpDraft{
				op(domain.OpSetTaskStatus, `{"task_id":10,"status":"done"}`),
				op(domain.OpCreateTask, `{"project_id":1,"title":"Write the eval command"}`),
			},
			Questions: []extract.QuestionDraft{{Text: "q", Reason: "r"}},
		},
		Forbid: []domain.OpType{domain.OpDeleteTask},
	}

	t.Run("same rows, other words", func(t *testing.T) {
		got := extract.ExtractionResult{
			// Observations split differently, links to the same rows; the
			// temp ref names another position.
			Observations: []extract.ObservationDraft{
				linked(domain.TargetTask, changeset.ID(10), domain.TargetProject, changeset.ID(1)),
				linked(domain.TargetTask, changeset.Temp(1), domain.TargetTask, changeset.ID(10)),
			},
			Ops: []extract.ChangeOpDraft{
				op(domain.OpCreateTask, `{"title":"Build eval","project_id":1}`),
				op(domain.OpSetTaskStatus, `{"status":"done","task_id":10}`),
			},
		}
		s := ScoreResult(f, got)
		if s.Ops != (Counts{Matched: 2}) || s.Links != (Counts{Matched: 3}) {
			t.Errorf("ops %+v links %+v, want all matched", s.Ops, s.Links)
		}
		if s.Questions != 0 || s.ExpectedQuestions != 1 || len(s.Forbidden) != 0 {
			t.Errorf("questions %d/%d forbidden %v", s.Questions, s.ExpectedQuestions, s.Forbidden)
		}
	})

	t.Run("wrong rows and a forbidden op", func(t *testing.T) {
		got := extract.ExtractionResult{
			Observations: []extract.ObservationDraft{linked(domain.TargetTask, changeset.ID(11))},
			Ops: []extract.ChangeOpDraft{
				op(domain.OpSetTaskStatus, `{"task_id":10,"status":"doing"}`),
				op(domain.OpDeleteTask, `{"task_id":12}`),
			},
		}
		s := ScoreResult(f, got)
		if s.Ops != (Counts{Missing: 2, Extra: 2}) {
			t.Errorf("ops %+v", s.Ops)
		}
		if s.Links != (Counts{Missing: 3, Extra: 1}) {
			t.Errorf("links %+v", s.Links)
		}
		if !slices.Equal(s.Forbidden, []domain.OpType{domain.OpDeleteTask}) {
			t.Errorf("forbidden %v", s.Forbidden)
		}
		if !slices.Contains(s.MissingOps, "set_task_status{status=done task_id=10}") ||
			!slices.Contains(s.ExtraOps, "delete_task{task_id=12}") {
			t.Errorf("missing %v extra %v", s.MissingOps, s.ExtraOps)
		}
		if !slices.Equal(s.ExtraLinks, []string{"task 11"}) {
			t.Errorf("extra links %v", s.ExtraLinks)
		}
	})

	t.Run("duplicates match once", func(t *testing.T) {
		got := extract.ExtractionResult{Ops: []extract.ChangeOpDraft{
			op(domain.OpSetTaskStatus, `{"task_id":10,"status":"done"}`),
			op(domain.OpSetTaskStatus, `{"task_id":10,"status":"done"}`),
		}}
		if s := ScoreResult(f, got); s.Ops != (Counts{Matched: 1, Missing: 1, Extra: 1}) {
			t.Errorf("ops %+v", s.Ops)
		}
	})
}

func TestOpKey(t *testing.T) {
	for _, tc := range []struct {
		op   extract.ChangeOpDraft
		want string
	}{
		{op(domain.OpUpdateTask, `{"task_id":10,"milestone_id":null,"title":"t"}`), "update_task{milestone_id=none task_id=10}"},
		{op(domain.OpCreateBlocker, `{"project_id":1,"task_id":"$new:3"}`), "create_blocker{project_id=1 task_id=new}"},
		{op(domain.OpParkIdea, `{"title":"t"}`), "park_idea{}"},
		{op(domain.OpParkIdea, `[1]`), "park_idea{?}"},
	} {
		if got := opKey(tc.op); got != tc.want {
			t.Errorf("opKey(%s) = %q, want %q", tc.op.Payload, got, tc.want)
		}
	}
}

func TestCounts(t *testing.T) {
	for _, tc := range []struct {
		c       Counts
		p, r, f float64
	}{
		{Counts{}, 1, 1, 1}, // nothing expected, nothing proposed
		{Counts{Matched: 1, Missing: 1}, 1, 0.5, 2.0 / 3},
		{Counts{Extra: 2}, 0, 1, 0},
		{Counts{Missing: 1, Extra: 1}, 0, 0, 0},
	} {
		if !near(tc.c.Precision(), tc.p) || !near(tc.c.Recall(), tc.r) || !near(tc.c.F1(), tc.f) {
			t.Errorf("%+v: P %v R %v F1 %v, want %v %v %v", tc.c, tc.c.Precision(), tc.c.Recall(), tc.c.F1(), tc.p, tc.r, tc.f)
		}
	}
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }
