package apitypes

import (
	"time"

	"github.com/ogredforman/waypoint/internal/domain"
)

// Milestone is an outcome-shaped chunk of a project; tasks point at it with
// milestone_id.
type Milestone struct {
	ID          int64                  `json:"id"`
	ProjectID   int64                  `json:"project_id"`
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	Status      domain.MilestoneStatus `json:"status"`
	Position    int64                  `json:"position"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

// CreateMilestoneRequest is the body of POST /api/projects/{id}/milestones.
// A milestone starts open, after the project's other milestones.
type CreateMilestoneRequest struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
}

// UpdateMilestoneRequest is the body of PATCH /api/milestones/{id}. Only the
// fields present change; position orders it within its project.
type UpdateMilestoneRequest struct {
	Title       *string                 `json:"title,omitempty"`
	Description *string                 `json:"description,omitempty"`
	Status      *domain.MilestoneStatus `json:"status,omitempty"`
	Position    *int64                  `json:"position,omitempty"`
}

// MoveMilestoneRequest is the body of POST /api/milestones/{id}/move. The
// milestone swaps position with its neighbour in the project; the response
// is the milestones whose position changed, none when it is already at that
// end.
type MoveMilestoneRequest struct {
	Direction MoveDirection `json:"direction"`
}
