package store

import (
	"context"
	"fmt"

	"github.com/ogredforman/waypoint/internal/db/dbgen"
	"github.com/ogredforman/waypoint/internal/domain"
)

func questionFromRow(r dbgen.FollowUpQuestion) domain.FollowUpQuestion {
	return domain.FollowUpQuestion{
		ID:              r.ID,
		EntryID:         r.EntryID,
		ExtractionRunID: r.ExtractionRunID,
		Text:            r.Text,
		Reason:          r.Reason,
		Status:          domain.QuestionStatus(r.Status),
		AnswerEntryID:   r.AnswerEntryID,
		CreatedAt:       fromMS(r.CreatedAt),
		UpdatedAt:       fromMS(r.UpdatedAt),
	}
}

// ListQuestionsByEntry returns the follow-up questions asked after the entry,
// from all of its runs, oldest first.
func (r reader) ListQuestionsByEntry(ctx context.Context, entryID int64) ([]domain.FollowUpQuestion, error) {
	rows, err := r.q.ListFollowUpQuestionsByEntry(ctx, entryID)
	if err != nil {
		return nil, fmt.Errorf("list questions of entry %d: %w", entryID, err)
	}
	out := make([]domain.FollowUpQuestion, len(rows))
	for i, row := range rows {
		out[i] = questionFromRow(row)
	}
	return out, nil
}

// CreateQuestion inserts q, ignoring its ID and timestamps.
func (tx *Tx) CreateQuestion(ctx context.Context, q domain.FollowUpQuestion) (domain.FollowUpQuestion, error) {
	if err := q.Validate(); err != nil {
		return domain.FollowUpQuestion{}, fmt.Errorf("create question: %w", err)
	}
	now := tx.nowMS()
	row, err := tx.q.CreateFollowUpQuestion(ctx, dbgen.CreateFollowUpQuestionParams{
		EntryID:         q.EntryID,
		ExtractionRunID: q.ExtractionRunID,
		Text:            q.Text,
		Reason:          q.Reason,
		Status:          string(q.Status),
		AnswerEntryID:   q.AnswerEntryID,
		CreatedAt:       now,
		UpdatedAt:       now,
	})
	if err != nil {
		return domain.FollowUpQuestion{}, fmt.Errorf("create question: %w", err)
	}
	return questionFromRow(row), nil
}

// DismissOpenQuestionsByEntry dismisses the entry's open questions and
// returns how many there were.
func (tx *Tx) DismissOpenQuestionsByEntry(ctx context.Context, entryID int64) (int, error) {
	n, err := tx.q.DismissOpenFollowUpQuestionsByEntry(ctx, dbgen.DismissOpenFollowUpQuestionsByEntryParams{
		UpdatedAt: tx.nowMS(),
		EntryID:   entryID,
	})
	if err != nil {
		return 0, fmt.Errorf("dismiss open questions of entry %d: %w", entryID, err)
	}
	return int(n), nil
}
