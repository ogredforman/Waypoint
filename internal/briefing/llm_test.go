package briefing

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ogredforman/waypoint/internal/domain"
	"github.com/ogredforman/waypoint/internal/extract"
)

// fakeCaller answers every call with input, or err, and keeps the last call.
type fakeCaller struct {
	input string
	err   error
	got   *extract.ToolCall
}

func (c *fakeCaller) Call(_ context.Context, tc extract.ToolCall) (extract.ToolReply, error) {
	c.got = &tc
	if c.err != nil {
		return extract.ToolReply{}, c.err
	}
	return extract.ToolReply{Input: json.RawMessage(c.input), Response: json.RawMessage(`{}`)}, nil
}

func newLLM(c extract.Caller) *LLM {
	return NewLLM(c, func() time.Time { return t0 }, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// llmState has tasks 1 (todo), 2 (todo, small), 3 (waiting on blocker 7),
// and 4 (done), and recent entries 20 and 10.
func llmState() State {
	return State{
		Project: domain.Project{ID: 1, Title: "Waypoint"},
		Tasks: []domain.Task{
			task(1), task(2, size(domain.SizeS), energy(domain.EnergyLow)), task(3), task(4, status(domain.TaskDone)),
		},
		Blockers: []domain.Blocker{{ID: 7, TaskID: ptr(int64(3)), Description: "Waiting on the API key", Status: domain.BlockerOpen}},
		RecentEntries: []RecentEntry{
			{ID: 20, CreatedAt: t0, Observations: []domain.RecapPoint{{Kind: domain.ObservationFeeling, Text: "Tired today"}}},
			{ID: 10, CreatedAt: t0.Add(-24 * time.Hour), Observations: []domain.RecapPoint{{Kind: domain.ObservationProgress, Text: "Wired the hub"}}},
		},
	}
}

const goodRecap = `[{"entry_id": 10, "points": [{"kind": "progress", "text": "Wired the SSE hub"}]},
	{"entry_id": 20, "points": [{"kind": "feeling", "text": " Felt tired "}]}]`

func TestLLMBrief(t *testing.T) {
	heur, _ := Heuristic{}.Brief(context.Background(), llmState())
	modelRecap := []domain.RecapEntry{
		{EntryID: 20, CreatedAt: t0, Points: []domain.RecapPoint{{Kind: domain.ObservationFeeling, Text: "Felt tired"}}},
		{EntryID: 10, CreatedAt: t0.Add(-24 * time.Hour), Points: []domain.RecapPoint{{Kind: domain.ObservationProgress, Text: "Wired the SSE hub"}}},
	}
	tests := []struct {
		name       string
		input      string
		err        error
		wantNext   int64
		wantReason string
		wantRecap  []domain.RecapEntry
		wantSource domain.BriefingSource
	}{
		{name: "all from the model", input: `{"next_task_id": 1, "reason": "It picks up where you left off.", "recap": ` + goodRecap + `}`,
			wantNext: 1, wantReason: "It picks up where you left off.", wantRecap: modelRecap, wantSource: domain.BriefingLLM},
		{name: "blocked task", input: `{"next_task_id": 3, "reason": "Go.", "recap": ` + goodRecap + `}`,
			wantNext: 2, wantReason: heur.NextReason, wantRecap: modelRecap, wantSource: domain.BriefingLLM},
		{name: "done task", input: `{"next_task_id": 4, "reason": "Go.", "recap": []}`,
			wantNext: 2, wantReason: heur.NextReason, wantRecap: heur.Recap, wantSource: domain.BriefingHeuristic},
		{name: "unknown task", input: `{"next_task_id": 99, "reason": "Go.", "recap": ` + goodRecap + `}`,
			wantNext: 2, wantReason: heur.NextReason, wantRecap: modelRecap, wantSource: domain.BriefingLLM},
		{name: "empty reason", input: `{"next_task_id": 1, "reason": "  ", "recap": ` + goodRecap + `}`,
			wantNext: 2, wantReason: heur.NextReason, wantRecap: modelRecap, wantSource: domain.BriefingLLM},
		{name: "unknown entry", input: `{"next_task_id": 1, "reason": "Small.", "recap": [{"entry_id": 5, "points": [{"kind": "note", "text": "x"}]}]}`,
			wantNext: 1, wantReason: "Small.", wantRecap: heur.Recap, wantSource: domain.BriefingLLM},
		{name: "entry twice", input: `{"next_task_id": 1, "reason": "Small.", "recap": [{"entry_id": 10, "points": [{"kind": "note", "text": "x"}]}, {"entry_id": 10, "points": [{"kind": "note", "text": "y"}]}]}`,
			wantNext: 1, wantReason: "Small.", wantRecap: heur.Recap, wantSource: domain.BriefingLLM},
		{name: "bad kind", input: `{"next_task_id": 1, "reason": "Small.", "recap": [{"entry_id": 10, "points": [{"kind": "mood", "text": "x"}]}]}`,
			wantNext: 1, wantReason: "Small.", wantRecap: heur.Recap, wantSource: domain.BriefingLLM},
		{name: "no points", input: `{"next_task_id": 1, "reason": "Small.", "recap": [{"entry_id": 10, "points": []}]}`,
			wantNext: 1, wantReason: "Small.", wantRecap: heur.Recap, wantSource: domain.BriefingLLM},
		{name: "empty recap with things to say", input: `{"next_task_id": 1, "reason": "Small.", "recap": []}`,
			wantNext: 1, wantReason: "Small.", wantRecap: heur.Recap, wantSource: domain.BriefingLLM},
		{name: "not an object", input: `"oops"`,
			wantNext: 2, wantReason: heur.NextReason, wantRecap: heur.Recap, wantSource: domain.BriefingHeuristic},
		{name: "call fails", err: errors.New("timeout"),
			wantNext: 2, wantReason: heur.NextReason, wantRecap: heur.Recap, wantSource: domain.BriefingHeuristic},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := newLLM(&fakeCaller{input: tt.input, err: tt.err}).Brief(context.Background(), llmState())
			if err != nil {
				t.Fatal(err)
			}
			if nextOf(b) != tt.wantNext || b.NextReason != tt.wantReason || b.Source != tt.wantSource {
				t.Errorf("next = %d %q (%s), want %d %q (%s)", nextOf(b), b.NextReason, b.Source, tt.wantNext, tt.wantReason, tt.wantSource)
			}
			if !slices.EqualFunc(b.Recap, tt.wantRecap, recapEqual) {
				t.Errorf("recap = %+v, want %+v", b.Recap, tt.wantRecap)
			}
		})
	}
}

