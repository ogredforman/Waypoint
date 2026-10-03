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
		Recap:       []domain.RecapEntry{},
		Source:      domain.BriefingSource(r.Source),
		GeneratedAt: fromMS(r.GeneratedAt),
		CreatedAt:   fromMS(r.CreatedAt),
		UpdatedAt:   fromMS(r.UpdatedAt),
	}
}

// GetBriefing returns the project's cached briefing with its recap, or
// ErrNotFound if none has been generated yet.
func (r reader) GetBriefing(ctx context.Context, projectID int64) (domain.Briefing, error) {
	row, err := r.q.GetBriefingByProject(ctx, projectID)
	if err != nil {
		return domain.Briefing{}, fmt.Errorf("get briefing of project %d: %w", projectID, notFound(err))
	}
	b := briefingFromRow(row)
	if b.Recap, err = r.briefingRecap(ctx, b.ID); err != nil {
		return domain.Briefing{}, fmt.Errorf("get briefing of project %d: %w", projectID, err)
	}
	return b, nil
}

// briefingRecap reads a briefing's recap points back into entries: the
// points were written entry by entry, so an entry's points are consecutive.
func (r reader) briefingRecap(ctx context.Context, briefingID int64) ([]domain.RecapEntry, error) {
	rows, err := r.q.ListBriefingRecap(ctx, briefingID)
	if err != nil {
		return nil, fmt.Errorf("list recap: %w", err)
	}
	out := []domain.RecapEntry{}
	for _, row := range rows {
		if n := len(out); n == 0 || out[n-1].EntryID != row.EntryID {
			out = append(out, domain.RecapEntry{EntryID: row.EntryID, CreatedAt: fromMS(row.EntryCreatedAt)})
		}
		last := &out[len(out)-1]
		last.Points = append(last.Points, domain.RecapPoint{Kind: domain.ObservationKind(row.Kind), Text: row.Text})
	}
	return out, nil
}

// UpsertBriefing stores b as its project's briefing, replacing any earlier
// one and its recap. Its ID, CreatedAt, UpdatedAt, and the recap entries'
// CreatedAt are ignored.
func (tx *Tx) UpsertBriefing(ctx context.Context, b domain.Briefing) (domain.Briefing, error) {
	if err := b.Validate(); err != nil {
		return domain.Briefing{}, fmt.Errorf("upsert briefing: %w", err)
	}
	row, err := tx.q.UpsertBriefing(ctx, dbgen.UpsertBriefingParams{
		ProjectID:   b.ProjectID,
		NextTaskID:  b.NextTaskID,
		NextReason:  b.NextReason,
		Source:      string(b.Source),
		GeneratedAt: b.GeneratedAt.UnixMilli(),
		Now:         tx.nowMS(),
	})
	if err != nil {
		return domain.Briefing{}, fmt.Errorf("upsert briefing of project %d: %w", b.ProjectID, err)
	}
	if err := tx.q.DeleteBriefingRecap(ctx, row.ID); err != nil {
		return domain.Briefing{}, fmt.Errorf("upsert briefing of project %d: clear recap: %w", b.ProjectID, err)
	}
	var pos int64
	for _, e := range b.Recap {
		for _, p := range e.Points {
			pos++
			err := tx.q.CreateBriefingRecapPoint(ctx, dbgen.CreateBriefingRecapPointParams{
				BriefingID: row.ID, EntryID: e.EntryID, Position: pos, Kind: string(p.Kind), Text: p.Text,
			})
			if err != nil {
				return domain.Briefing{}, fmt.Errorf("upsert briefing of project %d: recap: %w", b.ProjectID, err)
			}
		}
	}
	out := briefingFromRow(row)
	if out.Recap, err = tx.briefingRecap(ctx, row.ID); err != nil {
		return domain.Briefing{}, fmt.Errorf("upsert briefing of project %d: %w", b.ProjectID, err)
	}
	return out, nil
}
