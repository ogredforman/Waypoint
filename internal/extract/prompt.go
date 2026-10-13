package extract

import (
	_ "embed"
	"fmt"
	"strings"
	"text/template"
	"time"
)

// PromptVersion names the system prompt and the user message format below.
// Bump it whenever either changes, so runs and eval scores stay comparable.
const PromptVersion = "v5"

//go:embed prompt_system.md
var systemPromptText string

var systemPrompt = template.Must(template.New("system").Parse(systemPromptText))

// SystemPrompt is the instructions every LLM extractor sends, asking for at
// most maxQuestions follow-up questions (clamped to 0..MaxQuestions).
func SystemPrompt(maxQuestions int) string {
	maxQuestions = min(max(maxQuestions, 0), MaxQuestions)
	var b strings.Builder
	if err := systemPrompt.Execute(&b, struct{ MaxQuestions int }{maxQuestions}); err != nil {
		panic(fmt.Sprintf("system prompt: %v", err)) // the template is fixed; tests execute it
	}
	return b.String()
}

// UserMessage formats an extraction's input for the model: today's date, the
// entry's scope, the snapshot, the question being answered, the entry, and on
// a retry, the rejected result with what was wrong with it.
// Rows are written as "<kind> <id>" so the model returns the kind and ID it
// was shown. The output is deterministic for a given input.
func UserMessage(in ExtractInput) string {
	var b strings.Builder
	snap := in.Snapshot
	fmt.Fprintf(&b, "Today: %s\n", day(snap.TakenAt))

	scope := "none (pick the project it is clearly about, if any)"
	if id := in.Entry.ProjectID; id != nil {
		scope = fmt.Sprintf("project %d", *id)
		for _, p := range snap.Projects {
			if p.ID == *id {
				scope += " " + quote(p.Title)
			}
		}
	}
	fmt.Fprintf(&b, "Entry scoped to: %s\n", scope)

	b.WriteString("\n# Projects\n")
	if len(snap.Projects) == 0 {
		b.WriteString("(none)\n")
	}
	for _, p := range snap.Projects {
		fmt.Fprintf(&b, "\nproject %d %s (%s)\n", p.ID, quote(p.Title), p.Status)
		if len(p.Milestones) > 0 {
			b.WriteString("Open milestones, in order:\n")
			for _, m := range p.Milestones {
				fmt.Fprintf(&b, "- milestone %d: %s\n", m.ID, quote(m.Title))
			}
		}
		writeTasks(&b, p.Tasks)
		if len(p.Blockers) > 0 {
			b.WriteString("Open blockers:\n")
			for _, bl := range p.Blockers {
				on := ""
				if bl.TaskID != nil {
					on = fmt.Sprintf(" (on task %d)", *bl.TaskID)
				}
				fmt.Fprintf(&b, "- blocker %d%s: %s\n", bl.ID, on, quote(bl.Description))
			}
		}
	}

	b.WriteString("\n# Parked ideas\n")
	if len(snap.Ideas) == 0 {
		b.WriteString("(none)\n")
	}
	for _, i := range snap.Ideas {
		of := ""
		if i.ProjectID != nil {
			of = fmt.Sprintf(" (project %d)", *i.ProjectID)
		}
		fmt.Fprintf(&b, "- idea %d%s: %s\n", i.ID, of, quote(i.Title))
	}

	if len(snap.RecentEntries) > 0 {
		b.WriteString("\n# Recent entries, newest first (summaries only)\n")
		for _, e := range snap.RecentEntries {
			fmt.Fprintf(&b, "- %s: %s\n", day(e.CreatedAt), strings.Join(e.Summaries, " "))
		}
	}

	if q := in.ReplyTo; q != nil {
		b.WriteString("\n# This entry answers your earlier question\n")
		fmt.Fprintf(&b, "%s\n", quote(q.Text))
	}

	fmt.Fprintf(&b, "\n# Entry, written %s\n<entry>\n%s\n</entry>\n", day(in.Entry.CreatedAt), in.Entry.Body)

	if r := in.Retry; r != nil {
		b.WriteString("\n# Your previous answer was rejected\nThese problems were found in it:\n")
		for _, p := range r.Problems {
			fmt.Fprintf(&b, "- %s\n", p)
		}
		if len(r.Previous) > 0 {
			fmt.Fprintf(&b, "Your previous answer:\n<previous_answer>\n%s\n</previous_answer>\n", r.Previous)
		}
		fmt.Fprintf(&b, "Call %s again with a complete, corrected answer. Refer only to rows listed above, or to \"$new:N\" for what op N creates.\n", ToolName)
	}
	return b.String()
}

// writeTasks lists open tasks, each step under its parent. A step whose
// parent isn't open is listed at the top level.
func writeTasks(b *strings.Builder, tasks []SnapshotTask) {
	b.WriteString("Open tasks:\n")
	if len(tasks) == 0 {
		b.WriteString("(none)\n")
		return
	}
	open := make(map[int64]bool, len(tasks))
	for _, t := range tasks {
		open[t.ID] = true
	}
	steps := make(map[int64][]SnapshotTask)
	for _, t := range tasks {
		if t.ParentTaskID != nil && open[*t.ParentTaskID] {
			steps[*t.ParentTaskID] = append(steps[*t.ParentTaskID], t)
		}
	}
	var write func(t SnapshotTask, depth int)
	write = func(t SnapshotTask, depth int) {
		var about []string
		if t.MilestoneID != nil {
			about = append(about, fmt.Sprintf("milestone %d", *t.MilestoneID))
		}
		if t.Size != nil {
			about = append(about, fmt.Sprintf("size %s", *t.Size))
		}
		about = append(about, "updated "+day(t.UpdatedAt))
		if t.Stalled {
			about = append(about, "stalled")
		}
		fmt.Fprintf(b, "%s- task %d [%s] %s (%s)\n", strings.Repeat("  ", depth), t.ID, t.Status, quote(t.Title), strings.Join(about, ", "))
		for _, s := range steps[t.ID] {
			write(s, depth+1)
		}
	}
	for _, t := range tasks {
		if t.ParentTaskID == nil || !open[*t.ParentTaskID] {
			write(t, 0)
		}
	}
}

func day(t time.Time) string { return t.UTC().Format("2006-01-02") }

// quote writes a title or description as a Go string literal, so quotes and
// newlines in it can't break the layout.
func quote(s string) string { return fmt.Sprintf("%q", s) }
