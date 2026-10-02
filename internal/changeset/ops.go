package changeset

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/ogredforman/waypoint/internal/domain"
)

// Payload is the typed body of one change op (SPEC §6.2). Each op type has its
// own struct; DecodePayload returns them as values.
type Payload interface {
	OpType() domain.OpType
	// Validate checks the payload on its own: required fields, enums, and
	// well-formed refs. Whether referenced rows exist is CheckRefs' job.
	Validate() error
	// Creates names the kind of entity the op creates, which later ops in the
	// same changeset can reference by temp ID. ok is false for other ops.
	Creates() (kind domain.TargetType, ok bool)
	// refs lists the set refs in the payload, for temp-ID and existence checks.
	refs() []fieldRef
}

// fieldRef is one ref in a payload, with the field it came from and the kind
// of row it must point at.
type fieldRef struct {
	field string
	kind  domain.TargetType
	ref   Ref
}

// Tasks

// CreateTask adds a task to a project. New tasks start as todo; a done task is
// a create_task followed by set_task_status. Milestones are post-MVP, so
// milestone_id is rejected for now.
type CreateTask struct {
	ProjectID    Ref              `json:"project_id"`
	ParentTaskID *Ref             `json:"parent_task_id,omitempty"`
	MilestoneID  *int64           `json:"milestone_id,omitempty"`
	Title        string           `json:"title"`
	Notes        string           `json:"notes,omitempty"`
	Size         *domain.TaskSize `json:"size,omitempty"`
	Energy       *domain.Energy   `json:"energy,omitempty"`
}

// UpdateTask changes the fields that are present. Status changes go through
// SetTaskStatus, not here. A null parent_task_id makes the task top-level.
type UpdateTask struct {
	TaskID       Ref                       `json:"task_id"`
	Title        *string                   `json:"title,omitempty"`
	Notes        *string                   `json:"notes,omitempty"`
	Size         Nullable[domain.TaskSize] `json:"size,omitzero"`
	Energy       Nullable[domain.Energy]   `json:"energy,omitzero"`
	ParentTaskID Nullable[Ref]             `json:"parent_task_id,omitzero"`
	Position     *int64                    `json:"position,omitempty"`
}

// SetTaskStatus moves a task to another status.
type SetTaskStatus struct {
	TaskID Ref               `json:"task_id"`
	Status domain.TaskStatus `json:"status"`
}

// SplitTask breaks a vague task into startable subtasks of it. The subtasks
// can't be referenced by temp ID.
type SplitTask struct {
	TaskID   Ref       `json:"task_id"`
	Subtasks []Subtask `json:"subtasks"`
}

// Subtask is one step proposed by SplitTask.
type Subtask struct {
	Title  string           `json:"title"`
	Notes  string           `json:"notes,omitempty"`
	Size   *domain.TaskSize `json:"size,omitempty"`
	Energy *domain.Energy   `json:"energy,omitempty"`
}

// AddDependency says TaskID can't start until BlockedByTaskID is done.
type AddDependency struct {
	TaskID          Ref `json:"task_id"`
	BlockedByTaskID Ref `json:"blocked_by_task_id"`
}

// DeleteTask removes a task for good (ADR 0002).
type DeleteTask struct {
	TaskID Ref `json:"task_id"`
}

// Blockers

// CreateBlocker records a blocker on a project, optionally on one of its tasks.
type CreateBlocker struct {
	ProjectID   Ref    `json:"project_id"`
	TaskID      *Ref   `json:"task_id,omitempty"`
	Description string `json:"description"`
}

// UpdateBlocker changes the fields that are present. A null task_id leaves
// the blocker on the project with no task.
type UpdateBlocker struct {
	BlockerID   Ref           `json:"blocker_id"`
	Description *string       `json:"description,omitempty"`
	TaskID      Nullable[Ref] `json:"task_id,omitzero"`
}

