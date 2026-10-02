package store

import (
	"context"
	"fmt"

	"github.com/ogredforman/waypoint/internal/db/dbgen"
	"github.com/ogredforman/waypoint/internal/domain"
)

func observationFromRow(r dbgen.Observation) domain.Observation {
	return domain.Observation{
		ID:              r.ID,
		EntryID:         r.EntryID,
		ExtractionRunID: r.ExtractionRunID,
		Kind:            domain.ObservationKind(r.Kind),
		Summary:         r.Summary,
		SpanStart:       r.SpanStart,
		SpanEnd:         r.SpanEnd,
		CreatedAt:       fromMS(r.CreatedAt),
	}
}

func linkFromRow(r dbgen.ObservationLink) domain.ObservationLink {
	return domain.ObservationLink{
		ID:            r.ID,
		ObservationID: r.ObservationID,
		TargetType:    domain.TargetType(r.TargetType),
		TargetID:      r.TargetID,
		Confidence:    r.Confidence,
		CreatedAt:     fromMS(r.CreatedAt),
	}
}

func pendingLinkFromRow(r dbgen.PendingObservationLink) domain.PendingObservationLink {
	return domain.PendingObservationLink{
		ID:            r.ID,
		ObservationID: r.ObservationID,
		ChangeOpID:    r.ChangeOpID,
		TargetType:    domain.TargetType(r.TargetType),
		Confidence:    r.Confidence,
		CreatedAt:     fromMS(r.CreatedAt),
	}
}

func linksFromRows(rows []dbgen.ObservationLink) []domain.ObservationLink {
	out := make([]domain.ObservationLink, len(rows))
	for i, row := range rows {
		out[i] = linkFromRow(row)
	}
	return out
}

func (r reader) GetObservation(ctx context.Context, id int64) (domain.Observation, error) {
	row, err := r.q.GetObservation(ctx, id)
	if err != nil {
		return domain.Observation{}, fmt.Errorf("get observation %d: %w", id, notFound(err))
	}
	return observationFromRow(row), nil
}

// ListObservationsByEntry returns the entry's observations in text order.
func (r reader) ListObservationsByEntry(ctx context.Context, entryID int64) ([]domain.Observation, error) {
	rows, err := r.q.ListObservationsByEntry(ctx, entryID)
	if err != nil {
		return nil, fmt.Errorf("list observations of entry %d: %w", entryID, err)
	}
	out := make([]domain.Observation, len(rows))
	for i, row := range rows {
		out[i] = observationFromRow(row)
	}
	return out, nil
}

// ListLinksByObservation returns the observation's links in insert order.
func (r reader) ListLinksByObservation(ctx context.Context, observationID int64) ([]domain.ObservationLink, error) {
	rows, err := r.q.ListLinksByObservation(ctx, observationID)
	if err != nil {
		return nil, fmt.Errorf("list links of observation %d: %w", observationID, err)
	}
	return linksFromRows(rows), nil
}

// ListLinksByTarget returns the links pointing at one project element, oldest
// first.
func (r reader) ListLinksByTarget(ctx context.Context, targetType domain.TargetType, targetID int64) ([]domain.ObservationLink, error) {
	rows, err := r.q.ListLinksByTarget(ctx, dbgen.ListLinksByTargetParams{
		TargetType: string(targetType),
		TargetID:   targetID,
	})
	if err != nil {
		return nil, fmt.Errorf("list links to %s %d: %w", targetType, targetID, err)
	}
	return linksFromRows(rows), nil
}

// CreateObservation inserts o, ignoring its ID and CreatedAt. Checking the
// span against the entry body is the extraction validator's job.
func (tx *Tx) CreateObservation(ctx context.Context, o domain.Observation) (domain.Observation, error) {
	if err := o.Validate(); err != nil {
		return domain.Observation{}, fmt.Errorf("create observation: %w", err)
	}
	row, err := tx.q.CreateObservation(ctx, dbgen.CreateObservationParams{
		EntryID:         o.EntryID,
		ExtractionRunID: o.ExtractionRunID,
		Kind:            string(o.Kind),
		Summary:         o.Summary,
		SpanStart:       o.SpanStart,
		SpanEnd:         o.SpanEnd,
		CreatedAt:       tx.nowMS(),
	})
	if err != nil {
		return domain.Observation{}, fmt.Errorf("create observation: %w", err)
	}
	return observationFromRow(row), nil
}

