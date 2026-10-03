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

// ProjectObservations is the body of GET /api/projects/{id}/observations:
// what the diary says about the project, the observations linked to it or to
// its tasks or blockers. Newest entry first, an entry's observations in text
// order; of a reprocessed entry, only the latest run's count. A page holds
// whole entries; when there may be more, NextBefore is the before= value for
// the next page.
type ProjectObservations struct {
	Observations []LinkedObservation `json:"observations"`
	NextBefore   *int64              `json:"next_before" tstype:"number | null,required"`
}

// Briefing is the body of GET /api/projects/{id}/briefing: the project's
// one next action and the context around it.
type Briefing struct {
	ProjectID  int64  `json:"project_id"`
	NextTask   *Task  `json:"next_task" tstype:"Task | null,required"`
	NextReason string `json:"next_reason"`
	// Recap is where the user left off: a few recent entries about the
	// project, newest first.
	Recap  []RecapEntry          `json:"recap"`
	Source domain.BriefingSource `json:"source"`
	// Excerpt is the most recent entry text about NextTask.
	Excerpt *Excerpt `json:"excerpt" tstype:"Excerpt | null,required"`
	// Blockers are the project's open blockers, oldest first.
	Blockers []Blocker `json:"blockers"`
	// Alternatives are up to 2 small or low-energy tasks that can be
	// started now instead.
	Alternatives []Task    `json:"alternatives"`
	GeneratedAt  time.Time `json:"generated_at"`
}

// RecapEntry is one entry in a briefing's recap, as a few short points.
type RecapEntry struct {
	EntryID   int64        `json:"entry_id"`
	CreatedAt time.Time    `json:"created_at"`
	Points    []RecapPoint `json:"points"`
}

// RecapPoint is one thing an entry said, with its observation kind.
type RecapPoint struct {
	Kind domain.ObservationKind `json:"kind"`
	Text string                 `json:"text"`
}

// Excerpt is a piece of an entry quoted in a briefing.
type Excerpt struct {
	EntryID   int64     `json:"entry_id"`
	CreatedAt time.Time `json:"created_at"`
	Text      string    `json:"text"`
}
