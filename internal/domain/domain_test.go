package domain

import (
	"encoding/json"
	"errors"
	"math"
	"slices"
	"strings"
	"testing"
)

func ptr[T any](v T) *T { return &v }

// fields returns the fields named in a ValidationError, or nil for a nil error.
func fields(t *testing.T, err error) []string {
	t.Helper()
	if err == nil {
		return nil
	}
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error %v is not a *ValidationError", err)
	}
	var out []string
	for _, p := range ve.Problems {
		out = append(out, p.Field)
	}
	return out
}

type validator interface{ Validate() error }

type validateCase struct {
	name string
	v    validator
	want []string // fields with problems; nil means valid
}

func runValidate(t *testing.T, cases []validateCase) {
	t.Helper()
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := fields(t, tt.v.Validate()); !slices.Equal(got, tt.want) {
				t.Errorf("problem fields %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEnumValid(t *testing.T) {
	for _, tt := range []struct {
		name   string
		good   bool
		bad    bool
		values int
	}{
		{"ProjectStatus", ProjectParked.Valid(), ProjectStatus("archived").Valid(), len(ProjectStatuses())},
		{"MilestoneStatus", MilestoneDropped.Valid(), MilestoneStatus("late").Valid(), len(MilestoneStatuses())},
		{"TaskStatus", TaskInProgress.Valid(), TaskStatus("overdue").Valid(), len(TaskStatuses())},
		{"TaskSize", SizeXS.Valid(), TaskSize("xl").Valid(), len(TaskSizes())},
		{"Energy", EnergyDeep.Valid(), Energy("high").Valid(), len(Energies())},
		{"ExtractionRunStatus", RunRunning.Valid(), ExtractionRunStatus("done").Valid(), len(ExtractionRunStatuses())},
		{"QuestionStatus", QuestionDismissed.Valid(), QuestionStatus("ignored").Valid(), len(QuestionStatuses())},
		{"ObservationKind", ObservationFeeling.Valid(), ObservationKind("rant").Valid(), len(ObservationKinds())},
		{"TargetType", TargetIdea.Valid(), TargetType("entry").Valid(), len(TargetTypes())},
		{"BlockerStatus", BlockerResolved.Valid(), BlockerStatus("stuck").Valid(), len(BlockerStatuses())},
		{"IdeaStatus", IdeaPromoted.Valid(), IdeaStatus("active").Valid(), len(IdeaStatuses())},
		{"ChangesetSource", SourceManual.Valid(), ChangesetSource("import").Valid(), len(ChangesetSources())},
		{"ChangesetStatus", ChangesetPartiallyApplied.Valid(), ChangesetStatus("done").Valid(), len(ChangesetStatuses())},
		{"OpType", OpAddDependency.Valid(), OpType("delete_task").Valid(), len(OpTypes())},
		{"ChangeOpStatus", OpEdited.Valid(), ChangeOpStatus("maybe").Valid(), len(ChangeOpStatuses())},
		{"BriefingSource", BriefingLLM.Valid(), BriefingSource("guess").Valid(), len(BriefingSources())},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.good {
				t.Error("known value reported invalid")
			}
			if tt.bad {
				t.Error("unknown value reported valid")
			}
			if tt.values == 0 {
				t.Error("no values listed")
			}
		})
	}
	// The zero value is never a valid enum, so a forgotten field is caught.
	if TaskStatus("").Valid() || OpType("").Valid() {
		t.Error("empty string reported valid")
	}
}

func TestValidationErrorMessage(t *testing.T) {
	err := Project{Title: " ", Status: "archived"}.Validate()
	want := `invalid project: title: is required; status: is not an allowed value`
	if err == nil || err.Error() != want {
		t.Errorf("got %v, want %q", err, want)
	}
}

func TestProjectValidate(t *testing.T) {
	ok := Project{Title: "Build Waypoint", Status: ProjectActive}
	blank := ok
	blank.Title = "\t\n"
	bad := ok
	bad.Status = "archived"
	runValidate(t, []validateCase{
		{"valid", ok, nil},
		{"blank title", blank, []string{"title"}},
		{"bad status", bad, []string{"status"}},
		{"zero value", Project{}, []string{"title", "status"}},
	})
}

func TestMilestoneValidate(t *testing.T) {
	ok := Milestone{ProjectID: 1, Title: "MVP", Status: MilestoneOpen}
	noProject := ok
	noProject.ProjectID = 0
	runValidate(t, []validateCase{
		{"valid", ok, nil},
		{"no project", noProject, []string{"project_id"}},
		{"zero value", Milestone{}, []string{"project_id", "title", "status"}},
	})
}

