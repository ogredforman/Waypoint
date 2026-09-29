package store

import (
	"context"
	"fmt"

	"github.com/ogredforman/waypoint/internal/db/dbgen"
	"github.com/ogredforman/waypoint/internal/domain"
)

// Entries are immutable: there is deliberately no update or delete here.

func entryFromRow(r dbgen.Entry) domain.Entry {
	return domain.Entry{
		ID:                r.ID,
		Body:              r.Body,
		ProjectID:         r.ProjectID,
		ReplyToQuestionID: r.ReplyToQuestionID,
		CreatedAt:         fromMS(r.CreatedAt),
	}
}

func entriesFromRows(rows []dbgen.Entry) []domain.Entry {
	out := make([]domain.Entry, len(rows))
	for i, row := range rows {
		out[i] = entryFromRow(row)
	}
	return out
}

func (r reader) GetEntry(ctx context.Context, id int64) (domain.Entry, error) {
	row, err := r.q.GetEntry(ctx, id)
	if err != nil {
		return domain.Entry{}, fmt.Errorf("get entry %d: %w", id, notFound(err))
	}
	return entryFromRow(row), nil
}

// ListRecentEntries returns up to limit entries, newest first.
func (r reader) ListRecentEntries(ctx context.Context, limit int) ([]domain.Entry, error) {
	if err := checkLimit(limit); err != nil {
		return nil, fmt.Errorf("list entries: %w", err)
	}
	rows, err := r.q.ListRecentEntries(ctx, int64(limit))
	if err != nil {
		return nil, fmt.Errorf("list entries: %w", err)
	}
	return entriesFromRows(rows), nil
}

// ListRecentEntriesByProject returns up to limit entries scoped to the
// project, newest first. Unscoped entries are not included.
func (r reader) ListRecentEntriesByProject(ctx context.Context, projectID int64, limit int) ([]domain.Entry, error) {
	if err := checkLimit(limit); err != nil {
		return nil, fmt.Errorf("list entries of project %d: %w", projectID, err)
	}
	rows, err := r.q.ListRecentEntriesByProject(ctx, dbgen.ListRecentEntriesByProjectParams{
		ProjectID: &projectID,
		Limit:     int64(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("list entries of project %d: %w", projectID, err)
	}
	return entriesFromRows(rows), nil
}

// CreateEntry inserts e, ignoring its ID and CreatedAt.
func (tx *Tx) CreateEntry(ctx context.Context, e domain.Entry) (domain.Entry, error) {
	if err := e.Validate(); err != nil {
		return domain.Entry{}, fmt.Errorf("create entry: %w", err)
	}
	row, err := tx.q.CreateEntry(ctx, dbgen.CreateEntryParams{
		Body:              e.Body,
		ProjectID:         e.ProjectID,
		ReplyToQuestionID: e.ReplyToQuestionID,
		CreatedAt:         tx.nowMS(),
	})
	if err != nil {
		return domain.Entry{}, fmt.Errorf("create entry: %w", err)
	}
	return entryFromRow(row), nil
}
