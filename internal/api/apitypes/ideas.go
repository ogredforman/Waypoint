package apitypes

import (
	"time"

	"github.com/ogredforman/waypoint/internal/domain"
)

type Idea struct {
	ID            int64             `json:"id"`
	ProjectID     *int64            `json:"project_id" tstype:"number | null,required"`
	Title         string            `json:"title"`
	Body          string            `json:"body"`
	Status        domain.IdeaStatus `json:"status"`
	OriginEntryID *int64            `json:"origin_entry_id" tstype:"number | null,required"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

// UpdateIdeaRequest is the body of PATCH /api/ideas/{id}. Only the fields
// present change; a null project_id unties the idea from its project.
// Promoting goes through POST /api/ideas/{id}/promote instead.
type UpdateIdeaRequest struct {
	Title     *string            `json:"title,omitempty"`
	Body      *string            `json:"body,omitempty"`
	ProjectID *int64             `json:"project_id,omitempty" tstype:"number | null"`
	Status    *domain.IdeaStatus `json:"status,omitempty" tstype:"domain.IdeaStatus & ('parked' | 'discarded')"`
}

// PromoteTarget is what a parked idea becomes.
type PromoteTarget string

const (
	PromoteTargetTask    PromoteTarget = "task"
	PromoteTargetProject PromoteTarget = "project"
)

// PromoteIdeaRequest is the body of POST /api/ideas/{id}/promote. A task
// goes into ProjectID, or the idea's own project if that is unset. A
// project is created parked.
type PromoteIdeaRequest struct {
	To        PromoteTarget `json:"to"`
	ProjectID *int64        `json:"project_id,omitempty"`
}

// PromoteIdeaResponse is the promoted idea and what it became: Task or
// Project, whichever was asked for.
type PromoteIdeaResponse struct {
	Idea    Idea     `json:"idea"`
	Task    *Task    `json:"task" tstype:"Task | null,required"`
	Project *Project `json:"project" tstype:"Project | null,required"`
}
