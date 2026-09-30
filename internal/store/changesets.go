package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ogredforman/waypoint/internal/db/dbgen"
	"github.com/ogredforman/waypoint/internal/domain"
)

// Changesets and their ops are stored here; applying them is
// internal/changeset's job. Payloads are stored as given: callers validate
// them with changeset.ValidateOps first.

func changesetFromRow(r dbgen.Changeset) domain.Changeset {
	cs := domain.Changeset{
		ID:                r.ID,
		Source:            domain.ChangesetSource(r.Source),
		ExtractionRunID:   r.ExtractionRunID,
		ParentChangesetID: r.ParentChangesetID,
		Status:            domain.ChangesetStatus(r.Status),
		CreatedAt:         fromMS(r.CreatedAt),
		UpdatedAt:         fromMS(r.UpdatedAt),
	}
	if r.ResolvedAt != nil {
		t := fromMS(*r.ResolvedAt)
		cs.ResolvedAt = &t
	}
	return cs
}

func changeOpFromRow(r dbgen.ChangeOp) domain.ChangeOp {
	op := domain.ChangeOp{
		ID:          r.ID,
		ChangesetID: r.ChangesetID,
		Position:    r.Position,
		OpType:      domain.OpType(r.OpType),
		Payload:     json.RawMessage(r.Payload),
		Rationale:   r.Rationale,
		Confidence:  r.Confidence,
		Status:      domain.ChangeOpStatus(r.Status),
		CreatedAt:   fromMS(r.CreatedAt),
		UpdatedAt:   fromMS(r.UpdatedAt),
	}
	if r.AppliedPayload != nil {
		op.AppliedPayload = json.RawMessage(*r.AppliedPayload)
	}
	return op
}

func rawPtrToString(b json.RawMessage) *string {
	if b == nil {
		return nil
	}
	s := string(b)
	return &s
}

func (r reader) GetChangeset(ctx context.Context, id int64) (domain.Changeset, error) {
	row, err := r.q.GetChangeset(ctx, id)
	if err != nil {
		return domain.Changeset{}, fmt.Errorf("get changeset %d: %w", id, notFound(err))
	}
	return changesetFromRow(row), nil
}

// ListChangeOps returns the changeset's ops in position order.
func (r reader) ListChangeOps(ctx context.Context, changesetID int64) ([]domain.ChangeOp, error) {
	rows, err := r.q.ListChangeOpsByChangeset(ctx, changesetID)
	if err != nil {
		return nil, fmt.Errorf("list ops of changeset %d: %w", changesetID, err)
	}
	out := make([]domain.ChangeOp, len(rows))
	for i, row := range rows {
		out[i] = changeOpFromRow(row)
	}
	return out, nil
}

// ListChangesetsByEntry returns the extraction changesets of the entry, from
// all of its runs and including split-offs, oldest first.
func (r reader) ListChangesetsByEntry(ctx context.Context, entryID int64) ([]domain.Changeset, error) {
	rows, err := r.q.ListChangesetsByEntry(ctx, entryID)
	if err != nil {
		return nil, fmt.Errorf("list changesets of entry %d: %w", entryID, err)
	}
	out := make([]domain.Changeset, len(rows))
	for i, row := range rows {
		out[i] = changesetFromRow(row)
	}
	return out, nil
}

// ChangesetEntryID returns the entry an extraction changeset came from, or
// nil for a manual one.
func (r reader) ChangesetEntryID(ctx context.Context, cs domain.Changeset) (*int64, error) {
	if cs.ExtractionRunID == nil {
		return nil, nil
	}
	id, err := r.q.GetExtractionRunEntryID(ctx, *cs.ExtractionRunID)
	if err != nil {
		return nil, fmt.Errorf("entry of changeset %d: %w", cs.ID, notFound(err))
	}
	return &id, nil
}