func TestTaskValidate(t *testing.T) {
	ok := Task{
		ID: 3, ProjectID: 1, MilestoneID: ptr(int64(1)), ParentTaskID: ptr(int64(2)),
		Title: "Stub the /login route", Status: TaskTodo,
		Size: ptr(SizeS), Energy: ptr(EnergyLow), OriginEntryID: ptr(int64(1)),
	}
	minimal := Task{ProjectID: 1, Title: "x", Status: TaskDone}
	ownParent := ok
	ownParent.ParentTaskID = ptr(int64(3))
	unsavedParent := minimal
	unsavedParent.ParentTaskID = ptr(int64(0))
	badEnums := ok
	badEnums.Size, badEnums.Energy = ptr(TaskSize("xl")), ptr(Energy("high"))
	badRefs := ok
	badRefs.MilestoneID, badRefs.OriginEntryID = ptr(int64(-1)), ptr(int64(0))
	runValidate(t, []validateCase{
		{"valid", ok, nil},
		{"minimal", minimal, nil},
		{"own parent", ownParent, []string{"parent_task_id"}},
		{"zero parent reference", unsavedParent, []string{"parent_task_id"}},
		{"bad size and energy", badEnums, []string{"size", "energy"}},
		{"bad optional references", badRefs, []string{"milestone_id", "origin_entry_id"}},
		{"zero value", Task{}, []string{"project_id", "title", "status"}},
	})
}

func TestTaskDependencyValidate(t *testing.T) {
	runValidate(t, []validateCase{
		{"valid", TaskDependency{TaskID: 2, BlockedByTaskID: 1}, nil},
		{"self", TaskDependency{TaskID: 1, BlockedByTaskID: 1}, []string{"blocked_by_task_id"}},
		{"missing blocker", TaskDependency{TaskID: 1}, []string{"blocked_by_task_id"}},
	})
}

func TestEntryValidate(t *testing.T) {
	runValidate(t, []validateCase{
		{"valid", Entry{Body: "todo: write the schema", ProjectID: ptr(int64(1))}, nil},
		{"auto-scoped reply", Entry{Body: "yes", ReplyToQuestionID: ptr(int64(1))}, nil},
		{"blank body", Entry{Body: "  "}, []string{"body"}},
		{"bad project", Entry{Body: "x", ProjectID: ptr(int64(0))}, []string{"project_id"}},
	})
}

func TestObservationValidate(t *testing.T) {
	ok := Observation{EntryID: 1, ExtractionRunID: 1, Kind: ObservationProgress, Summary: "Wrote the schema", SpanStart: 0, SpanEnd: 22}
	empty := ok
	empty.SpanStart, empty.SpanEnd = 5, 5
	reversed := ok
	reversed.SpanStart, reversed.SpanEnd = 5, 2
	negative := ok
	negative.SpanStart = -1
	badKind := ok
	badKind.Kind = "rant"
	runValidate(t, []validateCase{
		{"valid", ok, nil},
		{"empty span", empty, nil},
		{"reversed span", reversed, []string{"span_end"}},
		{"negative start", negative, []string{"span_start"}},
		{"bad kind", badKind, []string{"kind"}},
		{"zero value", Observation{}, []string{"entry_id", "extraction_run_id", "kind", "summary"}},
	})
}

func TestObservationLinkValidate(t *testing.T) {
	ok := ObservationLink{ObservationID: 1, TargetType: TargetTask, TargetID: 1, Confidence: 0.9}
	over := ok
	over.Confidence = 1.5
	nan := ok
	nan.Confidence = math.NaN()
	edges := ok
	edges.Confidence = 0
	badTarget := ok
	badTarget.TargetType = "entry"
	runValidate(t, []validateCase{
		{"valid", ok, nil},
		{"confidence 0", edges, nil},
		{"confidence over 1", over, []string{"confidence"}},
		{"confidence NaN", nan, []string{"confidence"}},
		{"bad target type", badTarget, []string{"target_type"}},
	})
}

func TestBlockerValidate(t *testing.T) {
	ok := Blocker{ProjectID: 1, TaskID: ptr(int64(1)), Description: "Unsure about enums", Status: BlockerOpen, OriginEntryID: ptr(int64(1))}
	resolved := ok
	resolved.Status, resolved.ResolvedEntryID = BlockerResolved, ptr(int64(2))
	resolvedManually := ok
	resolvedManually.Status = BlockerResolved
	openWithResolver := ok
	openWithResolver.ResolvedEntryID = ptr(int64(2))
	runValidate(t, []validateCase{
		{"valid", ok, nil},
		{"resolved by entry", resolved, nil},
		{"resolved manually", resolvedManually, nil},
		{"open with resolving entry", openWithResolver, []string{"resolved_entry_id"}},
		{"zero value", Blocker{}, []string{"project_id", "description", "status"}},
	})
}

func TestIdeaValidate(t *testing.T) {
	runValidate(t, []validateCase{
		{"valid", Idea{Title: "Voice entry", Status: IdeaParked}, nil},
		{"project idea", Idea{ProjectID: ptr(int64(1)), Title: "x", Body: "y", Status: IdeaPromoted}, nil},
		{"bad status", Idea{Title: "x", Status: "active"}, []string{"status"}},
		{"blank title", Idea{Title: "", Status: IdeaParked}, []string{"title"}},
	})
}

