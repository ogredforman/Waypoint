package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"unicode/utf16"

	"github.com/ogredforman/waypoint/internal/api/apitypes"
	"github.com/ogredforman/waypoint/internal/briefing"
	"github.com/ogredforman/waypoint/internal/domain"
	"github.com/ogredforman/waypoint/internal/store"
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
	GetProject(ctx context.Context, id int64) (domain.Project, error)
	GetTask(ctx context.Context, id int64) (domain.Task, error)
	GetBlocker(ctx context.Context, id int64) (domain.Blocker, error)
	GetIdea(ctx context.Context, id int64) (domain.Idea, error)
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
	dtos := make([]apitypes.ChangeOp, len(ops))
	for i, op := range ops {
		dtos[i] = changeOpDTO(op)
		if dtos[i].Refs, err = opRefs(ctx, r, op.Payload); err != nil {
			return apitypes.Changeset{}, err
		}
	}
	return apitypes.Changeset{
		ID:                cs.ID,
		Source:            cs.Source,
		EntryID:           entryID,
		ParentChangesetID: cs.ParentChangesetID,
		Status:            cs.Status,
		Ops:               dtos,
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
		Refs:           map[string]string{},
	}
}

// refFields are the payload fields that refer to an existing row (SPEC §6.2),
// and the kind of row each names.
var refFields = map[string]string{
	"project_id":         "project",
	"task_id":            "task",
	"parent_task_id":     "task",
	"blocked_by_task_id": "task",
	"blocker_id":         "blocker",
	"idea_id":            "idea",
}

// opRefs names the existing rows an op payload refers to, keyed "kind:id".
// Temporary IDs ("$new:N"), nulls, and rows that no longer exist are skipped.
func opRefs(ctx context.Context, r changesetReads, payload json.RawMessage) (map[string]string, error) {
	refs := map[string]string{}
	var fields map[string]json.RawMessage
	if json.Unmarshal(payload, &fields) != nil {
		return refs, nil
	}
	for field, kind := range refFields {
		var id *int64
		if raw, ok := fields[field]; !ok || json.Unmarshal(raw, &id) != nil || id == nil {
			continue
		}
		name, err := refName(ctx, r, kind, *id)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("name %s %d: %w", kind, *id, err)
		}
		refs[fmt.Sprintf("%s:%d", kind, *id)] = name
	}
	return refs, nil
}

func refName(ctx context.Context, r changesetReads, kind string, id int64) (string, error) {
	switch kind {
	case "project":
		p, err := r.GetProject(ctx, id)
		return p.Title, err
	case "task":
		t, err := r.GetTask(ctx, id)
		return t.Title, err
	case "blocker":
		b, err := r.GetBlocker(ctx, id)
		return b.Description, err
	default:
		i, err := r.GetIdea(ctx, id)
		return i.Title, err
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
