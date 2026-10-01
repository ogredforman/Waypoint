package mock

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ogredforman/waypoint/internal/domain"
	"github.com/ogredforman/waypoint/internal/extract"
)

var now = time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)

func ptr[T any](v T) *T { return &v }

// snapshot has two projects. Project 1 has an in-progress task (10), a todo
// task (11), and an open blocker (20); project 2 has a todo task (12).
func snapshot() extract.StateSnapshot {
	return extract.StateSnapshot{
		TakenAt: now,
		Projects: []extract.SnapshotProject{
			{
				ID: 1, Title: "Waypoint planner", Status: domain.ProjectActive,
				Tasks: []extract.SnapshotTask{
					{ID: 10, Title: "Write the mock extractor", Status: domain.TaskInProgress, UpdatedAt: now.Add(-time.Hour)},
					{ID: 11, Title: "Set up CI", Status: domain.TaskTodo, UpdatedAt: now},
				},
				Blockers: []extract.SnapshotBlocker{{ID: 20, Description: "waiting on the SQLite driver release"}},
			},
			{
				ID: 2, Title: "Garden shed", Status: domain.ProjectActive,
				Tasks: []extract.SnapshotTask{
					{ID: 12, Title: "Buy lumber", Status: domain.TaskTodo, UpdatedAt: now},
				},
			},
		},
	}
}

