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

// ListRecentEntriesForProject returns up to limit entries about the project,
// newest first: those scoped to it, and unscoped ones with an observation
// linked to the project or to one of its tasks or blockers.
func (r reader) ListRecentEntriesForProject(ctx context.Context, projectID int64, limit int) ([]domain.Entry, error) {
	if err := checkLimit(limit); err != nil {
		return nil, fmt.Errorf("list entries about project %d: %w", projectID, err)
	}
	rows, err := r.q.ListRecentEntriesForProject(ctx, dbgen.ListRecentEntriesForProjectParams{
		ProjectID: &projectID,
		Limit:     int64(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("list entries about project %d: %w", projectID, err)
	}
	return entriesFromRows(rows), nil
}

// ListEntriesBefore returns up to limit entries written before the entry
// beforeID, newest first.
func (r reader) ListEntriesBefore(ctx context.Context, beforeID int64, limit int) ([]domain.Entry, error) {
	if err := checkLimit(limit); err != nil {
		return nil, fmt.Errorf("list entries before %d: %w", beforeID, err)
	}
	rows, err := r.q.ListEntriesBefore(ctx, dbgen.ListEntriesBeforeParams{
		BeforeID: beforeID,
		Limit:    int64(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("list entries before %d: %w", beforeID, err)
	}
	return entriesFromRows(rows), nil
}

// ListEntriesBeforeByProject returns up to limit entries scoped to the
// project and written before the entry beforeID, newest first.
func (r reader) ListEntriesBeforeByProject(ctx context.Context, projectID, beforeID int64, limit int) ([]domain.Entry, error) {
	if err := checkLimit(limit); err != nil {
		return nil, fmt.Errorf("list entries of project %d before %d: %w", projectID, beforeID, err)
	}
	rows, err := r.q.ListEntriesBeforeByProject(ctx, dbgen.ListEntriesBeforeByProjectParams{
		ProjectID: &projectID,
		BeforeID:  beforeID,
		Limit:     int64(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("list entries of project %d before %d: %w", projectID, beforeID, err)
	}
	return entriesFromRows(rows), nil
}

// ListEntriesBeforeForProject returns up to limit entries about the project
// (as in ListRecentEntriesForProject) written before the entry beforeID,
// newest first.
func (r reader) ListEntriesBeforeForProject(ctx context.Context, projectID, beforeID int64, limit int) ([]domain.Entry, error) {
	if err := checkLimit(limit); err != nil {
		return nil, fmt.Errorf("list entries about project %d before %d: %w", projectID, beforeID, err)
	}
	rows, err := r.q.ListEntriesBeforeForProject(ctx, dbgen.ListEntriesBeforeForProjectParams{
		BeforeID:  beforeID,
		ProjectID: &projectID,
		Limit:     int64(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("list entries about project %d before %d: %w", projectID, beforeID, err)
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

// DeleteEntry removes entry id with everything derived from it (ADR 0007):
// its runs, observations with their links and pending links, follow-up
// questions, and changesets (pending, split-off, and resolved, with their
// ops), plus its briefing recap points. The text is never edited; the entry
// goes whole. Changes already applied from it stay: tasks, blockers, and
// ideas lose their reference to it (origin, resolved-by), which is reference
// cleanup, not an edit of those rows. A question it answered becomes
// dismissed, and entries replying to its questions lose the reply link. It
// returns the deleted entry, or ErrNotFound. The caller makes sure no run of
// the entry is running.
func (tx *Tx) DeleteEntry(ctx context.Context, id int64) (domain.Entry, error) {
	e, err := tx.GetEntry(ctx, id)
	if err != nil {
		return domain.Entry{}, err
	}
	ref := &id
	steps := []struct {
		what string
		run  func() error
	}{
		{"pending links", func() error { return tx.q.DeleteEntryPendingLinks(ctx, id) }},
		{"observation links", func() error { return tx.q.DeleteEntryObservationLinks(ctx, id) }},
		{"observations", func() error { return tx.q.DeleteEntryObservations(ctx, id) }},
		{"change ops", func() error { return tx.q.DeleteEntryChangeOps(ctx, id) }},
		{"changesets", func() error { return tx.q.DeleteEntryChangesets(ctx, id) }},
		{"reply links", func() error { return tx.q.ClearRepliesToEntryQuestions(ctx, id) }},
		{"answered questions", func() error {
			return tx.q.DismissQuestionsAnsweredByEntry(ctx, dbgen.DismissQuestionsAnsweredByEntryParams{
				UpdatedAt: tx.nowMS(), AnswerEntryID: ref,
			})
		}},
		{"questions", func() error { return tx.q.DeleteEntryQuestions(ctx, id) }},
		{"task origins", func() error { return tx.q.ClearTaskOrigins(ctx, ref) }},
		{"blocker entries", func() error { return tx.q.ClearBlockerEntries(ctx, ref) }},
		{"idea origins", func() error { return tx.q.ClearIdeaOrigins(ctx, ref) }},
		{"runs", func() error { return tx.q.DeleteEntryRuns(ctx, id) }},
		{"entry", func() error { return tx.q.DeleteEntry(ctx, id) }},
	}
	for _, s := range steps {
		if err := s.run(); err != nil {
			return domain.Entry{}, fmt.Errorf("delete entry %d: %s: %w", id, s.what, err)
		}
	}
	return e, nil
}
