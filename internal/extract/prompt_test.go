package extract

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ogredforman/waypoint/internal/domain"
)

func TestSystemPromptQuestionLimit(t *testing.T) {
	cases := []struct {
		limit int
		want  string
	}{
		{-1, "turned follow-up questions off"},
		{0, "turned follow-up questions off"},
		{1, "Ask at most 1 follow-up question,"},
		{2, "Ask at most 2 follow-up questions,"},
		{5, "Ask at most 2 follow-up questions,"}, // never above the hard cap
	}
	for _, c := range cases {
		got := SystemPrompt(c.limit)
		if !strings.Contains(got, c.want) {
			t.Errorf("SystemPrompt(%d) lacks %q", c.limit, c.want)
		}
		if strings.Contains(got, "{{") || strings.Contains(got, "<no value>") {
			t.Errorf("SystemPrompt(%d) has an unfilled template action", c.limit)
		}
	}
}

// The prompt names every observation kind and every op extraction may
// propose, and none it may not: Validate rejects those.
func TestSystemPromptCoversKindsAndOps(t *testing.T) {
	p := SystemPrompt(2)
	for _, k := range domain.ObservationKinds() {
		if !strings.Contains(p, "- "+string(k)+":") {
			t.Errorf("observation kind %q isn't described", k)
		}
	}
	notProposed := []domain.OpType{domain.OpDeleteProject}
	for _, op := range domain.OpTypes() {
		described := strings.Contains(p, "- "+string(op)+" {")
		if slices.Contains(notProposed, op) {
			if strings.Contains(p, string(op)) {
				t.Errorf("op %q is mentioned, but extraction can't propose it", op)
			}
		} else if !described {
			t.Errorf("op %q isn't described", op)
		}
	}
}

func TestUserMessage(t *testing.T) {
	at := func(d int) time.Time { return time.Date(2026, 10, d, 9, 30, 0, 0, time.UTC) }
	project := int64(1)
	in := ExtractInput{
		Entry: domain.Entry{ID: 9, Body: "Login works now.\nNext: \"remember me\".", ProjectID: &project, CreatedAt: at(12)},
		Snapshot: StateSnapshot{
			TakenAt: at(12),
			Projects: []SnapshotProject{
				{
					ID: 1, Title: "Build Waypoint", Status: domain.ProjectActive,
					Milestones: []SnapshotMilestone{{ID: 3, Title: "MVP"}, {ID: 6, Title: "Beta"}},
					Tasks: []SnapshotTask{
						{ID: 10, Title: "Write the login form", Status: domain.TaskInProgress, MilestoneID: ptr(int64(3)), UpdatedAt: at(2)},
						{ID: 12, Title: "Add validation", Status: domain.TaskTodo, ParentTaskID: ptr(int64(10)), UpdatedAt: at(3)},
						{ID: 13, Title: "Orphan step", Status: domain.TaskTodo, ParentTaskID: ptr(int64(99)), UpdatedAt: at(4)},
					},
					Blockers: []SnapshotBlocker{
						{ID: 4, TaskID: ptr(int64(10)), Description: "Waiting on \"design\""},
						{ID: 5, Description: "No test server"},
					},
				},
				{ID: 2, Title: "Garden", Status: domain.ProjectActive},
			},
			Ideas: []SnapshotIdea{
				{ID: 7, ProjectID: &project, Title: "Dark mode"},
				{ID: 8, Title: "Learn Rust"},
			},
			RecentEntries: []SnapshotEntry{
				{ID: 8, CreatedAt: at(11), Summaries: []string{"Started the login form.", "Feels slow."}},
			},
		},
		ReplyTo: &domain.FollowUpQuestion{Text: "What's next after login?"},
	}
	want := `Today: 2026-10-12
Entry scoped to: project 1 "Build Waypoint"

# Projects

project 1 "Build Waypoint" (active)
Open milestones, in order:
- milestone 3: "MVP"
- milestone 6: "Beta"
Open tasks:
- task 10 [in_progress] "Write the login form" (milestone 3, updated 2026-10-02)
  - task 12 [todo] "Add validation" (updated 2026-10-03)
- task 13 [todo] "Orphan step" (updated 2026-10-04)
Open blockers:
- blocker 4 (on task 10): "Waiting on \"design\""
- blocker 5: "No test server"

project 2 "Garden" (active)
Open tasks:
(none)

# Parked ideas
- idea 7 (project 1): "Dark mode"
- idea 8: "Learn Rust"

# Recent entries, newest first (summaries only)
- 2026-10-11: Started the login form. Feels slow.

# This entry answers your earlier question
"What's next after login?"

# Entry, written 2026-10-12
<entry>
Login works now.
Next: "remember me".
</entry>
`
	if got := UserMessage(in); got != want {
		t.Errorf("UserMessage:\n%s\nwant:\n%s", got, want)
	}
}

func TestUserMessageEmptyState(t *testing.T) {
	got := UserMessage(ExtractInput{Entry: domain.Entry{Body: "hello"}})
	for _, want := range []string{"Entry scoped to: none", "# Projects\n(none)", "# Parked ideas\n(none)", "<entry>\nhello\n</entry>"} {
		if !strings.Contains(got, want) {
			t.Errorf("UserMessage lacks %q:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{"Recent entries", "earlier question"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("UserMessage has an empty %q section", unwanted)
		}
	}
}

func TestSystemPromptNamesTool(t *testing.T) {
	if !strings.Contains(SystemPrompt(MaxQuestions), ToolName) {
		t.Errorf("system prompt does not name the %s tool", ToolName)
	}
}