// run extracts body, scoped to project if non-zero, and checks the result
// against the shared validator.
func run(t *testing.T, snap extract.StateSnapshot, project int64, body string) extract.ExtractionResult {
	t.Helper()
	in := extract.ExtractInput{Entry: domain.Entry{ID: 1, Body: body}, Snapshot: snap}
	if project != 0 {
		in.Entry.ProjectID = &project
	}
	res, err := New(0).Extract(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if err := extract.Validate(in, res); err != nil {
		t.Fatalf("result fails validation: %v", err)
	}
	return res
}

// ops renders the ops as "op_type payload" for comparison.
func ops(res extract.ExtractionResult) []string {
	out := []string{}
	for _, o := range res.Ops {
		out = append(out, string(o.OpType)+" "+string(o.Payload))
	}
	return out
}

// observations renders the observations as "kind [span text] links".
func observations(body string, res extract.ExtractionResult) []string {
	out := []string{}
	for _, o := range res.Observations {
		s := fmt.Sprintf("%s [%s]", o.Kind, body[o.SpanStart:o.SpanEnd])
		for _, l := range o.Links {
			s += fmt.Sprintf(" %s:%d", l.TargetType, l.TargetID)
		}
		out = append(out, s)
	}
	return out
}

func equal(t *testing.T, what string, got, want []string) {
	t.Helper()
	if want == nil {
		want = []string{}
	}
	if !slices.Equal(got, want) {
		t.Errorf("%s:\n got  %q\n want %q", what, got, want)
	}
}

func TestPrefixes(t *testing.T) {
	for _, tt := range []struct {
		name    string
		body    string
		wantObs []string
		wantOps []string
	}{
		{
			name:    "done matches a task",
			body:    "done: wrote the mock extractor",
			wantObs: []string{"progress [done: wrote the mock extractor] project:1 task:10"},
			wantOps: []string{`set_task_status {"task_id":10,"status":"done"}`},
		},
		{
			name:    "done with no match creates the task",
			body:    "done: painted the fence",
			wantObs: []string{"progress [done: painted the fence] project:1"},
			wantOps: []string{
				`create_task {"project_id":1,"title":"painted the fence"}`,
				`set_task_status {"task_id":"$new:1","status":"done"}`,
			},
		},
		{
			name:    "doing matches a task in another project",
			body:    "doing: buy lumber",
			wantObs: []string{"progress [doing: buy lumber] project:2 task:12"},
			wantOps: []string{`set_task_status {"task_id":12,"status":"in_progress"}`},
		},
		{
			name:    "doing a task already in progress proposes nothing",
			body:    "doing: mock extractor",
			wantObs: []string{"progress [doing: mock extractor] project:1 task:10"},
		},
		{
			name:    "todo",
			body:    "todo: add the SSE hub",
			wantOps: []string{`create_task {"project_id":1,"title":"add the SSE hub"}`},
		},
		{
			name:    "todo then doing the same task",
			body:    "todo: add the SSE hub\ndoing: SSE hub",
			wantObs: []string{"progress [doing: SSE hub] project:1"},
			wantOps: []string{
				`create_task {"project_id":1,"title":"add the SSE hub"}`,
				`set_task_status {"task_id":"$new:1","status":"in_progress"}`,
			},
		},
		{
			name:    "blocked",
			body:    "blocked: no test machine",
			wantObs: []string{"blocker [blocked: no test machine] project:1"},
			wantOps: []string{`create_blocker {"project_id":1,"description":"no test machine"}`},
		},
		{
			name:    "unblocked matches a blocker",
			body:    "unblocked: SQLite driver release is out",
			wantOps: []string{`resolve_blocker {"blocker_id":20}`},
		},
		{
			name:    "unblocked twice resolves once",
			body:    "unblocked: SQLite driver release\nunblocked: SQLite driver release",
			wantObs: []string{"note [unblocked: SQLite driver release] project:1"},
			wantOps: []string{`resolve_blocker {"blocker_id":20}`},
		},
		{
			name:    "unblocked with no match is a note",
			body:    "unblocked: the weather",
			wantObs: []string{"note [unblocked: the weather] project:1"},
		},
		{
			name:    "idea",
			body:    "idea: dark mode",
			wantObs: []string{"idea [idea: dark mode] project:1"},
			wantOps: []string{`park_idea {"project_id":1,"title":"dark mode"}`},
		},
		{
			name:    "feel and decided",
			body:    "feel: tired but good\ndecided: ship Friday",
			wantObs: []string{"feeling [feel: tired but good] project:1", "decision [decided: ship Friday] project:1"},
		},
		{
			name:    "other lines are notes",
			body:    "Long day.\nnote: not a prefix\ntodo:",
			wantObs: []string{"note [Long day.] project:1", "note [note: not a prefix] project:1", "note [todo:] project:1"},
		},
		{
			name: "bullets, case, indentation, CRLF, and blank lines",
			body: "  - TODO: first\r\n\r\n* Idea:  second  \r\n",
			wantObs: []string{
				"idea [Idea:  second] project:1",
			},
			wantOps: []string{
				`create_task {"project_id":1,"title":"first"}`,
				`park_idea {"project_id":1,"title":"second"}`,
			},
		},
		{
			name:    "multi-byte text",
			body:    "☕ café\nfeel: très bien ☕",
			wantObs: []string{"note [☕ café] project:1", "feeling [feel: très bien ☕] project:1"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			res := run(t, snapshot(), 1, tt.body)
			equal(t, "observations", observations(tt.body, res), tt.wantObs)
			equal(t, "ops", ops(res), tt.wantOps)
		})
	}
}

func TestConfidenceAndRationale(t *testing.T) {
	res := run(t, snapshot(), 1, "done: mock extractor\ntodo: more tests")
	if got := *res.Ops[0].Confidence; got >= 1 || got < matchThreshold {
		t.Errorf("fuzzy match confidence = %v, want in [%v, 1)", got, matchThreshold)
	}
	if got := *res.Ops[1].Confidence; got != 1 {
		t.Errorf("create confidence on a scoped entry = %v, want 1", got)
	}
	for _, o := range res.Ops {
		if strings.Contains(o.Rationale, "guessed") {
			t.Errorf("scoped entry op says the project was guessed: %q", o.Rationale)
		}
	}
}

func TestProjectGuess(t *testing.T) {
	for _, tt := range []struct {
		name      string
		snap      func() extract.StateSnapshot
		body      string
		wantOps   []string
		wantConf  float64
		wantGuess bool
	}{
		{
			name: "only project",
			snap: func() extract.StateSnapshot {
				s := snapshot()
				s.Projects = s.Projects[1:]
				return s
			},
			body:      "todo: paint it",
			wantOps:   []string{`create_task {"project_id":2,"title":"paint it"}`},
			wantConf:  onlyProjectGuess,
			wantGuess: true,
		},
		{
			name:      "title words",
			body:      "The garden needs work.\ntodo: order a shed kit",
			wantOps:   []string{`create_task {"project_id":2,"title":"order a shed kit"}`},
			wantConf:  cluedProjectGuess,
			wantGuess: true,
		},
		{
			name: "a matched task outweighs a title word",
			body: "Garden later.\ndoing: mock extractor\ntodo: tests",
			wantOps: []string{
				`create_task {"project_id":1,"title":"tests"}`,
			},
			wantConf:  cluedProjectGuess,
			wantGuess: true,
		},
		{
			name:      "a tie goes to the first project",
			body:      "garden and planner\ntodo: plan",
			wantOps:   []string{`create_task {"project_id":1,"title":"plan"}`},
			wantConf:  cluedProjectGuess,
			wantGuess: true,
		},
		{
			name:      "no clues still guesses, with low confidence",
			body:      "todo: call mom",
			wantOps:   []string{`create_task {"project_id":1,"title":"call mom"}`},
			wantConf:  blindProjectGuess,
			wantGuess: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			snap := snapshot()
			if tt.snap != nil {
				snap = tt.snap()
			}
			res := run(t, snap, 0, tt.body)
			equal(t, "ops", ops(res), tt.wantOps)
			op := res.Ops[len(res.Ops)-1]
			if *op.Confidence != tt.wantConf {
				t.Errorf("confidence = %v, want %v", *op.Confidence, tt.wantConf)
			}
			if got := strings.Contains(op.Rationale, "project guessed"); got != tt.wantGuess {
				t.Errorf("rationale %q: says guessed = %v, want %v", op.Rationale, got, tt.wantGuess)
			}
		})
	}
}