// CreateChangeset inserts cs and its ops, ignoring IDs, ChangesetID, and
// timestamps. It returns both as stored.
func (tx *Tx) CreateChangeset(ctx context.Context, cs domain.Changeset, ops []domain.ChangeOp) (domain.Changeset, []domain.ChangeOp, error) {
	if err := cs.Validate(); err != nil {
		return domain.Changeset{}, nil, fmt.Errorf("create changeset: %w", err)
	}
	now := tx.nowMS()
	row, err := tx.q.CreateChangeset(ctx, dbgen.CreateChangesetParams{
		Source:            string(cs.Source),
		ExtractionRunID:   cs.ExtractionRunID,
		ParentChangesetID: cs.ParentChangesetID,
		Status:            string(cs.Status),
		CreatedAt:         now,
		UpdatedAt:         now,
	})
	if err != nil {
		return domain.Changeset{}, nil, fmt.Errorf("create changeset: %w", err)
	}
	created := changesetFromRow(row)

	out := make([]domain.ChangeOp, len(ops))
	for i, op := range ops {
		op.ChangesetID = created.ID
		if err := op.Validate(); err != nil {
			return domain.Changeset{}, nil, fmt.Errorf("create changeset: op at position %d: %w", op.Position, err)
		}
		opRow, err := tx.q.CreateChangeOp(ctx, dbgen.CreateChangeOpParams{
			ChangesetID:    created.ID,
			Position:       op.Position,
			OpType:         string(op.OpType),
			Payload:        string(op.Payload),
			Rationale:      op.Rationale,
			Confidence:     op.Confidence,
			Status:         string(op.Status),
			AppliedPayload: rawPtrToString(op.AppliedPayload),
			CreatedAt:      now,
			UpdatedAt:      now,
		})
		if err != nil {
			return domain.Changeset{}, nil, fmt.Errorf("create changeset: op at position %d: %w", op.Position, err)
		}
		out[i] = changeOpFromRow(opRow)
	}
	return created, out, nil
}

// ResolveChangeset sets changeset id's final status and resolved_at.
func (tx *Tx) ResolveChangeset(ctx context.Context, id int64, status domain.ChangesetStatus) (domain.Changeset, error) {
	if !status.Valid() || status == domain.ChangesetPending {
		return domain.Changeset{}, fmt.Errorf("resolve changeset %d: %q is not a final status", id, status)
	}
	now := tx.nowMS()
	row, err := tx.q.ResolveChangeset(ctx, dbgen.ResolveChangesetParams{
		Status:     string(status),
		ResolvedAt: &now,
		UpdatedAt:  now,
		ID:         id,
	})
	if err != nil {
		return domain.Changeset{}, fmt.Errorf("resolve changeset %d: %w", id, notFound(err))
	}
	return changesetFromRow(row), nil
}

// SetChangeOpResult records what happened to op id: its status and, for an
// edited or delete op, the applied payload.
func (tx *Tx) SetChangeOpResult(ctx context.Context, id int64, status domain.ChangeOpStatus, applied json.RawMessage) (domain.ChangeOp, error) {
	if !status.Valid() {
		return domain.ChangeOp{}, fmt.Errorf("set result of op %d: status is not an allowed value", id)
	}
	if applied != nil && !json.Valid(applied) {
		return domain.ChangeOp{}, fmt.Errorf("set result of op %d: applied payload is not valid JSON", id)
	}
	row, err := tx.q.SetChangeOpResult(ctx, dbgen.SetChangeOpResultParams{
		Status:         string(status),
		AppliedPayload: rawPtrToString(applied),
		UpdatedAt:      tx.nowMS(),
		ID:             id,
	})
	if err != nil {
		return domain.ChangeOp{}, fmt.Errorf("set result of op %d: %w", id, notFound(err))
	}
	return changeOpFromRow(row), nil
}

// MoveChangeOp moves op id to another changeset, keeping its position, and
// replaces its payload.
func (tx *Tx) MoveChangeOp(ctx context.Context, id, toChangesetID int64, payload json.RawMessage) (domain.ChangeOp, error) {
	if !json.Valid(payload) {
		return domain.ChangeOp{}, fmt.Errorf("move op %d: payload is not valid JSON", id)
	}
	row, err := tx.q.MoveChangeOp(ctx, dbgen.MoveChangeOpParams{
		ChangesetID: toChangesetID,
		Payload:     string(payload),
		UpdatedAt:   tx.nowMS(),
		ID:          id,
	})
	if err != nil {
		return domain.ChangeOp{}, fmt.Errorf("move op %d: %w", id, notFound(err))
	}
	return changeOpFromRow(row), nil
}
