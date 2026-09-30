package store

import (
	"context"
	"fmt"

	"github.com/ogredforman/waypoint/internal/db/dbgen"
	"github.com/ogredforman/waypoint/internal/domain"
)

func briefingFromRow(r dbgen.Briefing) domain.Briefing {
	return domain.Briefing{
		ID:          r.ID,
		ProjectID:   r.ProjectID,
		NextTaskID:  r.NextTaskID,
		NextReason:  r.NextReason,
		Recap:       r.Recap,
		Source:      domain.BriefingSource(r.Source),
		GeneratedAt: fromMS(r.GeneratedAt),
		CreatedAt:   fromMS(r.CreatedAt),
		UpdatedAt:   fromMS(r.UpdatedAt),
	}
}

// GetBriefing returns the project's cached briefing, or ErrNotFound if none
// has been generated yet.
func (r reader) GetBriefing(ctx context.Context, projectID int64) (domain.Briefing, error) {
	row, err := r.q.GetBriefingByProject(ctx, projectID)
	if err != nil {
		return domain.Briefing{}, fmt.Errorf("get briefing of project %d: %w", projectID, notFound(err))
	}
	return briefingFromRow(row), nil
}

// UpsertBriefing stores b as its project's briefing, replacing any earlier
// one. Its ID, CreatedAt, and UpdatedAt are ignored.
func (tx *Tx) UpsertBriefing(ctx context.Context, b domain.Briefing) (domain.Briefing, error) {
	if err := b.Validate(); err != nil {
		return domain.Briefing{}, fmt.Errorf("upsert briefing: %w", err)
	}
	row, err := tx.q.UpsertBriefing(ctx, dbgen.UpsertBriefingParams{
		ProjectID:   b.ProjectID,
		NextTaskID:  b.NextTaskID,
		NextReason:  b.NextReason,
		Recap:       b.Recap,
		Source:      string(b.Source),
		GeneratedAt: b.GeneratedAt.UnixMilli(),
		Now:         tx.nowMS(),
	})
	if err != nil {
		return domain.Briefing{}, fmt.Errorf("upsert briefing of project %d: %w", b.ProjectID, err)
	}
	return briefingFromRow(row), nil
}
