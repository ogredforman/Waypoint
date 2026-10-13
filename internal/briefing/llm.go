package briefing

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/ogredforman/waypoint/internal/domain"
	"github.com/ogredforman/waypoint/internal/extract"
)

// BriefTool names the tool the LLM Briefer asks the model to call.
const (
	BriefTool        = "write_briefing"
	BriefDescription = "Return the project's next action, why, and a recap of its recent entries."
)

// Caps on the model's text, well above what the prompt asks for, so a
// rambling answer falls back rather than filling the "Now" screen.
const (
	maxReasonLen = 300
	maxPointLen  = 200
)

//go:embed llm_prompt.md
var llmPrompt string

//go:embed llm_schema.json
var llmSchema []byte

// LLM is the model-backed Briefer (SPEC §7.3). It asks the model through an
// extract.Caller, so it never knows the provider. Each part of the model's
// answer is checked, and a part that fails falls back to the Heuristic's:
// the next action unless it names a task that can be started now with a
// reason, the recap unless every entry is a recent one with valid points.
// The briefing's Source is llm when any part came from the model.
type LLM struct {
	caller extract.Caller
	now    func() time.Time
	logger *slog.Logger
}

// NewLLM returns an LLM Briefer that calls c. now gives "today" in the
// prompt.
func NewLLM(c extract.Caller, now func() time.Time, logger *slog.Logger) *LLM {
	return &LLM{caller: c, now: now, logger: logger}
}

// llmBriefing is the model's tool input.
type llmBriefing struct {
	NextTaskID int64  `json:"next_task_id"`
	Reason     string `json:"reason"`
	Recap      []struct {
		EntryID int64               `json:"entry_id"`
		Points  []domain.RecapPoint `json:"points"`
	} `json:"recap"`
}

// Brief asks the model unless there is no task to start, in which case the
// Heuristic's answer stands. It returns an error only if the Heuristic does;
// a failed model call is logged and the Heuristic's briefing returned.
func (l *LLM) Brief(ctx context.Context, st State) (domain.Briefing, error) {
	b, err := Heuristic{}.Brief(ctx, st)
	if err != nil {
		return domain.Briefing{}, err
	}
	ready := startable(st)
	if len(ready) == 0 {
		return b, nil
	}
	log := l.logger.With("project_id", st.Project.ID)
	reply, err := l.caller.Call(ctx, extract.ToolCall{
		System:      llmPrompt,
		User:        l.userMessage(st, ready),
		Tool:        BriefTool,
		Description: BriefDescription,
		Schema:      json.RawMessage(slices.Clone(llmSchema)),
	})
	if err != nil {
		log.Warn("llm briefing failed; using the heuristic", "err", err)
		return b, nil
	}
	log.Debug("llm briefing reply", "response", string(reply.Response))
	var got llmBriefing
	if err := json.Unmarshal(reply.Input, &got); err != nil {
		log.Warn("llm briefing unreadable; using the heuristic", "err", err)
		return b, nil
	}

	if problem := checkNext(got, ready); problem != "" {
		log.Warn("llm next action rejected; using the heuristic's", "problem", problem)
	} else {
		id := got.NextTaskID
		b.NextTaskID, b.NextReason = &id, strings.TrimSpace(got.Reason)
		b.Source = domain.BriefingLLM
	}
	if recap, problem := checkRecap(got, st.RecentEntries); problem != "" {
		log.Warn("llm recap rejected; using the heuristic's", "problem", problem)
	} else {
		b.Recap = recap
		b.Source = domain.BriefingLLM
	}
	return b, nil
}

// checkNext says what is wrong with the model's next action, or "" if
// nothing is.
func checkNext(got llmBriefing, ready []domain.Task) string {
	if !slices.ContainsFunc(ready, func(t domain.Task) bool { return t.ID == got.NextTaskID }) {
		return fmt.Sprintf("next_task_id %d is not a task that can be started now", got.NextTaskID)
	}
	reason := strings.TrimSpace(got.Reason)
	if reason == "" || len(reason) > maxReasonLen {
		return fmt.Sprintf("reason is empty or over %d bytes", maxReasonLen)
	}
	return ""
}

// checkRecap returns the model's recap in the order of recent (newest
// first), or what is wrong with it. An entry must be one of recent, once,
// with 1 to recapPoints points of a valid kind and short, non-empty text.
// An empty recap is wrong when the heuristic would have something to say.
func checkRecap(got llmBriefing, recent []RecentEntry) ([]domain.RecapEntry, string) {
	if len(got.Recap) > recapEntries {
		return nil, fmt.Sprintf("recap has %d entries, over %d", len(got.Recap), recapEntries)
	}
	byID := map[int64]domain.RecapEntry{}
	for _, e := range got.Recap {
		if !slices.ContainsFunc(recent, func(r RecentEntry) bool { return r.ID == e.EntryID }) {
			return nil, fmt.Sprintf("recap names entry %d, not a recent one", e.EntryID)
		}
		if _, dup := byID[e.EntryID]; dup {
			return nil, fmt.Sprintf("recap names entry %d twice", e.EntryID)
		}
		if len(e.Points) == 0 || len(e.Points) > recapPoints {
			return nil, fmt.Sprintf("entry %d has %d points, want 1 to %d", e.EntryID, len(e.Points), recapPoints)
		}
		re := domain.RecapEntry{EntryID: e.EntryID}
		for _, p := range e.Points {
			text := strings.TrimSpace(p.Text)
			if !p.Kind.Valid() || text == "" || len(text) > maxPointLen {
				return nil, fmt.Sprintf("entry %d has a point with an unknown kind, or empty or over-long text", e.EntryID)
			}
			re.Points = append(re.Points, domain.RecapPoint{Kind: p.Kind, Text: text})
		}
		byID[e.EntryID] = re
	}
	if len(byID) == 0 && len(recap(recent)) > 0 {
		return nil, "recap is empty but recent entries have observations"
	}
	out := []domain.RecapEntry{}
	for _, r := range recent {
		if e, ok := byID[r.ID]; ok {
			e.CreatedAt = r.CreatedAt
			out = append(out, e)
		}
	}
	return out, ""
}