// ResolveBlocker marks a blocker resolved.
type ResolveBlocker struct {
	BlockerID Ref `json:"blocker_id"`
}

// DeleteBlocker removes a blocker for good (ADR 0002).
type DeleteBlocker struct {
	BlockerID Ref `json:"blocker_id"`
}

// Ideas

// ParkIdea puts a new idea in the parking lot, optionally tied to a project.
type ParkIdea struct {
	ProjectID *Ref   `json:"project_id,omitempty"`
	Title     string `json:"title"`
	Body      string `json:"body,omitempty"`
}

// UpdateIdea changes the fields that are present. A null project_id unties
// the idea from its project. Promoting an idea is a status change here plus
// the op that creates the task or project.
type UpdateIdea struct {
	IdeaID    Ref                `json:"idea_id"`
	Title     *string            `json:"title,omitempty"`
	Body      *string            `json:"body,omitempty"`
	ProjectID Nullable[Ref]      `json:"project_id,omitzero"`
	Status    *domain.IdeaStatus `json:"status,omitempty"`
}

// DeleteIdea removes an idea for good (ADR 0002).
type DeleteIdea struct {
	IdeaID Ref `json:"idea_id"`
}

// Projects

// CreateProject adds a project. It is always created parked (SPEC §6.2); a
// manual "create active" follows it with an UpdateProject in the same
// changeset.
type CreateProject struct {
	Title string `json:"title"`
	Why   string `json:"why,omitempty"`
}

// UpdateProject changes the fields that are present. The active-project cap
// on a status change is checked by the applier.
type UpdateProject struct {
	ProjectID        Ref                   `json:"project_id"`
	Title            *string               `json:"title,omitempty"`
	Why              *string               `json:"why,omitempty"`
	DefinitionOfDone *string               `json:"definition_of_done,omitempty"`
	Status           *domain.ProjectStatus `json:"status,omitempty"`
}

// OpType

func (CreateTask) OpType() domain.OpType     { return domain.OpCreateTask }
func (UpdateTask) OpType() domain.OpType     { return domain.OpUpdateTask }
func (SetTaskStatus) OpType() domain.OpType  { return domain.OpSetTaskStatus }
func (SplitTask) OpType() domain.OpType      { return domain.OpSplitTask }
func (AddDependency) OpType() domain.OpType  { return domain.OpAddDependency }
func (DeleteTask) OpType() domain.OpType     { return domain.OpDeleteTask }
func (CreateBlocker) OpType() domain.OpType  { return domain.OpCreateBlocker }
func (UpdateBlocker) OpType() domain.OpType  { return domain.OpUpdateBlocker }
func (ResolveBlocker) OpType() domain.OpType { return domain.OpResolveBlocker }
func (DeleteBlocker) OpType() domain.OpType  { return domain.OpDeleteBlocker }
func (ParkIdea) OpType() domain.OpType       { return domain.OpParkIdea }
func (UpdateIdea) OpType() domain.OpType     { return domain.OpUpdateIdea }
func (DeleteIdea) OpType() domain.OpType     { return domain.OpDeleteIdea }
func (CreateProject) OpType() domain.OpType  { return domain.OpCreateProject }
func (UpdateProject) OpType() domain.OpType  { return domain.OpUpdateProject }

// Creates

