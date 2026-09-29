package store

import (
	"context"
	"fmt"

	"github.com/ogredforman/waypoint/internal/db/dbgen"
	"github.com/ogredforman/waypoint/internal/domain"
)

func ideaFromRow(r dbgen.Idea) domain.Idea {
	return domain.Idea{
		ID:            r.ID,
		ProjectID:     r.ProjectID,
		Title:         r.Title,
		Body:          r.Body,
		Status:        domain.IdeaStatus(r.Status),
		OriginEntryID: r.OriginEntryID,
		CreatedAt:     fromMS(r.CreatedAt),
		UpdatedAt:     fromMS(r.UpdatedAt),
	}
}

func ideasFromRows(rows []dbgen.Idea) []domain.Idea {
	out := make([]domain.Idea, len(rows))
	for i, row := range rows {
		out[i] = ideaFromRow(row)
	}
	return out
}

func (r reader) GetIdea(ctx context.Context, id int64) (domain.Idea, error) {
	row, err := r.q.GetIdea(ctx, id)
	if err != nil {
		return domain.Idea{}, fmt.Errorf("get idea %d: %w", id, notFound(err))
	}
	return ideaFromRow(row), nil
}

// ListIdeasByStatus returns every idea with the given status, oldest first.
func (r reader) ListIdeasByStatus(ctx context.Context, status domain.IdeaStatus) ([]domain.Idea, error) {
	rows, err := r.q.ListIdeasByStatus(ctx, string(status))
	if err != nil {
		return nil, fmt.Errorf("list %s ideas: %w", status, err)
	}
	return ideasFromRows(rows), nil
}

// ListIdeasByProject returns the project's ideas in any status, oldest first.
func (r reader) ListIdeasByProject(ctx context.Context, projectID int64) ([]domain.Idea, error) {
	rows, err := r.q.ListIdeasByProject(ctx, &projectID)
	if err != nil {
		return nil, fmt.Errorf("list ideas of project %d: %w", projectID, err)
	}
	return ideasFromRows(rows), nil
}

// CreateIdea inserts i, ignoring its ID and timestamps.
func (tx *Tx) CreateIdea(ctx context.Context, i domain.Idea) (domain.Idea, error) {
	if err := i.Validate(); err != nil {
		return domain.Idea{}, fmt.Errorf("create idea: %w", err)
	}
	now := tx.nowMS()
	row, err := tx.q.CreateIdea(ctx, dbgen.CreateIdeaParams{
		ProjectID:     i.ProjectID,
		Title:         i.Title,
		Body:          i.Body,
		Status:        string(i.Status),
		OriginEntryID: i.OriginEntryID,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	if err != nil {
		return domain.Idea{}, fmt.Errorf("create idea: %w", err)
	}
	return ideaFromRow(row), nil
}

// UpdateIdea overwrites the editable fields of idea i.ID. OriginEntryID is
// fixed at creation and ignored here.
func (tx *Tx) UpdateIdea(ctx context.Context, i domain.Idea) (domain.Idea, error) {
	if err := i.Validate(); err != nil {
		return domain.Idea{}, fmt.Errorf("update idea %d: %w", i.ID, err)
	}
	row, err := tx.q.UpdateIdea(ctx, dbgen.UpdateIdeaParams{
		ProjectID: i.ProjectID,
		Title:     i.Title,
		Body:      i.Body,
		Status:    string(i.Status),
		UpdatedAt: tx.nowMS(),
		ID:        i.ID,
	})
	if err != nil {
		return domain.Idea{}, fmt.Errorf("update idea %d: %w", i.ID, notFound(err))
	}
	return ideaFromRow(row), nil
}

// DeleteIdea removes an idea and the observation links to it, and returns the
// idea as it was.
func (tx *Tx) DeleteIdea(ctx context.Context, id int64) (domain.Idea, error) {
	if err := tx.deleteLinksTo(ctx, domain.TargetIdea, id); err != nil {
		return domain.Idea{}, fmt.Errorf("delete idea %d: delete observation links: %w", id, err)
	}
	row, err := tx.q.DeleteIdea(ctx, id)
	if err != nil {
		return domain.Idea{}, fmt.Errorf("delete idea %d: %w", id, notFound(err))
	}
	return ideaFromRow(row), nil
}
