package api

import (
	"context"
	"unicode/utf16"

	"github.com/ogredforman/waypoint/internal/api/apitypes"
	"github.com/ogredforman/waypoint/internal/briefing"
	"github.com/ogredforman/waypoint/internal/domain"
)

// Conversions from domain values to wire DTOs. Lists are never nil, so they
// encode as [] rather than null.

func projectDTO(p domain.Project) apitypes.Project {
	return apitypes.Project{
		ID:               p.ID,
		Title:            p.Title,
		Why:              p.Why,
		DefinitionOfDone: p.DefinitionOfDone,
		Status:           p.Status,
		CreatedAt:        p.CreatedAt,
		UpdatedAt:        p.UpdatedAt,
	}
}

func taskDTO(t domain.Task) apitypes.Task {
	return apitypes.Task{
		ID:            t.ID,
		ProjectID:     t.ProjectID,
		ParentTaskID:  t.ParentTaskID,
		Title:         t.Title,
		Notes:         t.Notes,
		Status:        t.Status,
		Size:          t.Size,
		Energy:        t.Energy,
		Position:      t.Position,
		OriginEntryID: t.OriginEntryID,
		CreatedAt:     t.CreatedAt,
		UpdatedAt:     t.UpdatedAt,
	}
}

func blockerDTO(b domain.Blocker) apitypes.Blocker {
	return apitypes.Blocker{
		ID:              b.ID,
		ProjectID:       b.ProjectID,
		TaskID:          b.TaskID,
		Description:     b.Description,
		Status:          b.Status,
		OriginEntryID:   b.OriginEntryID,
		ResolvedEntryID: b.ResolvedEntryID,
		CreatedAt:       b.CreatedAt,
		UpdatedAt:       b.UpdatedAt,
	}
}

func ideaDTO(i domain.Idea) apitypes.Idea {
	return apitypes.Idea{
		ID:            i.ID,
		ProjectID:     i.ProjectID,
		Title:         i.Title,
		Body:          i.Body,
		Status:        i.Status,
		OriginEntryID: i.OriginEntryID,
		CreatedAt:     i.CreatedAt,
		UpdatedAt:     i.UpdatedAt,
	}
}

func entryDTO(e domain.Entry) apitypes.Entry {
	return apitypes.Entry{
		ID:                e.ID,
		Body:              e.Body,
		ProjectID:         e.ProjectID,
		ReplyToQuestionID: e.ReplyToQuestionID,
		CreatedAt:         e.CreatedAt,
	}
}

// runDTO leaves out the provider, model, snapshot, and raw output: the UI
// only needs to know how the run went.
func runDTO(r domain.ExtractionRun) apitypes.ExtractionRun {
	return apitypes.ExtractionRun{
		ID:        r.ID,
		EntryID:   r.EntryID,
		Status:    r.Status,
		Error:     r.Error,
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}
}

func questionDTO(q domain.FollowUpQuestion) apitypes.Question {
	return apitypes.Question{
		ID:            q.ID,
		EntryID:       q.EntryID,
		Text:          q.Text,
		Reason:        q.Reason,
		Status:        q.Status,
		AnswerEntryID: q.AnswerEntryID,
		CreatedAt:     q.CreatedAt,
	}
}

func mapSlice[T, U any](in []T, f func(T) U) []U {
	out := make([]U, len(in))
	for i, v := range in {
		out[i] = f(v)
	}
	return out
}

func briefingDTO(v briefing.View) apitypes.Briefing {
	b := apitypes.Briefing{
		ProjectID:    v.Briefing.ProjectID,
		NextReason:   v.Briefing.NextReason,
		Recap:        v.Briefing.Recap,
		Source:       v.Briefing.Source,
		Blockers:     mapSlice(v.Blockers, blockerDTO),
		Alternatives: mapSlice(v.Alternatives, taskDTO),
		GeneratedAt:  v.Briefing.GeneratedAt,
	}
	if v.Next != nil {
		t := taskDTO(*v.Next)
		b.NextTask = &t
	}
	if v.Excerpt != nil {
		b.Excerpt = &apitypes.Excerpt{EntryID: v.Excerpt.EntryID, CreatedAt: v.Excerpt.CreatedAt, Text: v.Excerpt.Text}
	}
	return b
}

// changesetReads are the store reads changesetDTO needs, from the Store or a
// read transaction.
type changesetReads interface {
	ListChangeOps(ctx context.Context, changesetID int64) ([]domain.ChangeOp, error)
	ChangesetEntryID(ctx context.Context, cs domain.Changeset) (*int64, error)
}

func changesetDTO(ctx context.Context, r changesetReads, cs domain.Changeset) (apitypes.Changeset, error) {
	ops, err := r.ListChangeOps(ctx, cs.ID)
	if err != nil {
		return apitypes.Changeset{}, err
	}
	entryID, err := r.ChangesetEntryID(ctx, cs)
	if err != nil {
		return apitypes.Changeset{}, err
	}
	return apitypes.Changeset{
		ID:                cs.ID,
		Source:            cs.Source,
		EntryID:           entryID,
		ParentChangesetID: cs.ParentChangesetID,
		Status:            cs.Status,
		Ops:               mapSlice(ops, changeOpDTO),
		CreatedAt:         cs.CreatedAt,
		ResolvedAt:        cs.ResolvedAt,
	}, nil
}

func changeOpDTO(op domain.ChangeOp) apitypes.ChangeOp {
	return apitypes.ChangeOp{
		ID:             op.ID,
		Position:       op.Position,
		OpType:         op.OpType,
		Payload:        op.Payload,
		Rationale:      op.Rationale,
		Confidence:     op.Confidence,
		Status:         op.Status,
		AppliedPayload: op.AppliedPayload,
	}
}

// utf16Offset converts off, a byte offset into the UTF-8 string s, to the
// same position counted in UTF-16 code units, which is how JavaScript
// indexes strings (SPEC §10.3). An offset past the end counts as the end.
func utf16Offset(s string, off int64) int64 {
	off = min(max(off, 0), int64(len(s)))
	var n int64
	for _, r := range s[:off] {
		n += int64(utf16.RuneLen(r))
	}
	return n
}

// quote returns the part of body an observation's byte span covers, or ""
// if the span doesn't fit the body.
func quote(body string, o domain.Observation) string {
	if o.SpanStart < 0 || o.SpanEnd < o.SpanStart || o.SpanEnd > int64(len(body)) {
		return ""
	}
	return body[o.SpanStart:o.SpanEnd]
}
