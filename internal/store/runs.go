package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ogredforman/waypoint/internal/db/dbgen"
	"github.com/ogredforman/waypoint/internal/domain"
)

// Extraction runs double as the worker's job queue: a pending run is waiting,
// a running one is being processed.

func runFromRow(r dbgen.ExtractionRun) domain.ExtractionRun {
	run := domain.ExtractionRun{
		ID:            r.ID,
		EntryID:       r.EntryID,
		Provider:      r.Provider,
		Model:         r.Model,
		PromptVersion: r.PromptVersion,
		Status:        domain.ExtractionRunStatus(r.Status),
		Error:         r.Error,
		LatencyMS:     r.LatencyMs,
		CreatedAt:     fromMS(r.CreatedAt),
		UpdatedAt:     fromMS(r.UpdatedAt),
	}
	if r.InputSnapshot != nil {
		run.InputSnapshot = json.RawMessage(*r.InputSnapshot)
	}
	if r.RawOutput != nil {
		run.RawOutput = json.RawMessage(*r.RawOutput)
	}
	return run
}

func (r reader) GetExtractionRun(ctx context.Context, id int64) (domain.ExtractionRun, error) {
	row, err := r.q.GetExtractionRun(ctx, id)
	if err != nil {
		return domain.ExtractionRun{}, fmt.Errorf("get extraction run %d: %w", id, notFound(err))
	}
	return runFromRow(row), nil
}

// ListExtractionRunsByEntry returns the entry's runs, oldest first.
func (r reader) ListExtractionRunsByEntry(ctx context.Context, entryID int64) ([]domain.ExtractionRun, error) {
	rows, err := r.q.ListExtractionRunsByEntry(ctx, entryID)
	if err != nil {
		return nil, fmt.Errorf("list runs of entry %d: %w", entryID, err)
	}
	out := make([]domain.ExtractionRun, len(rows))
	for i, row := range rows {
		out[i] = runFromRow(row)
	}
	return out, nil
}

// CountActiveExtractionRuns returns how many of the entry's runs are pending
// or running.
func (r reader) CountActiveExtractionRuns(ctx context.Context, entryID int64) (int, error) {
	n, err := r.q.CountActiveExtractionRunsByEntry(ctx, entryID)
	if err != nil {
		return 0, fmt.Errorf("count active runs of entry %d: %w", entryID, err)
	}
	return int(n), nil
}

// CountRunningExtractionRuns returns how many of the entry's runs are
// running (claimed by the worker).
func (r reader) CountRunningExtractionRuns(ctx context.Context, entryID int64) (int, error) {
	n, err := r.q.CountRunningExtractionRunsByEntry(ctx, entryID)
	if err != nil {
		return 0, fmt.Errorf("count running runs of entry %d: %w", entryID, err)
	}
	return int(n), nil
}

// CreateExtractionRun inserts r as a pending run, ignoring its ID, status,
// results, and timestamps.
func (tx *Tx) CreateExtractionRun(ctx context.Context, r domain.ExtractionRun) (domain.ExtractionRun, error) {
	r.Status = domain.RunPending
	r.InputSnapshot, r.RawOutput, r.Error, r.LatencyMS = nil, nil, nil, nil
	if err := r.Validate(); err != nil {
		return domain.ExtractionRun{}, fmt.Errorf("create extraction run: %w", err)
	}
	now := tx.nowMS()
	row, err := tx.q.CreateExtractionRun(ctx, dbgen.CreateExtractionRunParams{
		EntryID:       r.EntryID,
		Provider:      r.Provider,
		Model:         r.Model,
		PromptVersion: r.PromptVersion,
		Status:        string(domain.RunPending),
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	if err != nil {
		return domain.ExtractionRun{}, fmt.Errorf("create extraction run: %w", err)
	}
	return runFromRow(row), nil
}

// ClaimNextExtractionRun marks the oldest pending run running and returns it.
// It returns ErrNotFound if no run is pending.
func (tx *Tx) ClaimNextExtractionRun(ctx context.Context) (domain.ExtractionRun, error) {
	row, err := tx.q.ClaimNextExtractionRun(ctx, tx.nowMS())
	if err != nil {
		return domain.ExtractionRun{}, fmt.Errorf("claim extraction run: %w", notFound(err))
	}
	return runFromRow(row), nil
}

// ResetRunningExtractionRuns puts every running run back to pending and
// returns how many there were. Call it at startup, before any run is claimed.
func (tx *Tx) ResetRunningExtractionRuns(ctx context.Context) (int, error) {
	n, err := tx.q.ResetRunningExtractionRuns(ctx, tx.nowMS())
	if err != nil {
		return 0, fmt.Errorf("reset running extraction runs: %w", err)
	}
	return int(n), nil
}

// SetExtractionRunSnapshot stores the snapshot given to run id's extractor.
func (tx *Tx) SetExtractionRunSnapshot(ctx context.Context, id int64, snapshot json.RawMessage) (domain.ExtractionRun, error) {
	if !json.Valid(snapshot) {
		return domain.ExtractionRun{}, fmt.Errorf("set snapshot of run %d: not valid JSON", id)
	}
	row, err := tx.q.SetExtractionRunSnapshot(ctx, dbgen.SetExtractionRunSnapshotParams{
		InputSnapshot: rawPtrToString(snapshot),
		UpdatedAt:     tx.nowMS(),
		ID:            id,
	})
	if err != nil {
		return domain.ExtractionRun{}, fmt.Errorf("set snapshot of run %d: %w", id, notFound(err))
	}
	return runFromRow(row), nil
}

// RunOutcome is how a running extraction run ended.
type RunOutcome struct {
	Status    domain.ExtractionRunStatus // succeeded or failed
	RawOutput json.RawMessage            // nil if the extractor returned nothing
	Error     *string                    // set exactly when failed
	LatencyMS *int64
}

// FinishExtractionRun records how running run id ended. It returns
// ErrNotFound if the run doesn't exist or isn't running.
func (tx *Tx) FinishExtractionRun(ctx context.Context, id int64, o RunOutcome) (domain.ExtractionRun, error) {
	if o.Status != domain.RunSucceeded && o.Status != domain.RunFailed {
		return domain.ExtractionRun{}, fmt.Errorf("finish run %d: %q is not a final status", id, o.Status)
	}
	if (o.Status == domain.RunFailed) != (o.Error != nil) {
		return domain.ExtractionRun{}, fmt.Errorf("finish run %d: error is set exactly when the run failed", id)
	}
	if o.RawOutput != nil && !json.Valid(o.RawOutput) {
		return domain.ExtractionRun{}, fmt.Errorf("finish run %d: raw output is not valid JSON", id)
	}
	if o.LatencyMS != nil && *o.LatencyMS < 0 {
		return domain.ExtractionRun{}, fmt.Errorf("finish run %d: latency must not be negative", id)
	}
	row, err := tx.q.FinishExtractionRun(ctx, dbgen.FinishExtractionRunParams{
		Status:    string(o.Status),
		RawOutput: rawPtrToString(o.RawOutput),
		Error:     o.Error,
		LatencyMs: o.LatencyMS,
		UpdatedAt: tx.nowMS(),
		ID:        id,
	})
	if err != nil {
		return domain.ExtractionRun{}, fmt.Errorf("finish run %d: %w", id, notFound(err))
	}
	return runFromRow(row), nil
}