func (CreateTask) Creates() (domain.TargetType, bool)     { return domain.TargetTask, true }
func (UpdateTask) Creates() (domain.TargetType, bool)     { return "", false }
func (SetTaskStatus) Creates() (domain.TargetType, bool)  { return "", false }
func (SplitTask) Creates() (domain.TargetType, bool)      { return "", false }
func (AddDependency) Creates() (domain.TargetType, bool)  { return "", false }
func (DeleteTask) Creates() (domain.TargetType, bool)     { return "", false }
func (CreateBlocker) Creates() (domain.TargetType, bool)  { return domain.TargetBlocker, true }
func (UpdateBlocker) Creates() (domain.TargetType, bool)  { return "", false }
func (ResolveBlocker) Creates() (domain.TargetType, bool) { return "", false }
func (DeleteBlocker) Creates() (domain.TargetType, bool)  { return "", false }
func (ParkIdea) Creates() (domain.TargetType, bool)       { return domain.TargetIdea, true }
func (UpdateIdea) Creates() (domain.TargetType, bool)     { return "", false }
func (DeleteIdea) Creates() (domain.TargetType, bool)     { return "", false }
func (CreateProject) Creates() (domain.TargetType, bool)  { return domain.TargetProject, true }
func (UpdateProject) Creates() (domain.TargetType, bool)  { return "", false }

// FieldRef is one ref set in a payload: the JSON field it came from and the
// kind of row it must point at.
type FieldRef struct {
	Field string
	Kind  domain.TargetType
	Ref   Ref
}

// Refs lists the refs set in p, in field order. Extraction validation uses it
// to check existing-row refs against the snapshot.
func Refs(p Payload) []FieldRef {
	var out []FieldRef
	for _, r := range p.refs() {
		out = append(out, FieldRef{Field: r.field, Kind: r.kind, Ref: r.ref})
	}
	return out
}

// refs

func one(field string, kind domain.TargetType, r Ref) fieldRef {
	return fieldRef{field: field, kind: kind, ref: r}
}

func appendOpt(out []fieldRef, field string, kind domain.TargetType, r *Ref) []fieldRef {
	if r != nil {
		out = append(out, one(field, kind, *r))
	}
	return out
}

func appendNullable(out []fieldRef, field string, kind domain.TargetType, n Nullable[Ref]) []fieldRef {
	return appendOpt(out, field, kind, n.Value)
}

func (p CreateTask) refs() []fieldRef {
	return appendOpt([]fieldRef{one("project_id", domain.TargetProject, p.ProjectID)},
		"parent_task_id", domain.TargetTask, p.ParentTaskID)
}

func (p UpdateTask) refs() []fieldRef {
	return appendNullable([]fieldRef{one("task_id", domain.TargetTask, p.TaskID)},
		"parent_task_id", domain.TargetTask, p.ParentTaskID)
}

func (p SetTaskStatus) refs() []fieldRef {
	return []fieldRef{one("task_id", domain.TargetTask, p.TaskID)}
}

func (p SplitTask) refs() []fieldRef {
	return []fieldRef{one("task_id", domain.TargetTask, p.TaskID)}
}

func (p AddDependency) refs() []fieldRef {
	return []fieldRef{
		one("task_id", domain.TargetTask, p.TaskID),
		one("blocked_by_task_id", domain.TargetTask, p.BlockedByTaskID),
	}
}

func (p DeleteTask) refs() []fieldRef {
	return []fieldRef{one("task_id", domain.TargetTask, p.TaskID)}
}

func (p CreateBlocker) refs() []fieldRef {
	return appendOpt([]fieldRef{one("project_id", domain.TargetProject, p.ProjectID)},
		"task_id", domain.TargetTask, p.TaskID)
}

func (p UpdateBlocker) refs() []fieldRef {
	return appendNullable([]fieldRef{one("blocker_id", domain.TargetBlocker, p.BlockerID)},
		"task_id", domain.TargetTask, p.TaskID)
}

func (p ResolveBlocker) refs() []fieldRef {
	return []fieldRef{one("blocker_id", domain.TargetBlocker, p.BlockerID)}
}

func (p DeleteBlocker) refs() []fieldRef {
	return []fieldRef{one("blocker_id", domain.TargetBlocker, p.BlockerID)}
}

func (p ParkIdea) refs() []fieldRef {
	return appendOpt(nil, "project_id", domain.TargetProject, p.ProjectID)
}

