package apitypes

import (
	"time"

	"github.com/ogredforman/waypoint/internal/domain"
)

type Project struct {
	ID               int64                `json:"id"`
	Title            string               `json:"title"`
	Why              string               `json:"why"`
	DefinitionOfDone string               `json:"definition_of_done"`
	Status           domain.ProjectStatus `json:"status"`
	CreatedAt        time.Time            `json:"created_at"`
	UpdatedAt        time.Time            `json:"updated_at"`
}

// CreateProjectRequest is the body of POST /api/projects. A project is
// created parked unless Status says otherwise; active counts against the
// active-project cap.
type CreateProjectRequest struct {
	Title            string                `json:"title"`
	Why              string                `json:"why,omitempty"`
	DefinitionOfDone string                `json:"definition_of_done,omitempty"`
	Status           *domain.ProjectStatus `json:"status,omitempty"`
}

// UpdateProjectRequest is the body of PATCH /api/projects/{id}. Only the
// fields present change.
type UpdateProjectRequest struct {
	Title            *string               `json:"title,omitempty"`
	Why              *string               `json:"why,omitempty"`
	DefinitionOfDone *string               `json:"definition_of_done,omitempty"`
	Status           *domain.ProjectStatus `json:"status,omitempty"`
}

// Briefing is the body of GET /api/projects/{id}/briefing: the project's
// one next action and the context around it.
type Briefing struct {
	ProjectID  int64                 `json:"project_id"`
	NextTask   *Task                 `json:"next_task" tstype:"Task | null,required"`
	NextReason string                `json:"next_reason"`
	Recap      string                `json:"recap"`
	Source     domain.BriefingSource `json:"source"`
	// Excerpt is the most recent entry text about NextTask.
	Excerpt *Excerpt `json:"excerpt" tstype:"Excerpt | null,required"`
	// Blockers are the project's open blockers, oldest first.
	Blockers []Blocker `json:"blockers"`
	// Alternatives are up to 2 small or low-energy tasks that can be
	// started now instead.
	Alternatives []Task    `json:"alternatives"`
	GeneratedAt  time.Time `json:"generated_at"`
}

// Excerpt is a piece of an entry quoted in a briefing.
type Excerpt struct {
	EntryID   int64     `json:"entry_id"`
	CreatedAt time.Time `json:"created_at"`
	Text      string    `json:"text"`
}
