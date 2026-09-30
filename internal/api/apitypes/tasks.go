package apitypes

import (
	"time"

	"github.com/ogredforman/waypoint/internal/domain"
)

type Task struct {
	ID            int64             `json:"id"`
	ProjectID     int64             `json:"project_id"`
	ParentTaskID  *int64            `json:"parent_task_id" tstype:"number | null,required"`
	Title         string            `json:"title"`
	Notes         string            `json:"notes"`
	Status        domain.TaskStatus `json:"status"`
	Size          *domain.TaskSize  `json:"size" tstype:"domain.TaskSize | null,required"`
	Energy        *domain.Energy    `json:"energy" tstype:"domain.Energy | null,required"`
	Position      int64             `json:"position"`
	OriginEntryID *int64            `json:"origin_entry_id" tstype:"number | null,required"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

// CreateTaskRequest is the body of POST /api/projects/{id}/tasks. A task
// starts as todo unless Status says otherwise.
type CreateTaskRequest struct {
	ParentTaskID *int64             `json:"parent_task_id,omitempty"`
	Title        string             `json:"title"`
	Notes        string             `json:"notes,omitempty"`
	Size         *domain.TaskSize   `json:"size,omitempty"`
	Energy       *domain.Energy     `json:"energy,omitempty"`
	Status       *domain.TaskStatus `json:"status,omitempty"`
}

// UpdateTaskRequest is the body of PATCH /api/tasks/{id}. Only the fields
// present change; null clears size, energy, or parent_task_id.
type UpdateTaskRequest struct {
	Title        *string            `json:"title,omitempty"`
	Notes        *string            `json:"notes,omitempty"`
	Size         *domain.TaskSize   `json:"size,omitempty" tstype:"domain.TaskSize | null"`
	Energy       *domain.Energy     `json:"energy,omitempty" tstype:"domain.Energy | null"`
	ParentTaskID *int64             `json:"parent_task_id,omitempty" tstype:"number | null"`
	Position     *int64             `json:"position,omitempty"`
	Status       *domain.TaskStatus `json:"status,omitempty"`
}

// TaskDetail is the body of GET /api/tasks/{id}: the task, the entry it came
// from, and everything written about it.
type TaskDetail struct {
	Task        Task   `json:"task"`
	OriginEntry *Entry `json:"origin_entry" tstype:"Entry | null,required"`
	// Observations are those linked to the task, oldest first. Of a
	// reprocessed entry, only the latest run's count.
	Observations []LinkedObservation `json:"observations"`
}

// LinkedObservation is an observation linked to a task, with the words of
// the entry it quotes.
type LinkedObservation struct {
	ID             int64                  `json:"id"`
	EntryID        int64                  `json:"entry_id"`
	EntryCreatedAt time.Time              `json:"entry_created_at"`
	Kind           domain.ObservationKind `json:"kind"`
	Summary        string                 `json:"summary"`
	Quote          string                 `json:"quote"`
	Confidence     float64                `json:"confidence"`
}