// TestBlindGuessLinks checks that a project guessed without clues isn't
// linked from observations: the review UI can move ops, not observations.
func TestBlindGuessLinks(t *testing.T) {
	body := "Long day.\ndone: mock extractor\nsomething else"
	snap := snapshot()
	snap.Projects[0].Title, snap.Projects[1].Title = "Alpha", "Beta"
	snap.Projects[0].Tasks = nil // no task clue either
	res := run(t, snap, 0, body)
	equal(t, "observations", observations(body, res), []string{
		"note [Long day.]",
		"progress [done: mock extractor]",
		"note [something else]",
	})
}

// TestTitleCluesIgnorePrefixes checks that a prefix word doesn't count as a
// project title word.
func TestTitleCluesIgnorePrefixes(t *testing.T) {
	snap := snapshot()
	snap.Projects[1].Title = "Todo app"
	res := run(t, snap, 0, "todo: planner docs")
	equal(t, "ops", ops(res), []string{`create_task {"project_id":1,"title":"planner docs"}`})
}

func TestEntryProjectBeatsClues(t *testing.T) {
	res := run(t, snapshot(), 1, "garden shed\ntodo: lumber rack")
	equal(t, "ops", ops(res), []string{`create_task {"project_id":1,"title":"lumber rack"}`})
}

func TestNoProjects(t *testing.T) {
	body := "todo: something\nblocked: stuck\nidea: new thing\ndone: a thing"
	res := run(t, extract.StateSnapshot{TakenAt: now}, 0, body)
	equal(t, "observations", observations(body, res), []string{
		"note [todo: something]",
		"blocker [blocked: stuck]",
		"idea [idea: new thing]",
		"note [done: a thing]",
	})
	equal(t, "ops", ops(res), []string{`park_idea {"title":"new thing"}`})
}

