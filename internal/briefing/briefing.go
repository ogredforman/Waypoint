package briefing

import (
	"context"
	"time"

	"github.com/ogredforman/waypoint/internal/domain"
)

// Briefer decides a project's next action and writes its recap. Brief returns
// the briefing's ProjectID, NextTaskID, NextReason, Recap, and Source; the
// Service stamps the rest. A NextTaskID must be one of st's open tasks. The
// recap is a few of st.RecentEntries, newest first, each as short points of an
// observation kind; the Service drops entries that aren't among them.
type Briefer interface {
	Brief(ctx context.Context, st State) (domain.Briefing, error)
}

// State is what a Briefer sees of one project.
type State struct {
	Project domain.Project
	// Tasks are all the project's tasks, open or not, in position order.
	Tasks []domain.Task
	// Dependencies are those of the project's tasks.
	Dependencies []domain.TaskDependency
	// Milestones are the project's milestones, open or not, in order.
	Milestones []domain.Milestone
	// Blockers are the project's open blockers, oldest first.
	Blockers []domain.Blocker
	// RecentEntries are the latest entries about the project, newest first.
	RecentEntries []RecentEntry
}

// RecentEntry is an entry as its latest run's observations, each a kind and a
// summary, not its body.
type RecentEntry struct {
	ID           int64
	CreatedAt    time.Time
	Observations []domain.RecapPoint
}

// View is a project's briefing as the "Now" screen shows it: the cached
// judgment, plus the parts that are plain facts about current state and so
// are read fresh each time.
type View struct {
	Briefing domain.Briefing
	// Next is the task Briefing.NextTaskID names, or nil if there is none.
	Next *domain.Task
	// Step places Next among its parent's steps, or is nil if Next is not a
	// subtask.
	Step *Step
	// Excerpt is the most recent entry text about Next, or nil if none.
	Excerpt *Excerpt
	// Blockers are the project's open blockers, oldest first.
	Blockers []domain.Blocker
	// Alternatives are up to 2 small or low-energy tasks that can be started
	// now, other than Next.
	Alternatives []domain.Task
}

// Step is a subtask's place in its parent's steps.
type Step struct {
	Parent domain.Task
	// Number counts from 1, by position (ties by id).
	Number int
}

// Excerpt is the part of an entry an observation covers.
type Excerpt struct {
	EntryID   int64
	CreatedAt time.Time
	Text      string
}
