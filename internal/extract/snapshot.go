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
// with its open tasks and blockers.
type SnapshotProject struct {
	ID       int64                `json:"id"`
	Title    string               `json:"title"`
	Status   domain.ProjectStatus `json:"status"`
	Tasks    []SnapshotTask       `json:"tasks"`
	Blockers []SnapshotBlocker    `json:"blockers"`
}

// SnapshotTask is an open (todo or in-progress) task.
type SnapshotTask struct {
	ID           int64             `json:"id"`
	Title        string            `json:"title"`
	Status       domain.TaskStatus `json:"status"`
	ParentTaskID *int64            `json:"parent_task_id,omitempty"`
	UpdatedAt    time.Time         `json:"updated_at"`
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
// its observations rather than its body.
type SnapshotEntry struct {
	ID        int64     `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	Summaries []string  `json:"summaries"`
}

// Has reports whether the snapshot contains the row of kind k with the given
// ID. Milestones are never in it.
func (s StateSnapshot) Has(k domain.TargetType, id int64) bool {
	for _, p := range s.Projects {
		switch k {
		case domain.TargetProject:
			if p.ID == id {
				return true
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