func TestFollowUpQuestionValidate(t *testing.T) {
	ok := FollowUpQuestion{EntryID: 1, ExtractionRunID: 1, Text: "What's the next step?", Status: QuestionOpen}
	answered := ok
	answered.Status, answered.AnswerEntryID = QuestionAnswered, ptr(int64(2))
	answeredNoEntry := ok
	answeredNoEntry.Status = QuestionAnswered
	dismissedWithEntry := ok
	dismissedWithEntry.Status, dismissedWithEntry.AnswerEntryID = QuestionDismissed, ptr(int64(2))
	runValidate(t, []validateCase{
		{"valid", ok, nil},
		{"answered", answered, nil},
		{"answered without entry", answeredNoEntry, []string{"answer_entry_id"}},
		{"dismissed with entry", dismissedWithEntry, []string{"answer_entry_id"}},
		{"zero value", FollowUpQuestion{}, []string{"entry_id", "extraction_run_id", "text", "status"}},
	})
}

func TestExtractionRunValidate(t *testing.T) {
	ok := ExtractionRun{EntryID: 1, Provider: "mock", Status: RunPending}
	done := ok
	done.Status, done.InputSnapshot, done.RawOutput, done.LatencyMS = RunSucceeded, json.RawMessage(`{}`), json.RawMessage(`{"ops":[]}`), ptr(int64(12))
	badJSON := ok
	badJSON.RawOutput = json.RawMessage(`{oops`)
	negLatency := ok
	negLatency.LatencyMS = ptr(int64(-1))
	runValidate(t, []validateCase{
		{"valid", ok, nil},
		{"succeeded", done, nil},
		{"bad raw output", badJSON, []string{"raw_output"}},
		{"negative latency", negLatency, []string{"latency_ms"}},
		{"zero value", ExtractionRun{}, []string{"entry_id", "provider", "status"}},
	})
}

func TestChangesetValidate(t *testing.T) {
	runValidate(t, []validateCase{
		{"extraction", Changeset{Source: SourceExtraction, ExtractionRunID: ptr(int64(1)), Status: ChangesetPending}, nil},
		{"manual", Changeset{Source: SourceManual, Status: ChangesetApplied}, nil},
		{"extraction without run", Changeset{Source: SourceExtraction, Status: ChangesetPending}, []string{"extraction_run_id"}},
		{"manual with run", Changeset{Source: SourceManual, ExtractionRunID: ptr(int64(1)), Status: ChangesetPending}, []string{"extraction_run_id"}},
		{"bad status", Changeset{Source: SourceManual, Status: "done"}, []string{"status"}},
	})
}

func TestChangeOpValidate(t *testing.T) {
	ok := ChangeOp{ChangesetID: 1, OpType: OpCreateTask, Payload: json.RawMessage(`{"project_id":1,"title":"x"}`), Confidence: ptr(0.8), Status: OpPending}
	edited := ok
	edited.Status, edited.AppliedPayload = OpEdited, json.RawMessage(`{"project_id":1,"title":"y"}`)
	noPayload := ok
	noPayload.Payload = nil
	badPayload := ok
	badPayload.Payload = json.RawMessage(`nope`)
	badApplied := ok
	badApplied.AppliedPayload = json.RawMessage(`{`)
	badConfidence := ok
	badConfidence.Confidence = ptr(-0.1)
	badType := ok
	badType.OpType = "delete_task"
	runValidate(t, []validateCase{
		{"valid", ok, nil},
		{"edited", edited, nil},
		{"no payload", noPayload, []string{"payload"}},
		{"bad payload", badPayload, []string{"payload"}},
		{"bad applied payload", badApplied, []string{"applied_payload"}},
		{"bad confidence", badConfidence, []string{"confidence"}},
		{"bad op type", badType, []string{"op_type"}},
	})
}

func TestBriefingValidate(t *testing.T) {
	runValidate(t, []validateCase{
		{"valid", Briefing{ProjectID: 1, NextTaskID: ptr(int64(1)), Source: BriefingHeuristic}, nil},
		{"no next task", Briefing{ProjectID: 1, Source: BriefingLLM}, nil},
		{"zero value", Briefing{}, []string{"project_id", "source"}},
	})
}

// Problems must never echo free text (titles, bodies) back: they may be logged.
func TestProblemsDontEchoText(t *testing.T) {
	secret := "my private diary line"
	for _, v := range []validator{
		Task{Title: secret, Status: "bogus"},
		Observation{EntryID: 1, ExtractionRunID: 1, Kind: ObservationKind(secret), Summary: secret, SpanEnd: -1},
		ChangeOp{ChangesetID: 1, OpType: OpType(secret), Payload: json.RawMessage(secret), Status: ChangeOpStatus(secret)},
	} {
		err := v.Validate()
		if err == nil || strings.Contains(err.Error(), secret) {
			t.Errorf("error %v echoes free text", err)
		}
	}
}
