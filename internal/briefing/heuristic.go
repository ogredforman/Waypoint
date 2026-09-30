package briefing

import (
	"context"
	"strings"

	"github.com/ogredforman/waypoint/internal/domain"
)

const (
	// todoWindow is how many startable todo tasks, from the top of the list,
	// the heuristic weighs by size.
	todoWindow = 3
	// maxAlternatives caps the low-energy alternatives.
	maxAlternatives = 2
	// recapEntries is how many recent entries the recap covers.
	recapEntries = 3
	// recapSummaries caps the summaries quoted per entry.
	recapSummaries = 2
)

// Reasons the heuristic gives. They are plain and never count what hasn't
// been done (SPEC §5).
const (
	reasonInProgress  = "You were in the middle of this."
	reasonNext        = "It's next on your list."
	reasonNextSmall   = "It's next on your list and small enough to start now."
	reasonSmall       = "It's near the top of your list and small enough to start now."
	reasonSmaller     = "It's near the top of your list and a smaller step than the one above it."
	reasonAllWaiting  = "Everything open is waiting on a blocker or another task; clearing one of those is the way forward."
	reasonNoOpenTasks = "There are no open tasks yet. Write about what you want to do next and it can become one."
)

// Heuristic is the rule-based Briefer (SPEC §7.3), used in the prototype and
// as the fallback for the LLM one. Among the tasks that can be started now,
// it prefers an in-progress one, then the smallest of the first few todo
// tasks by position.
type Heuristic struct{}

func (Heuristic) Brief(_ context.Context, st State) (domain.Briefing, error) {
	b := domain.Briefing{ProjectID: st.Project.ID, Source: domain.BriefingHeuristic, Recap: recap(st.RecentEntries)}
	next, reason := pickNext(st)
	if next != nil {
		b.NextTaskID = &next.ID
	}
	b.NextReason = reason
	return b, nil
}

func isOpen(t domain.Task) bool {
	return t.Status == domain.TaskTodo || t.Status == domain.TaskInProgress
}

// startable returns the open tasks that can be started now, in position
// order. A task can't be if it has an open blocker, depends on an open task,
// or has open subtasks: those are the steps to start instead.
func startable(st State) []domain.Task {
	open := map[int64]bool{}
	for _, t := range st.Tasks {
		open[t.ID] = isOpen(t)
	}
	waiting := map[int64]bool{}
	for _, bl := range st.Blockers {
		if bl.Status == domain.BlockerOpen && bl.TaskID != nil {
			waiting[*bl.TaskID] = true
		}
	}
	for _, d := range st.Dependencies {
		if open[d.BlockedByTaskID] {
			waiting[d.TaskID] = true
		}
	}
	for _, t := range st.Tasks {
		if t.ParentTaskID != nil && isOpen(t) {
			waiting[*t.ParentTaskID] = true
		}
	}
	out := []domain.Task{}
	for _, t := range st.Tasks {
		if isOpen(t) && !waiting[t.ID] {
			out = append(out, t)
		}
	}
	return out
}

// sizeRank orders sizes for "prefer smaller"; an unsized task counts as
// medium.
func sizeRank(s *domain.TaskSize) int {
	if s == nil {
		return 2
	}
	switch *s {
	case domain.SizeXS:
		return 0
	case domain.SizeS:
		return 1
	case domain.SizeM:
		return 2
	default:
		return 3
	}
}

func isSmall(t domain.Task) bool { return sizeRank(t.Size) <= 1 }

// pickNext returns the next action and why, or nil and a reason when no open
// task can be started.
func pickNext(st State) (*domain.Task, string) {
	ready := startable(st)

	// The most recently touched in-progress task is where the user left off.
	var next *domain.Task
	for i, t := range ready {
		if t.Status == domain.TaskInProgress && (next == nil || t.UpdatedAt.After(next.UpdatedAt)) {
			next = &ready[i]
		}
	}
	if next != nil {
		return next, reasonInProgress
	}

	var todo []domain.Task
	for _, t := range ready {
		if t.Status == domain.TaskTodo && len(todo) < todoWindow {
			todo = append(todo, t)
		}
	}
	if len(todo) == 0 {
		for _, t := range st.Tasks {
			if isOpen(t) {
				return nil, reasonAllWaiting
			}
		}
		return nil, reasonNoOpenTasks
	}
	best := 0
	for i, t := range todo {
		if sizeRank(t.Size) < sizeRank(todo[best].Size) {
			best = i
		}
	}
	next = &todo[best]
	switch {
	case best == 0 && isSmall(*next):
		return next, reasonNextSmall
	case best == 0:
		return next, reasonNext
	case isSmall(*next):
		return next, reasonSmall
	default:
		return next, reasonSmaller
	}
}

// alternatives returns up to maxAlternatives small or low-energy tasks that
// can be started now, other than nextID, in position order.
func alternatives(st State, nextID *int64) []domain.Task {
	out := []domain.Task{}
	for _, t := range startable(st) {
		if len(out) == maxAlternatives {
			break
		}
		if nextID != nil && t.ID == *nextID {
			continue
		}
		low := t.Energy != nil && *t.Energy == domain.EnergyLow
		if low || isSmall(t) {
			out = append(out, t)
		}
	}
	return out
}

// recap writes one sentence per recent entry that has summaries, oldest
// first, so it reads up to where the user left off. It is empty when there
// is nothing to say.
func recap(entries []RecentEntry) string {
	var sentences []string // newest first
	for _, e := range entries {
		if len(sentences) == recapEntries {
			break
		}
		var parts []string
		for _, s := range e.Summaries {
			s = strings.TrimRight(strings.TrimSpace(s), ".")
			if s != "" && len(parts) < recapSummaries {
				parts = append(parts, s)
			}
		}
		if len(parts) > 0 {
			sentences = append(sentences, strings.Join(parts, "; ")+".")
		}
	}
	out := make([]string, len(sentences))
	for i, s := range sentences {
		label := "Then: "
		switch {
		case i == 0:
			label = "Most recently: "
		case i == len(sentences)-1:
			label = "Earlier: "
		}
		out[len(sentences)-1-i] = label + s
	}
	return strings.Join(out, " ")
}
