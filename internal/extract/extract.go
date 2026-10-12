package extract

import (
	"context"
	"encoding/json"

	"github.com/ogredforman/waypoint/internal/changeset"
	"github.com/ogredforman/waypoint/internal/domain"
)

// MaxQuestions is the most follow-up questions one entry may ever get (SPEC
// §5). The worker keeps fewer when the questions-per-entry setting is lower.
const MaxQuestions = domain.MaxQuestionsPerEntry

// Extractor turns one diary entry into proposed observations, ops, and
// questions. Its output is untrusted until Validate accepts it.
type Extractor interface {
	Extract(ctx context.Context, in ExtractInput) (ExtractionResult, error)
	Name() string // "mock", "anthropic", ...
}

// ExtractInput is everything an extractor sees: the entry, a snapshot of the
// state it may refer to, the question the entry answers, if any, and how many
// questions to ask at most (the questions-per-entry setting).
type ExtractInput struct {
	Entry         domain.Entry
	Snapshot      StateSnapshot
	ReplyTo       *domain.FollowUpQuestion // nil unless Entry.ReplyToQuestionID is set
	MaxQuestions  int                      // 0..MaxQuestions; the worker drops any extras
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
// An LLM extractor gets a Quote from the model instead, copied from the body,
// and sets the span from it with ResolveQuotes.
type ObservationDraft struct {
	Kind      domain.ObservationKind `json:"kind"`
	Summary   string                 `json:"summary"`
	Quote     string                 `json:"quote,omitempty"`
	SpanStart int64                  `json:"span_start"`
	SpanEnd   int64                  `json:"span_end"`
	Links     []LinkDraft            `json:"links,omitempty"`
}

// LinkDraft ties an observation to an existing row from the snapshot, or, with
// a temp ID ("$new:N"), to the entity the op at position N creates. A temp
// link is stored pending and becomes a real link when that op is applied; it
// is dropped if the op is rejected.
type LinkDraft struct {
	TargetType domain.TargetType `json:"target_type"`
	TargetID   changeset.Ref     `json:"target_id"`
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