// CreateObservationLink inserts l, ignoring its ID and CreatedAt. The target
// can't be a foreign key, so callers check that it exists.
func (tx *Tx) CreateObservationLink(ctx context.Context, l domain.ObservationLink) (domain.ObservationLink, error) {
	if err := l.Validate(); err != nil {
		return domain.ObservationLink{}, fmt.Errorf("create observation link: %w", err)
	}
	row, err := tx.q.CreateObservationLink(ctx, dbgen.CreateObservationLinkParams{
		ObservationID: l.ObservationID,
		TargetType:    string(l.TargetType),
		TargetID:      l.TargetID,
		Confidence:    l.Confidence,
		CreatedAt:     tx.nowMS(),
	})
	if err != nil {
		return domain.ObservationLink{}, fmt.Errorf("create observation link: %w", err)
	}
	return linkFromRow(row), nil
}

// CreatePendingLink inserts l, ignoring its ID and CreatedAt. The op must
// create an entity of l.TargetType; callers check that.
func (tx *Tx) CreatePendingLink(ctx context.Context, l domain.PendingObservationLink) (domain.PendingObservationLink, error) {
	if err := l.Validate(); err != nil {
		return domain.PendingObservationLink{}, fmt.Errorf("create pending observation link: %w", err)
	}
	row, err := tx.q.CreatePendingLink(ctx, dbgen.CreatePendingLinkParams{
		ObservationID: l.ObservationID,
		ChangeOpID:    l.ChangeOpID,
		TargetType:    string(l.TargetType),
		Confidence:    l.Confidence,
		CreatedAt:     tx.nowMS(),
	})
	if err != nil {
		return domain.PendingObservationLink{}, fmt.Errorf("create pending observation link: %w", err)
	}
	return pendingLinkFromRow(row), nil
}

// ListPendingLinksByOp returns the links waiting on change op opID.
func (r reader) ListPendingLinksByOp(ctx context.Context, opID int64) ([]domain.PendingObservationLink, error) {
	rows, err := r.q.ListPendingLinksByOp(ctx, opID)
	if err != nil {
		return nil, fmt.Errorf("list pending links of op %d: %w", opID, err)
	}
	out := make([]domain.PendingObservationLink, len(rows))
	for i, row := range rows {
		out[i] = pendingLinkFromRow(row)
	}
	return out, nil
}

// ResolvePendingLinks turns the links waiting on change op opID into links to
// targetID, the entity the op created, and deletes them.
func (tx *Tx) ResolvePendingLinks(ctx context.Context, opID, targetID int64) error {
	pending, err := tx.ListPendingLinksByOp(ctx, opID)
	if err != nil {
		return err
	}
	for _, p := range pending {
		if _, err := tx.CreateObservationLink(ctx, domain.ObservationLink{
			ObservationID: p.ObservationID,
			TargetType:    p.TargetType,
			TargetID:      targetID,
			Confidence:    p.Confidence,
		}); err != nil {
			return fmt.Errorf("resolve pending links of op %d: %w", opID, err)
		}
	}
	return tx.DropPendingLinks(ctx, opID)
}

// DropPendingLinks deletes the links waiting on change op opID, which won't
// create anything. The observations stay.
func (tx *Tx) DropPendingLinks(ctx context.Context, opID int64) error {
	if err := tx.q.DeletePendingLinksByOp(ctx, opID); err != nil {
		return fmt.Errorf("drop pending links of op %d: %w", opID, err)
	}
	return nil
}

// deleteLinksTo removes every link pointing at a deleted element.
func (tx *Tx) deleteLinksTo(ctx context.Context, targetType domain.TargetType, targetID int64) error {
	return tx.q.DeleteLinksByTarget(ctx, dbgen.DeleteLinksByTargetParams{
		TargetType: string(targetType),
		TargetID:   targetID,
	})
}
