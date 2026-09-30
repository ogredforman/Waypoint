package extract

import (
	"context"
	"encoding/json"

	"github.com/ogredforman/waypoint/internal/domain"
)

// MaxQuestions is the most follow-up questions one entry may get (SPEC §5).
const MaxQuestions = 2

// Extractor turns one diary entry into proposed observations, ops, and
// questions. Its output is untrusted until Validate accepts it.
type Extractor interface {
	Extract(ctx context.Context, in ExtractInput) (ExtractionResult, error)
	Name() string // "mock", "anthropic", ...
}

// ExtractInput is everything an extractor sees: the entry and a snapshot of
// the state it may refer to.
type ExtractInput struct {
	Entry         domain.Entry
	Snapshot      StateSnapshot
	PromptVersion string
}

// ExtractionResult is an extractor's output. It is stored as the run's raw
// output, so it has JSON tags.
type ExtractionResult struct {
	Observations []ObservationDraft `json:"observations"`
	Ops          []ChangeOpDraft    `json:"ops"`
	Questions    []QuestionDraft    `json:"questions"`
}

// ObservationDraft is a typed fragment of the entry. SpanStart and SpanEnd are
// byte offsets into the entry body, on UTF-8 character boundaries. An empty
// span (SpanStart == SpanEnd) means the observation isn't tied to any text.
type ObservationDraft struct {
	Kind      domain.ObservationKind `json:"kind"`
	Summary   string                 `json:"summary"`
	SpanStart int64                  `json:"span_start"`
	SpanEnd   int64                  `json:"span_end"`
	Links     []LinkDraft            `json:"links,omitempty"`
}

// LinkDraft ties an observation to an existing row from the snapshot. It can't
// point at an entity an op creates: observations are stored before the
// changeset is applied.
type LinkDraft struct {
	TargetType domain.TargetType `json:"target_type"`
	TargetID   int64             `json:"target_id"`
	Confidence *float64          `json:"confidence"` // required; a pointer so a missing value isn't read as 0
}

// ChangeOpDraft is one proposed op. Its position in the changeset is its index
// in ExtractionResult.Ops plus one, which is what "$new:N" refs name.
type ChangeOpDraft struct {
	OpType     domain.OpType   `json:"op_type"`
	Payload    json.RawMessage `json:"payload"`
	Rationale  string          `json:"rationale,omitempty"`
	Confidence *float64        `json:"confidence,omitempty"`
}

// QuestionDraft is a follow-up question, with why it's being asked.
type QuestionDraft struct {
	Text   string `json:"text"`
	Reason string `json:"reason"`
}