func recapEqual(a, b domain.RecapEntry) bool {
	return a.EntryID == b.EntryID && a.CreatedAt.Equal(b.CreatedAt) && slices.Equal(a.Points, b.Points)
}

func TestLLMSkipsModelWithNothingToStart(t *testing.T) {
	st := llmState()
	st.Tasks = []domain.Task{task(3), task(4, status(domain.TaskDone))}
	c := &fakeCaller{err: errors.New("should not be called")}
	b, err := newLLM(c).Brief(context.Background(), st)
	if err != nil {
		t.Fatal(err)
	}
	if c.got != nil || b.NextTaskID != nil || b.NextReason != reasonAllWaiting || b.Source != domain.BriefingHeuristic {
		t.Errorf("briefing = %+v (called: %v), want the heuristic's without a model call", b, c.got != nil)
	}
}

func TestLLMCall(t *testing.T) {
	c := &fakeCaller{err: errors.New("stop")}
	if _, err := newLLM(c).Brief(context.Background(), llmState()); err != nil {
		t.Fatal(err)
	}
	if c.got.Tool != BriefTool || c.got.System != llmPrompt || !json.Valid(c.got.Schema) {
		t.Fatalf("call = %+v, want the briefing tool, prompt, and schema", c.got)
	}
	for _, want := range []string{
		"Today: 2026-09-29",
		`Project 1: "Waypoint"`,
		`- task 1: "t" (todo; updated 2026-09-29; can start now)`,
		`- task 2: "t" (todo; size s; low energy; updated 2026-09-29; can start now)`,
		`- task 3: "t" (todo; updated 2026-09-29; waiting on blocker 7)`,
		`- blocker 7: "Waiting on the API key" (on task 3)`,
		"- entry 20 (2026-09-29):\n  - feeling: \"Tired today\"",
		"- entry 10 (2026-09-28):",
	} {
		if !strings.Contains(c.got.User, want) {
			t.Errorf("user message lacks %q:\n%s", want, c.got.User)
		}
	}
	if strings.Contains(c.got.User, "task 4") {
		t.Errorf("user message lists the done task:\n%s", c.got.User)
	}
}

// The schema's observation kinds are the domain's.
func TestLLMSchemaKinds(t *testing.T) {
	var s struct {
		Properties struct {
			Recap struct {
				Items struct {
					Properties struct {
						Points struct {
							Items struct {
								Properties struct {
									Kind struct {
										Enum []domain.ObservationKind `json:"enum"`
									} `json:"kind"`
								} `json:"properties"`
							} `json:"items"`
						} `json:"points"`
					} `json:"properties"`
				} `json:"items"`
			} `json:"recap"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(llmSchema, &s); err != nil {
		t.Fatal(err)
	}
	got := s.Properties.Recap.Items.Properties.Points.Items.Properties.Kind.Enum
	want := domain.ObservationKinds()
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("schema kinds = %v, want %v", got, want)
	}
}

func TestLLMWithoutModelIsQuiet(t *testing.T) {
	var logs bytes.Buffer
	l := NewLLM(&fakeCaller{err: extract.ErrNoModel}, func() time.Time { return t0 }, slog.New(slog.NewTextHandler(&logs, nil)))
	b, err := l.Brief(context.Background(), llmState())
	if err != nil {
		t.Fatal(err)
	}
	if b.Source != domain.BriefingHeuristic {
		t.Errorf("source = %s, want the heuristic's", b.Source)
	}
	if logs.Len() != 0 {
		t.Errorf("logged %q; the mock provider isn't a failure", logs.String())
	}
}
