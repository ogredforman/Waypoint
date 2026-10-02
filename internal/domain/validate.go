package domain

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// Validate methods check one value on its own: enums, required text, and the
// rules the schema's CHECK constraints enforce. They don't know whether
// referenced rows exist; that needs the store. The value's own ID may be zero
// (not saved yet), but required references and any set optional reference must
// be positive IDs.

// FieldError is one problem with one field.
type FieldError struct {
	Field   string // snake_case, matching the column name
	Problem string
}

// ValidationError lists every problem found with one value.
type ValidationError struct {
	Entity   string
	Problems []FieldError
}

func (e *ValidationError) Error() string {
	parts := make([]string, len(e.Problems))
	for i, p := range e.Problems {
		parts[i] = p.Field + ": " + p.Problem
	}
	return "invalid " + e.Entity + ": " + strings.Join(parts, "; ")
}

// checker collects problems while a Validate method runs.
type checker struct {
	problems []FieldError
}

func (c *checker) add(field, format string, args ...any) {
	c.problems = append(c.problems, FieldError{Field: field, Problem: fmt.Sprintf(format, args...)})
}

func (c *checker) notBlank(field, value string) {
	if strings.TrimSpace(value) == "" {
		c.add(field, "is required")
	}
}

func (c *checker) ref(field string, id int64) {
	if id <= 0 {
		c.add(field, "must be a positive ID, got %d", id)
	}
}

func (c *checker) optRef(field string, id *int64) {
	if id != nil {
		c.ref(field, *id)
	}
}

// enum doesn't echo the value: it may come from model output and contain
// diary text, and validation errors get logged.
func (c *checker) enum(field string, valid bool) {
	if !valid {
		c.add(field, "is not an allowed value")
	}
}

func (c *checker) confidence(field string, v float64) {
	if math.IsNaN(v) || v < 0 || v > 1 {
		c.add(field, "must be between 0 and 1, got %v", v)
	}
}

func (c *checker) optJSON(field string, raw json.RawMessage) {
	if raw != nil && !json.Valid(raw) {
		c.add(field, "is not valid JSON")
	}
}

func (c *checker) err(entity string) error {
	if len(c.problems) == 0 {
		return nil
	}
	return &ValidationError{Entity: entity, Problems: c.problems}
}

func (p Project) Validate() error {
	var c checker
	c.notBlank("title", p.Title)
	c.enum("status", p.Status.Valid())
	return c.err("project")
}

func (m Milestone) Validate() error {
	var c checker
	c.ref("project_id", m.ProjectID)
	c.notBlank("title", m.Title)
	c.enum("status", m.Status.Valid())
	return c.err("milestone")
}

func (t Task) Validate() error {
	var c checker
	c.ref("project_id", t.ProjectID)
	c.optRef("milestone_id", t.MilestoneID)
	c.optRef("parent_task_id", t.ParentTaskID)
	c.optRef("origin_entry_id", t.OriginEntryID)
	c.notBlank("title", t.Title)
	c.enum("status", t.Status.Valid())
	if t.Size != nil {
		c.enum("size", t.Size.Valid())
	}
	if t.Energy != nil {
		c.enum("energy", t.Energy.Valid())
	}
	if t.ID != 0 && t.ParentTaskID != nil && *t.ParentTaskID == t.ID {
		c.add("parent_task_id", "a task can't be its own parent")
	}
	return c.err("task")
}

func (d TaskDependency) Validate() error {
	var c checker
	c.ref("task_id", d.TaskID)
	c.ref("blocked_by_task_id", d.BlockedByTaskID)
	if d.TaskID > 0 && d.TaskID == d.BlockedByTaskID {
		c.add("blocked_by_task_id", "a task can't block itself")
	}
	return c.err("task dependency")
}

func (e Entry) Validate() error {
	var c checker
	c.notBlank("body", e.Body)
	c.optRef("project_id", e.ProjectID)
	c.optRef("reply_to_question_id", e.ReplyToQuestionID)
	return c.err("entry")
}

