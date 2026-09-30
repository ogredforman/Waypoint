package apitypes

import (
	"time"

	"github.com/ogredforman/waypoint/internal/domain"
)

type Blocker struct {
	ID              int64                `json:"id"`
	ProjectID       int64                `json:"project_id"`
	TaskID          *int64               `json:"task_id" tstype:"number | null,required"`
	Description     string               `json:"description"`
	Status          domain.BlockerStatus `json:"status"`
	OriginEntryID   *int64               `json:"origin_entry_id" tstype:"number | null,required"`
	ResolvedEntryID *int64               `json:"resolved_entry_id" tstype:"number | null,required"`
	CreatedAt       time.Time            `json:"created_at"`
	UpdatedAt       time.Time            `json:"updated_at"`
}

// UpdateBlockerRequest is the body of PATCH /api/blockers/{id}. Only the
// fields present change; a null task_id leaves the blocker on the project
// alone. Status can only be set to resolved.
type UpdateBlockerRequest struct {
	Description *string               `json:"description,omitempty"`
	TaskID      *int64                `json:"task_id,omitempty" tstype:"number | null"`
	Status      *domain.BlockerStatus `json:"status,omitempty" tstype:"domain.BlockerStatus & 'resolved'"`
}