func (p UpdateIdea) refs() []fieldRef {
	return appendNullable([]fieldRef{one("idea_id", domain.TargetIdea, p.IdeaID)},
		"project_id", domain.TargetProject, p.ProjectID)
}

func (p DeleteIdea) refs() []fieldRef {
	return []fieldRef{one("idea_id", domain.TargetIdea, p.IdeaID)}
}

func (CreateProject) refs() []fieldRef { return nil }

func (p UpdateProject) refs() []fieldRef {
	return []fieldRef{one("project_id", domain.TargetProject, p.ProjectID)}
}

// Validate

func (p CreateTask) Validate() error {
	var c checker
	c.refs(p.refs())
	if p.MilestoneID != nil {
		c.add("milestone_id", "milestones aren't supported yet")
	}
	c.notBlank("title", p.Title)
	optEnum(&c, "size", p.Size)
	optEnum(&c, "energy", p.Energy)
	return c.err(p.OpType())
}

func (p UpdateTask) Validate() error {
	var c checker
	c.refs(p.refs())
	c.optNotBlank("title", p.Title)
	optEnum(&c, "size", p.Size.Value)
	optEnum(&c, "energy", p.Energy.Value)
	if p.Position != nil && *p.Position < 0 {
		c.add("position", "must not be negative, got %d", *p.Position)
	}
	if v := p.ParentTaskID.Value; v != nil && v.Problem() == "" && *v == p.TaskID {
		c.add("parent_task_id", "a task can't be its own parent")
	}
	if p.Title == nil && p.Notes == nil && !p.Size.Set && !p.Energy.Set &&
		!p.ParentTaskID.Set && p.Position == nil {
		c.add("payload", "changes nothing")
	}
	return c.err(p.OpType())
}

func (p SetTaskStatus) Validate() error {
	var c checker
	c.refs(p.refs())
	c.enum("status", p.Status.Valid())
	return c.err(p.OpType())
}

func (p SplitTask) Validate() error {
	var c checker
	c.refs(p.refs())
	if len(p.Subtasks) == 0 {
		c.add("subtasks", "needs at least one subtask")
	}
	for i, s := range p.Subtasks {
		prefix := fmt.Sprintf("subtasks[%d].", i)
		c.notBlank(prefix+"title", s.Title)
		optEnum(&c, prefix+"size", s.Size)
		optEnum(&c, prefix+"energy", s.Energy)
	}
	return c.err(p.OpType())
}

func (p AddDependency) Validate() error {
	var c checker
	c.refs(p.refs())
	if p.TaskID.Problem() == "" && p.TaskID == p.BlockedByTaskID {
		c.add("blocked_by_task_id", "a task can't block itself")
	}
	return c.err(p.OpType())
}

func (p DeleteTask) Validate() error {
	var c checker
	c.refs(p.refs())
	return c.err(p.OpType())
}

func (p CreateBlocker) Validate() error {
	var c checker
	c.refs(p.refs())
	c.notBlank("description", p.Description)
	return c.err(p.OpType())
}

func (p UpdateBlocker) Validate() error {
	var c checker
	c.refs(p.refs())
	c.optNotBlank("description", p.Description)
	if p.Description == nil && !p.TaskID.Set {
		c.add("payload", "changes nothing")
	}
	return c.err(p.OpType())
}

func (p ResolveBlocker) Validate() error {
	var c checker
	c.refs(p.refs())
	return c.err(p.OpType())
}

func (p DeleteBlocker) Validate() error {
	var c checker
	c.refs(p.refs())
	return c.err(p.OpType())
}

func (p ParkIdea) Validate() error {
	var c checker
	c.refs(p.refs())
	c.notBlank("title", p.Title)
	return c.err(p.OpType())
}

func (p UpdateIdea) Validate() error {
	var c checker
	c.refs(p.refs())
	c.optNotBlank("title", p.Title)
	optEnum(&c, "status", p.Status)
	if p.Title == nil && p.Body == nil && !p.ProjectID.Set && p.Status == nil {
		c.add("payload", "changes nothing")
	}
	return c.err(p.OpType())
}

