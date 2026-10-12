package apitypes

import (
	"time"

	"github.com/ogredforman/waypoint/internal/domain"
)

type Entry struct {
	ID                int64     `json:"id"`
	Body              string    `json:"body"`
	ProjectID         *int64    `json:"project_id" tstype:"number | null,required"`
	ReplyToQuestionID *int64    `json:"reply_to_question_id" tstype:"number | null,required"`
	CreatedAt         time.Time `json:"created_at"`
}

// EntryPage is the body of GET /api/entries: entries newest first. When
// there may be more, NextBefore is the before= value for the next page.
type EntryPage struct {
	Entries    []Entry `json:"entries"`
	NextBefore *int64  `json:"next_before" tstype:"number | null,required"`
}

// CreateEntryRequest is the body of POST /api/entries. Replying to a
// follow-up question marks it answered.
type CreateEntryRequest struct {
	Body              string `json:"body"`
	ProjectID         *int64 `json:"project_id,omitempty"`
	ReplyToQuestionID *int64 `json:"reply_to_question_id,omitempty"`
}

// CreateEntryResponse is the body of a 202 from POST /api/entries: the entry
// is stored and its extraction run queued. SSE events report the outcome.
type CreateEntryResponse struct {
	Entry Entry         `json:"entry"`
	Run   ExtractionRun `json:"run"`
}

type ExtractionRun struct {
	ID      int64                      `json:"id"`
	EntryID int64                      `json:"entry_id"`
	Status  domain.ExtractionRunStatus `json:"status"`
	Error   *string                    `json:"error" tstype:"string | null,required"`
	// Failure is why a failed run failed, for the UI to say plainly; null
	// unless the run failed.
	Failure   *string   `json:"failure" tstype:"'timeout' | 'unreachable' | 'invalid' | 'error' | null,required"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// EntryDetail is the body of GET /api/entries/{id}.
type EntryDetail struct {
	Entry Entry `json:"entry"`
	// Run is the entry's latest extraction run.
	Run *ExtractionRun `json:"run" tstype:"ExtractionRun | null,required"`
	// Observations are those of the latest run that produced any, in text
	// order.
	Observations []Observation `json:"observations"`
	// Questions are the follow-up questions from all runs, oldest first.
	Questions []Question `json:"questions"`
	// Changesets are those proposed from the entry, oldest first, including
	// split-offs.
	Changesets []Changeset `json:"changesets"`
}

// Observation is a typed fragment of an entry. SpanStart and SpanEnd index
// the entry body in UTF-16 code units, as JavaScript strings do.
type Observation struct {
	ID        int64                  `json:"id"`
	Kind      domain.ObservationKind `json:"kind"`
	Summary   string                 `json:"summary"`
	SpanStart int64                  `json:"span_start"`
	SpanEnd   int64                  `json:"span_end"`
	Links     []ObservationLink      `json:"links"`
}

// ObservationLink ties an observation to a project element.
type ObservationLink struct {
	TargetType domain.TargetType `json:"target_type"`
	TargetID   int64             `json:"target_id"`
	Confidence float64           `json:"confidence"`
}

// Question is a follow-up question asked after an entry.
type Question struct {
	ID            int64                 `json:"id"`
	EntryID       int64                 `json:"entry_id"`
	Text          string                `json:"text"`
	Reason        string                `json:"reason"`
	Status        domain.QuestionStatus `json:"status"`
	AnswerEntryID *int64                `json:"answer_entry_id" tstype:"number | null,required"`
	CreatedAt     time.Time             `json:"created_at"`
}
