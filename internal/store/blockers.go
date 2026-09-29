package store

import (
	"context"
	"fmt"

	"github.com/ogredforman/waypoint/internal/db/dbgen"
	"github.com/ogredforman/waypoint/internal/domain"
)

func blockerFromRow(r dbgen.Blocker) domain.Blocker {
	return domain.Blocker{
		ID:              r.ID,
		ProjectID:       r.ProjectID,
		TaskID:          r.TaskID,
		Description:     r.Description,
		Status:          domain.BlockerStatus(r.Status),
		OriginEntryID:   r.OriginEntryID,
		ResolvedEntryID: r.ResolvedEntryID,
		CreatedAt:       fromMS(r.CreatedAt),
		UpdatedAt:       fromMS(r.UpdatedAt),
	}
}

func (r reader) GetBlocker(ctx context.Context, id int64) (domain.Blocker, error) {
	row, err := r.q.GetBlocker(ctx, id)
	if err != nil {
		return domain.Blocker{}, fmt.Errorf("get blocker %d: %w", id, notFound(err))
	}
	return blockerFromRow(row), nil
}

// ListBlockersByProject returns the project's blockers, open and resolved,
// oldest first.
func (r reader) ListBlockersByProject(ctx context.Context, projectID int64) ([]domain.Blocker, error) {
	rows, err := r.q.ListBlockersByProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list blockers of project %d: %w", projectID, err)
	}
	out := make([]domain.Blocker, len(rows))
	for i, row := range rows {
		out[i] = blockerFromRow(row)
	}
	return out, nil
}

// CreateBlocker inserts b, ignoring its ID and timestamps.
func (tx *Tx) CreateBlocker(ctx context.Context, b domain.Blocker) (domain.Blocker, error) {
	if err := b.Validate(); err != nil {
		return domain.Blocker{}, fmt.Errorf("create blocker: %w", err)
	}
	now := tx.nowMS()
	row, err := tx.q.CreateBlocker(ctx, dbgen.CreateBlockerParams{
		ProjectID:       b.ProjectID,
		TaskID:          b.TaskID,
		Description:     b.Description,
		Status:          string(b.Status),
		OriginEntryID:   b.OriginEntryID,
		ResolvedEntryID: b.ResolvedEntryID,
		CreatedAt:       now,
		UpdatedAt:       now,
	})
	if err != nil {
		return domain.Blocker{}, fmt.Errorf("create blocker: %w", err)
	}
	return blockerFromRow(row), nil
}

// UpdateBlocker overwrites the editable fields of blocker b.ID. ProjectID and
// OriginEntryID are fixed at creation and ignored here.
func (tx *Tx) UpdateBlocker(ctx context.Context, b domain.Blocker) (domain.Blocker, error) {
	if err := b.Validate(); err != nil {
		return domain.Blocker{}, fmt.Errorf("update blocker %d: %w", b.ID, err)
	}
	row, err := tx.q.UpdateBlocker(ctx, dbgen.UpdateBlockerParams{
		TaskID:          b.TaskID,
		Description:     b.Description,
		Status:          string(b.Status),
		ResolvedEntryID: b.ResolvedEntryID,
		UpdatedAt:       tx.nowMS(),
		ID:              b.ID,
	})
	if err != nil {
		return domain.Blocker{}, fmt.Errorf("update blocker %d: %w", b.ID, notFound(err))
	}
	return blockerFromRow(row), nil
}

// DeleteBlocker removes a blocker and the observation links to it, and returns
// the blocker as it was.
func (tx *Tx) DeleteBlocker(ctx context.Context, id int64) (domain.Blocker, error) {
	if err := tx.deleteLinksTo(ctx, domain.TargetBlocker, id); err != nil {
		return domain.Blocker{}, fmt.Errorf("delete blocker %d: delete observation links: %w", id, err)
	}
	row, err := tx.q.DeleteBlocker(ctx, id)
	if err != nil {
		return domain.Blocker{}, fmt.Errorf("delete blocker %d: %w", id, notFound(err))
	}
	return blockerFromRow(row), nil
}
