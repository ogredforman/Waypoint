package domain

import (
	"encoding/json"
	"time"
)

// Entity types from SPEC §6.1. IDs are SQLite rowids; zero means "not saved
// yet". Optional references and fields are pointers, nil when unset.

// Project is a goal the user is pursuing.
type Project struct {
	ID               int64
	Title            string
	Why              string
	DefinitionOfDone string
	Status           ProjectStatus
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// Milestone is an outcome-shaped chunk of a project. Its tasks point at it.
type Milestone struct {
	ID          int64
	ProjectID   int64
	Title       string
	Description string
	Status      MilestoneStatus
	Position    int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Task is a concrete action the user does.
type Task struct {
	ID            int64
	ProjectID     int64
	MilestoneID   *int64
	ParentTaskID  *int64
	Title         string
	Notes         string
	Status        TaskStatus
	Size          *TaskSize
	Energy        *Energy
	Position      int64
	OriginEntryID *int64
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// TaskDependency says TaskID can't start until BlockedByTaskID is done.
type TaskDependency struct {
	ID              int64
	TaskID          int64
	BlockedByTaskID int64
	CreatedAt       time.Time
}

// Entry is a diary entry. Entries are immutable once saved.
type Entry struct {
	ID                int64
	Body              string
	ProjectID         *int64 // nil: the extractor picks the project
	ReplyToQuestionID *int64
	CreatedAt         time.Time
}

// Observation is a typed fragment of an entry. SpanStart and SpanEnd locate it
// in the entry body; the extraction validator checks them against the body.
type Observation struct {
	ID              int64
	EntryID         int64
	ExtractionRunID int64
	Kind            ObservationKind
	Summary         string
	SpanStart       int64
	SpanEnd         int64
	CreatedAt       time.Time
}

// LatestRun returns the observations from the newest run that produced any,
// in their order. A reprocessed entry has observations from several runs;
// only the latest describes it. A run with no observations, such as a failed
// one, doesn't hide an earlier run's.
func LatestRun(obs []Observation) []Observation {
	var latest int64
	for _, o := range obs {
		latest = max(latest, o.ExtractionRunID)
	}
	out := []Observation{}
	for _, o := range obs {
		if o.ExtractionRunID == latest {
			out = append(out, o)
		}
	}
	return out
}

// ObservationLink ties an observation to a project element. TargetID points
// into the table named by TargetType.
type ObservationLink struct {
	ID            int64
	ObservationID int64
	TargetType    TargetType
	TargetID      int64
	Confidence    float64
	CreatedAt     time.Time
}

// PendingObservationLink ties an observation to the entity a change op will
// create ("$new:N" in extractor output). Applying the op turns it into an
// ObservationLink; rejecting the op drops it.
type PendingObservationLink struct {
	ID            int64
	ObservationID int64
	ChangeOpID    int64
	TargetType    TargetType
	Confidence    float64
	CreatedAt     time.Time
}

// Blocker is something preventing progress on a project or task.
type Blocker struct {
	ID              int64
	ProjectID       int64
	TaskID          *int64
	Description     string
	Status          BlockerStatus
	OriginEntryID   *int64
	ResolvedEntryID *int64
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Idea is a parking-lot item: a new idea or a project spark.
type Idea struct {
	ID            int64
	ProjectID     *int64
	Title         string
	Body          string
	Status        IdeaStatus
	OriginEntryID *int64
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// FollowUpQuestion is a question the system asks after an entry.
type FollowUpQuestion struct {
	ID              int64
	EntryID         int64
	ExtractionRunID int64
	Text            string
	Reason          string
	Status          QuestionStatus
	AnswerEntryID   *int64
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// ExtractionRun is one extractor pass over an entry. It doubles as the
// worker's job queue.
type ExtractionRun struct {
	ID            int64
	EntryID       int64
	Provider      string
	Model         string
	PromptVersion string
	InputSnapshot json.RawMessage // nil until the snapshot is built
	RawOutput     json.RawMessage // nil until the extractor returns
	Status        ExtractionRunStatus
	Error         *string
	LatencyMS     *int64
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Changeset groups the ops proposed by one extraction run or one manual edit.
// ExtractionRunID is set exactly when Source is SourceExtraction. A changeset
// split off another when it was applied (ADR 0003) has ParentChangesetID set
// and the parent's source and run.
type Changeset struct {
	ID                int64
	Source            ChangesetSource
	ExtractionRunID   *int64
	ParentChangesetID *int64
	Status            ChangesetStatus
	CreatedAt         time.Time
	UpdatedAt         time.Time
	ResolvedAt        *time.Time
}

// ChangeOp is one proposed mutation inside a changeset. Payload is the op as
// proposed; AppliedPayload is what was actually applied, set when the user
// edited it. Payload shapes per OpType are defined by the changeset package.
type ChangeOp struct {
	ID             int64
	ChangesetID    int64
	Position       int64
	OpType         OpType
	Payload        json.RawMessage
	Rationale      *string
	Confidence     *float64
	Status         ChangeOpStatus
	AppliedPayload json.RawMessage
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Briefing is the cached "what's next" for a project.
type Briefing struct {
	ID         int64
	ProjectID  int64
	NextTaskID *int64
	NextReason string
	// Recap is where the user left off: a few recent entries about the
	// project, newest first.
	Recap       []RecapEntry
	Source      BriefingSource
	GeneratedAt time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// RecapEntry is one entry in a briefing's recap, as a few short points.
// CreatedAt is the entry's, read from it; it is ignored on write.
type RecapEntry struct {
	EntryID   int64
	CreatedAt time.Time
	Points    []RecapPoint
}

// RecapPoint is one thing an entry said, with the kind of observation it is.
type RecapPoint struct {
	Kind ObservationKind
	Text string
}