// userMessage writes the project for the model: rows as "task 12" so it
// returns the IDs it was shown, the tasks that can be started now marked.
// The output is deterministic for a given state and day.
func (l *LLM) userMessage(st State, ready []domain.Task) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Today: %s\n\n", day(l.now()))
	fmt.Fprintf(&b, "Project %d: %q\n", st.Project.ID, st.Project.Title)
	if why := strings.TrimSpace(st.Project.Why); why != "" {
		fmt.Fprintf(&b, "Why: %q\n", why)
	}
	if done := strings.TrimSpace(st.Project.DefinitionOfDone); done != "" {
		fmt.Fprintf(&b, "Done when: %q\n", done)
	}

	open := []domain.Milestone{}
	for _, m := range st.Milestones {
		if m.Status == domain.MilestoneOpen {
			open = append(open, m)
		}
	}
	if len(open) > 0 {
		b.WriteString("\nOpen milestones, in order:\n")
		for _, m := range open {
			fmt.Fprintf(&b, "- milestone %d: %q\n", m.ID, m.Title)
		}
	}

	canStart := map[int64]bool{}
	for _, t := range ready {
		canStart[t.ID] = true
	}
	waitsOn := map[int64][]string{}
	for _, d := range st.Dependencies {
		if i := slices.IndexFunc(st.Tasks, func(t domain.Task) bool { return t.ID == d.BlockedByTaskID }); i >= 0 && isOpen(st.Tasks[i]) {
			waitsOn[d.TaskID] = append(waitsOn[d.TaskID], fmt.Sprintf("task %d", d.BlockedByTaskID))
		}
	}
	for _, bl := range st.Blockers {
		if bl.TaskID != nil {
			waitsOn[*bl.TaskID] = append(waitsOn[*bl.TaskID], fmt.Sprintf("blocker %d", bl.ID))
		}
	}
	for _, t := range st.Tasks {
		if t.ParentTaskID != nil && isOpen(t) {
			waitsOn[*t.ParentTaskID] = append(waitsOn[*t.ParentTaskID], fmt.Sprintf("its step task %d", t.ID))
		}
	}
	inMilestone := milestones(st)
	b.WriteString("\nOpen tasks, in list order:\n")
	for _, t := range st.Tasks {
		if !isOpen(t) {
			continue
		}
		attrs := []string{string(t.Status)}
		if t.Size != nil {
			attrs = append(attrs, "size "+string(*t.Size))
		}
		if t.Energy != nil {
			attrs = append(attrs, string(*t.Energy)+" energy")
		}
		if m := inMilestone[t.ID]; m != nil {
			attrs = append(attrs, fmt.Sprintf("milestone %d", *m))
		}
		if t.ParentTaskID != nil {
			attrs = append(attrs, fmt.Sprintf("a step of task %d", *t.ParentTaskID))
		}
		attrs = append(attrs, "updated "+day(t.UpdatedAt))
		if canStart[t.ID] {
			attrs = append(attrs, "can start now")
		} else {
			attrs = append(attrs, "waiting on "+strings.Join(waitsOn[t.ID], ", "))
		}
		fmt.Fprintf(&b, "- task %d: %q (%s)\n", t.ID, t.Title, strings.Join(attrs, "; "))
		if n := strings.TrimSpace(t.Notes); n != "" {
			fmt.Fprintf(&b, "  notes: %q\n", n)
		}
	}

	if len(st.Blockers) > 0 {
		b.WriteString("\nOpen blockers:\n")
		for _, bl := range st.Blockers {
			fmt.Fprintf(&b, "- blocker %d: %q", bl.ID, bl.Description)
			if bl.TaskID != nil {
				fmt.Fprintf(&b, " (on task %d)", *bl.TaskID)
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("\nRecent entries, newest first:\n")
	if len(st.RecentEntries) == 0 {
		b.WriteString("(none)\n")
	}
	for _, e := range st.RecentEntries {
		fmt.Fprintf(&b, "- entry %d (%s):\n", e.ID, day(e.CreatedAt))
		if len(e.Observations) == 0 {
			b.WriteString("  (no observations)\n")
		}
		for _, o := range e.Observations {
			fmt.Fprintf(&b, "  - %s: %q\n", o.Kind, o.Text)
		}
	}
	return b.String()
}

func day(t time.Time) string { return t.UTC().Format("2006-01-02") }