func (p DeleteIdea) Validate() error {
	var c checker
	c.refs(p.refs())
	return c.err(p.OpType())
}

func (p CreateProject) Validate() error {
	var c checker
	c.notBlank("title", p.Title)
	return c.err(p.OpType())
}

func (p UpdateProject) Validate() error {
	var c checker
	c.refs(p.refs())
	c.optNotBlank("title", p.Title)
	optEnum(&c, "status", p.Status)
	if p.Title == nil && p.Why == nil && p.DefinitionOfDone == nil && p.Status == nil {
		c.add("payload", "changes nothing")
	}
	return c.err(p.OpType())
}

// Decoding

// DecodePayload decodes and validates the payload of an op of type t. Unknown
// fields and trailing data are rejected. Every problem comes back as a
// *domain.ValidationError; the payload is returned alongside it when it
// decoded, so callers can still inspect it.
func DecodePayload(t domain.OpType, raw json.RawMessage) (Payload, error) {
	switch t {
	case domain.OpCreateTask:
		return decode[CreateTask](raw)
	case domain.OpUpdateTask:
		return decode[UpdateTask](raw)
	case domain.OpSetTaskStatus:
		return decode[SetTaskStatus](raw)
	case domain.OpSplitTask:
		return decode[SplitTask](raw)
	case domain.OpAddDependency:
		return decode[AddDependency](raw)
	case domain.OpDeleteTask:
		return decode[DeleteTask](raw)
	case domain.OpCreateBlocker:
		return decode[CreateBlocker](raw)
	case domain.OpUpdateBlocker:
		return decode[UpdateBlocker](raw)
	case domain.OpResolveBlocker:
		return decode[ResolveBlocker](raw)
	case domain.OpDeleteBlocker:
		return decode[DeleteBlocker](raw)
	case domain.OpParkIdea:
		return decode[ParkIdea](raw)
	case domain.OpUpdateIdea:
		return decode[UpdateIdea](raw)
	case domain.OpDeleteIdea:
		return decode[DeleteIdea](raw)
	case domain.OpCreateProject:
		return decode[CreateProject](raw)
	case domain.OpUpdateProject:
		return decode[UpdateProject](raw)
	}
	var c checker
	c.enum("op_type", false)
	return nil, c.errEntity("change op")
}

func decode[T Payload](raw json.RawMessage) (Payload, error) {
	var p T
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		var c checker
		c.decodeProblem(err)
		return nil, c.err(p.OpType())
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		var c checker
		c.add("payload", "has data after the JSON object")
		return nil, c.err(p.OpType())
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		var c checker
		c.add("payload", "must be a JSON object")
		return nil, c.err(p.OpType())
	}
	return p, p.Validate()
}

// jsonFieldName is what a payload field name looks like. Unknown field names
// are only echoed in errors when they match, since model output could put
// anything there.
var jsonFieldName = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)

func (c *checker) decodeProblem(err error) {
	var typeErr *json.UnmarshalTypeError
	var syntaxErr *json.SyntaxError
	switch {
	case errors.As(err, &typeErr):
		field := typeErr.Field
		if field == "" {
			c.add("payload", "must be a JSON object")
			return
		}
		c.add(field, "has the wrong type")
	case errors.As(err, &syntaxErr), errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		c.add("payload", "is not valid JSON")
	case strings.HasPrefix(err.Error(), `json: unknown field "`):
		name := strings.TrimSuffix(strings.TrimPrefix(err.Error(), `json: unknown field "`), `"`)
		if jsonFieldName.MatchString(name) {
			c.add(name, "is not a known field")
		} else {
			c.add("payload", "has an unknown field")
		}
	default:
		c.add("payload", "could not be decoded")
	}
}
