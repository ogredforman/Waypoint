// Package apitypes holds the wire DTOs for the HTTP API and the SSE event
// payloads. It is the source for the generated TypeScript types (ADR 0001),
// so it holds data shapes only.
package apitypes

// EntryProcessedEvent is the data of an "entry.processed" event: an
// extraction run succeeded and the entry's observations and questions are
// stored.
type EntryProcessedEvent struct {
	EntryID int64 `json:"entry_id"`
	RunID   int64 `json:"run_id"`
}

// ChangesetReadyEvent is the data of a "changeset.ready" event: a run
// proposed ops, stored as a pending changeset.
type ChangesetReadyEvent struct {
	EntryID     int64 `json:"entry_id"`
	RunID       int64 `json:"run_id"`
	ChangesetID int64 `json:"changeset_id"`
}

// RunFailedEvent is the data of a "run.failed" event: an extraction run
// failed and the entry can be reprocessed.
type RunFailedEvent struct {
	EntryID int64 `json:"entry_id"`
	RunID   int64 `json:"run_id"`
}

// BriefingUpdatedEvent is the data of a "briefing.updated" event: a
// project's briefing was regenerated.
type BriefingUpdatedEvent struct {
	ProjectID int64 `json:"project_id"`
}

// ChangesetAppliedEvent is the data of a "changeset.applied" event: a
// changeset, manual or from an entry, was applied. ProjectIDs lists the
// projects whose own row, tasks, or blockers it changed; ideas don't count.
type ChangesetAppliedEvent struct {
	ChangesetID int64   `json:"changeset_id"`
	ProjectIDs  []int64 `json:"project_ids"`
}
