package extract

import (
	"time"

	"github.com/ogredforman/waypoint/internal/domain"
)

// StateSnapshot is the compact state an extractor sees (SPEC §7.1, step 3).
// The pipeline builds it; it is stored as the run's input snapshot. Ops and
// links may only refer to existing rows that appear in it.
type StateSnapshot struct {
	TakenAt       time.Time         `json:"taken_at"`
	Projects      []SnapshotProject `json:"projects"`
	Ideas         []SnapshotIdea    `json:"ideas"`
	RecentEntries []SnapshotEntry   `json:"recent_entries"`
}

// SnapshotProject is an active project, or the one the entry is scoped to,
// with its open milestones (in order), open tasks, and open blockers.
type SnapshotProject struct {
	ID         int64                `json:"id"`
	Title      string               `json:"title"`
	Status     domain.ProjectStatus `json:"status"`
	Milestones []SnapshotMilestone  `json:"milestones"`
	Tasks      []SnapshotTask       `json:"tasks"`
	Blockers   []SnapshotBlocker    `json:"blockers"`
}

// SnapshotMilestone is an open milestone.
type SnapshotMilestone struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
}

// SnapshotTask is an open (todo or in-progress) task. MilestoneID is set only
// when the task's milestone is open, so it names a milestone in the snapshot.
// Stalled marks an in-progress task that, with its steps, hasn't changed for
// a while (the pipeline decides how long); the prompt asks the model to
// suggest splitting such a task when the entry is about it.
type SnapshotTask struct {
	ID           int64             `json:"id"`
	Title        string            `json:"title"`
	Status       domain.TaskStatus `json:"status"`
	ParentTaskID *int64            `json:"parent_task_id,omitempty"`
	MilestoneID  *int64            `json:"milestone_id,omitempty"`
	Size         *domain.TaskSize  `json:"size,omitempty"`
	UpdatedAt    time.Time         `json:"updated_at"`
	Stalled      bool              `json:"stalled,omitempty"`
}

// SnapshotBlocker is an open blocker.
type SnapshotBlocker struct {
	ID          int64  `json:"id"`
	TaskID      *int64 `json:"task_id,omitempty"`
	Description string `json:"description"`
}

// SnapshotIdea is a parked idea, title only.
type SnapshotIdea struct {
	ID        int64  `json:"id"`
	ProjectID *int64 `json:"project_id,omitempty"`
	Title     string `json:"title"`
}

// SnapshotEntry is a recent entry of the relevant project, as the summaries of
// its observations rather than its body, with the follow-up questions asked
// after it, so the model doesn't ask them again right away (SPEC §7.2).
type SnapshotEntry struct {
	ID        int64              `json:"id"`
	CreatedAt time.Time          `json:"created_at"`
	Summaries []string           `json:"summaries"`
	Questions []SnapshotQuestion `json:"questions,omitempty"`
}

// SnapshotQuestion is a follow-up question asked after a recent entry, and
// whether it is still open, was answered, or was dismissed.
type SnapshotQuestion struct {
	Text   string                `json:"text"`
	Status domain.QuestionStatus `json:"status"`
}

// Has reports whether the snapshot contains the row of kind k with the given
// ID.
func (s StateSnapshot) Has(k domain.TargetType, id int64) bool {
	for _, p := range s.Projects {
		switch k {
		case domain.TargetProject:
			if p.ID == id {
				return true
			}
		case domain.TargetMilestone:
			for _, m := range p.Milestones {
				if m.ID == id {
					return true
				}
			}
		case domain.TargetTask:
			for _, t := range p.Tasks {
				if t.ID == id {
					return true
				}
			}
		case domain.TargetBlocker:
			for _, b := range p.Blockers {
				if b.ID == id {
					return true
				}
			}
		}
	}
	if k == domain.TargetIdea {
		for _, i := range s.Ideas {
			if i.ID == id {
				return true
			}
		}
	}
	return false
}