func (o Observation) Validate() error {
	var c checker
	c.ref("entry_id", o.EntryID)
	c.ref("extraction_run_id", o.ExtractionRunID)
	c.enum("kind", o.Kind.Valid())
	c.notBlank("summary", o.Summary)
	if o.SpanStart < 0 {
		c.add("span_start", "must not be negative, got %d", o.SpanStart)
	}
	if o.SpanEnd < o.SpanStart {
		c.add("span_end", "must not be before span_start (%d), got %d", o.SpanStart, o.SpanEnd)
	}
	return c.err("observation")
}

func (l ObservationLink) Validate() error {
	var c checker
	c.ref("observation_id", l.ObservationID)
	c.enum("target_type", l.TargetType.Valid())
	c.ref("target_id", l.TargetID)
	c.confidence("confidence", l.Confidence)
	return c.err("observation link")
}

func (l PendingObservationLink) Validate() error {
	var c checker
	c.ref("observation_id", l.ObservationID)
	c.ref("change_op_id", l.ChangeOpID)
	c.enum("target_type", l.TargetType.Valid())
	c.confidence("confidence", l.Confidence)
	return c.err("pending observation link")
}

func (b Blocker) Validate() error {
	var c checker
	c.ref("project_id", b.ProjectID)
	c.optRef("task_id", b.TaskID)
	c.optRef("origin_entry_id", b.OriginEntryID)
	c.optRef("resolved_entry_id", b.ResolvedEntryID)
	c.notBlank("description", b.Description)
	c.enum("status", b.Status.Valid())
	if b.ResolvedEntryID != nil && b.Status != BlockerResolved {
		c.add("resolved_entry_id", "only a resolved blocker has a resolving entry")
	}
	return c.err("blocker")
}

func (i Idea) Validate() error {
	var c checker
	c.optRef("project_id", i.ProjectID)
	c.optRef("origin_entry_id", i.OriginEntryID)
	c.notBlank("title", i.Title)
	c.enum("status", i.Status.Valid())
	return c.err("idea")
}

func (q FollowUpQuestion) Validate() error {
	var c checker
	c.ref("entry_id", q.EntryID)
	c.ref("extraction_run_id", q.ExtractionRunID)
	c.optRef("answer_entry_id", q.AnswerEntryID)
	c.notBlank("text", q.Text)
	c.enum("status", q.Status.Valid())
	if (q.Status == QuestionAnswered) != (q.AnswerEntryID != nil) {
		c.add("answer_entry_id", "is set exactly when the question is answered")
	}
	return c.err("follow-up question")
}

func (r ExtractionRun) Validate() error {
	var c checker
	c.ref("entry_id", r.EntryID)
	c.notBlank("provider", r.Provider)
	c.optJSON("input_snapshot", r.InputSnapshot)
	c.optJSON("raw_output", r.RawOutput)
	c.enum("status", r.Status.Valid())
	if r.LatencyMS != nil && *r.LatencyMS < 0 {
		c.add("latency_ms", "must not be negative, got %d", *r.LatencyMS)
	}
	return c.err("extraction run")
}

func (s Changeset) Validate() error {
	var c checker
	c.enum("source", s.Source.Valid())
	c.optRef("extraction_run_id", s.ExtractionRunID)
	c.optRef("parent_changeset_id", s.ParentChangesetID)
	c.enum("status", s.Status.Valid())
	if (s.Source == SourceExtraction) != (s.ExtractionRunID != nil) {
		c.add("extraction_run_id", "is set exactly when the source is extraction")
	}
	return c.err("changeset")
}

func (o ChangeOp) Validate() error {
	var c checker
	c.ref("changeset_id", o.ChangesetID)
	// Positions start at 1: temp IDs ("$new:N") name ops by position.
	if o.Position < 1 {
		c.add("position", "must be at least 1, got %d", o.Position)
	}
	c.enum("op_type", o.OpType.Valid())
	if o.Payload == nil {
		c.add("payload", "is required")
	} else {
		c.optJSON("payload", o.Payload)
	}
	c.optJSON("applied_payload", o.AppliedPayload)
	if o.Confidence != nil {
		c.confidence("confidence", *o.Confidence)
	}
	c.enum("status", o.Status.Valid())
	return c.err("change op")
}

func (b Briefing) Validate() error {
	var c checker
	c.ref("project_id", b.ProjectID)
	c.optRef("next_task_id", b.NextTaskID)
	c.enum("source", b.Source.Valid())
	return c.err("briefing")
}