func TestQuestions(t *testing.T) {
	stalled := func() extract.StateSnapshot {
		s := snapshot()
		s.Projects[0].Tasks[0].UpdatedAt = now.Add(-4 * 24 * time.Hour)
		s.Projects[0].Blockers[0].TaskID = ptr(int64(11))
		return s
	}
	const next = "What's the next step?"
	const blockers = `Is anything in the way of "Write the mock extractor"?`
	for _, tt := range []struct {
		name string
		snap func() extract.StateSnapshot
		body string
		want []string
	}{
		{"no next step", snapshot, "feel: fine", []string{next}},
		{"todo is a next step", snapshot, "todo: x", nil},
		{"doing is a next step", snapshot, "doing: set up CI", nil},
		{"stalled task", stalled, "todo: x", []string{blockers}},
		{"both, capped", stalled, "feel: fine", []string{next, blockers}},
		{"stalled task the entry names", stalled, "done: mock extractor", []string{next}},
		{"stalled task with a blocker", func() extract.StateSnapshot {
			s := stalled()
			s.Projects[0].Blockers[0].TaskID = ptr(int64(10))
			return s
		}, "todo: x", nil},
		{"stalled task in a blocked project", func() extract.StateSnapshot {
			s := stalled()
			s.Projects[0].Blockers[0].TaskID = nil
			return s
		}, "todo: x", nil},
		{"entry names a blocker", stalled, "todo: x\nblocked: no time", nil},
		{"just under 3 days", func() extract.StateSnapshot {
			s := snapshot()
			s.Projects[0].Tasks[0].UpdatedAt = now.Add(-stalledAfter)
			return s
		}, "todo: x", nil},
		{"stalled task in another project", func() extract.StateSnapshot {
			s := stalled()
			s.Projects[0], s.Projects[1] = s.Projects[1], s.Projects[0]
			return s
		}, "todo: garden shed", nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			res := run(t, tt.snap(), 0, tt.body)
			var got []string
			for _, q := range res.Questions {
				got = append(got, q.Text)
			}
			equal(t, "questions", got, tt.want)
		})
	}
}

func TestReplyDoesNotRepeatQuestion(t *testing.T) {
	stalled := snapshot()
	stalled.Projects[0].Tasks[0].UpdatedAt = now.Add(-4 * 24 * time.Hour)
	stalled.Projects[0].Blockers[0].TaskID = ptr(int64(11))
	const next = "What's the next step?"
	const blockers = `Is anything in the way of "Write the mock extractor"?`
	for _, tt := range []struct {
		name    string
		replyTo string
		want    []string
	}{
		{"not a reply", "", []string{next, blockers}},
		{"reply to next step", " what's the NEXT step? ", []string{blockers}},
		{"reply to stalled task", blockers, []string{next}},
		{"reply to another question", "How did it go?", []string{next, blockers}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			in := extract.ExtractInput{Entry: domain.Entry{ID: 2, Body: "feel: fine"}, Snapshot: stalled}
			if tt.replyTo != "" {
				in.Entry.ReplyToQuestionID = ptr(int64(1))
				in.ReplyTo = &domain.FollowUpQuestion{ID: 1, EntryID: 1, Text: tt.replyTo}
			}
			res, err := New(0).Extract(context.Background(), in)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, q := range res.Questions {
				got = append(got, q.Text)
			}
			equal(t, "questions", got, tt.want)
		})
	}
}

func TestDeterministic(t *testing.T) {
	body := "done: mock\ntodo: a\nblocked: b\nidea: c\nunblocked: driver\nfeel: ok"
	first := run(t, snapshot(), 0, body)
	for range 5 {
		if got := run(t, snapshot(), 0, body); !reflect.DeepEqual(got, first) {
			t.Fatalf("results differ:\n%+v\n%+v", got, first)
		}
	}
}

func TestLatency(t *testing.T) {
	in := extract.ExtractInput{Entry: domain.Entry{ID: 1, Body: "hi"}, Snapshot: snapshot()}

	start := time.Now()
	if _, err := New(20*time.Millisecond).Extract(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d < 20*time.Millisecond {
		t.Errorf("returned after %v, want at least the latency", d)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := New(time.Hour).Extract(ctx, in); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want context.DeadlineExceeded", err)
	}
}

func TestName(t *testing.T) {
	var e extract.Extractor = New(0)
	if e.Name() != "mock" {
		t.Errorf("Name() = %q", e.Name())
	}
}
