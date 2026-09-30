package changeset

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ogredforman/waypoint/internal/domain"
	"github.com/ogredforman/waypoint/internal/store"
)

// checker collects problems while a payload or changeset is validated.
type checker struct {
	problems []domain.FieldError
}

func (c *checker) add(field, format string, args ...any) {
	c.problems = append(c.problems, domain.FieldError{Field: field, Problem: fmt.Sprintf(format, args...)})
}

func (c *checker) notBlank(field, value string) {
	if strings.TrimSpace(value) == "" {
		c.add(field, "is required")
	}
}

func (c *checker) optNotBlank(field string, value *string) {
	if value != nil {
		c.notBlank(field, *value)
	}
}

// enum doesn't echo the value: it may come from model output.
func (c *checker) enum(field string, valid bool) {
	if !valid {
		c.add(field, "is not an allowed value")
	}
}

// optEnum checks an optional enum. It's a function, not a method, because
// methods can't have type parameters.
func optEnum[T interface{ Valid() bool }](c *checker, field string, v *T) {
	c.enum(field, v == nil || (*v).Valid())
}

func (c *checker) refs(refs []fieldRef) {
	for _, r := range refs {
		if p := r.ref.problem(); p != "" {
			c.add(r.field, "%s", p)
		}
	}
}

func (c *checker) err(t domain.OpType) error { return c.errEntity(string(t) + " op") }

func (c *checker) errEntity(entity string) error {
	if len(c.problems) == 0 {
		return nil
	}
	return &domain.ValidationError{Entity: entity, Problems: c.problems}
}

// ValidateOps decodes and validates the ops of one changeset, given in
// position order, and returns their payloads. Positions must be positive and
// increasing. A temp ref must name an earlier op that creates the kind of
// entity the field expects. Problems from every op are gathered into one
// *domain.ValidationError, with fields named "ops[<position>].<field>".
func ValidateOps(ops []domain.ChangeOp) ([]Payload, error) {
	var c checker
	payloads := make([]Payload, len(ops))
	creates := map[int64]domain.TargetType{} // position -> kind created
	var last int64
	for i, op := range ops {
		prefix := fmt.Sprintf("ops[%d].", op.Position)
		if op.Position <= last {
			c.add(prefix+"position", "must be positive and after the previous op's")
		}
		last = max(last, op.Position)

		p, err := DecodePayload(op.OpType, op.Payload)
		var ve *domain.ValidationError
		if errors.As(err, &ve) {
			for _, fp := range ve.Problems {
				c.add(prefix+fp.Field, "%s", fp.Problem)
			}
		} else if err != nil {
			return nil, fmt.Errorf("op at position %d: %w", op.Position, err)
		}
		if p == nil {
			continue
		}
		payloads[i] = p

		for _, r := range p.refs() {
			if !r.ref.IsTemp() || r.ref.problem() != "" {
				continue
			}
			kind, ok := creates[r.ref.Temp]
			switch {
			case r.ref.Temp >= op.Position:
				c.add(prefix+r.field, "refers to an op that isn't earlier in the changeset")
			case !ok:
				c.add(prefix+r.field, "refers to position %d, where no op creates anything", r.ref.Temp)
			case kind != r.kind:
				c.add(prefix+r.field, "refers to position %d, which creates a %s, not a %s", r.ref.Temp, kind, r.kind)
			}
		}
		if kind, ok := p.Creates(); ok {
			creates[op.Position] = kind
		}
	}
	return payloads, c.errEntity("changeset")
}

// Reader is what CheckRefs reads. *store.Tx satisfies it.
type Reader interface {
	GetProject(ctx context.Context, id int64) (domain.Project, error)
	GetTask(ctx context.Context, id int64) (domain.Task, error)
	GetBlocker(ctx context.Context, id int64) (domain.Blocker, error)
	GetIdea(ctx context.Context, id int64) (domain.Idea, error)
}

// TempIDs maps the position of an applied creating op to the ID of the row it
// created.
type TempIDs map[int64]int64

// Resolve returns the row ID r points at. ok is false for a temp ref whose
// creating op hasn't been applied.
func (m TempIDs) Resolve(r Ref) (id int64, ok bool) {
	if !r.IsTemp() {
		return r.ID, true
	}
	id, ok = m[r.Temp]
	return id, ok
}

// maxTaskDepth bounds the walk up a task's parents. The tree is acyclic, so
// hitting it means the data is corrupt, not that the op is invalid.
const maxTaskDepth = 10000

// CheckRefs checks p against the current state, which includes the ops of
// the changeset applied before it: every ref resolves and points at an
// existing row; a parent task or a blocker's task is in the same project; a
// dependency's tasks are in the same project and differ; and a new parent
// doesn't make a cycle. The applier calls it inside the apply transaction,
// just before applying p. Problems come back as a *domain.ValidationError;
// any other error is a failed read.
func CheckRefs(ctx context.Context, r Reader, p Payload, ids TempIDs) error {
	k := refChecker{
		r:        r,
		ids:      map[string]int64{},
		tasks:    map[string]domain.Task{},
		blockers: map[string]domain.Blocker{},
		ideas:    map[string]domain.Idea{},
	}
	for _, fr := range p.refs() {
		if err := k.load(ctx, fr, ids); err != nil {
			return err
		}
	}
	if err := k.rules(ctx, p); err != nil {
		return err
	}
	return k.c.err(p.OpType())
}

// refChecker holds the rows CheckRefs loaded, keyed by payload field.
type refChecker struct {
	r        Reader
	c        checker
	ids      map[string]int64
	tasks    map[string]domain.Task
	blockers map[string]domain.Blocker
	ideas    map[string]domain.Idea
}

func (k *refChecker) load(ctx context.Context, fr fieldRef, temp TempIDs) error {
	if p := fr.ref.problem(); p != "" {
		k.c.add(fr.field, "%s", p)
		return nil
	}
	id, ok := temp.Resolve(fr.ref)
	if !ok {
		k.c.add(fr.field, "refers to the op at position %d, which wasn't applied", fr.ref.Temp)
		return nil
	}
	var err error
	switch fr.kind {
	case domain.TargetProject:
		_, err = k.r.GetProject(ctx, id)
	case domain.TargetTask:
		var t domain.Task
		if t, err = k.r.GetTask(ctx, id); err == nil {
			k.tasks[fr.field] = t
		}
	case domain.TargetBlocker:
		var b domain.Blocker
		if b, err = k.r.GetBlocker(ctx, id); err == nil {
			k.blockers[fr.field] = b
		}
	case domain.TargetIdea:
		var i domain.Idea
		if i, err = k.r.GetIdea(ctx, id); err == nil {
			k.ideas[fr.field] = i
		}
	default:
		return fmt.Errorf("check %s: unexpected ref kind %q", fr.field, fr.kind)
	}
	if errors.Is(err, store.ErrNotFound) {
		k.c.add(fr.field, "%s %d does not exist", fr.kind, id)
		return nil
	}
	if err != nil {
		return fmt.Errorf("check %s: %w", fr.field, err)
	}
	k.ids[fr.field] = id
	return nil
}

// rules checks the cross-row rules. Each only runs when the rows it needs
// loaded; a missing row is already reported.
func (k *refChecker) rules(ctx context.Context, p Payload) error {
	switch p := p.(type) {
	case CreateTask:
		k.sameProject("parent_task_id", "parent task", "project_id")
	case CreateBlocker:
		k.sameProject("task_id", "task", "project_id")
	case UpdateBlocker:
		b, okB := k.blockers["blocker_id"]
		t, okT := k.tasks["task_id"]
		if okB && okT && t.ProjectID != b.ProjectID {
			k.c.add("task_id", "task %d is in another project than the blocker", t.ID)
		}
	case AddDependency:
		t, okT := k.tasks["task_id"]
		dep, okD := k.tasks["blocked_by_task_id"]
		switch {
		case !okT || !okD:
		case t.ID == dep.ID:
			k.c.add("blocked_by_task_id", "a task can't block itself")
		case t.ProjectID != dep.ProjectID:
			k.c.add("blocked_by_task_id", "task %d is in another project", dep.ID)
		}
	case UpdateIdea:
		// Promoting happens once: a second promotion, say from a double
		// click, would create the task or project again.
		i, ok := k.ideas["idea_id"]
		switch {
		case !ok || p.Status == nil:
		case i.Status == domain.IdeaPromoted:
			k.c.add("status", "the idea was already promoted")
		case *p.Status == domain.IdeaPromoted && i.Status != domain.IdeaParked:
			k.c.add("status", "only a parked idea can be promoted")
		}
	case UpdateTask:
		t, okT := k.tasks["task_id"]
		parent, okP := k.tasks["parent_task_id"]
		if !okT || !okP {
			return nil
		}
		if parent.ProjectID != t.ProjectID {
			k.c.add("parent_task_id", "task %d is in another project", parent.ID)
			return nil
		}
		return k.noCycle(ctx, t, parent)
	}
	return nil
}

// sameProject reports a problem if the task loaded from taskField isn't in
// the project whose ID was resolved from projectField.
func (k *refChecker) sameProject(taskField, what, projectField string) {
	t, okT := k.tasks[taskField]
	projectID, okP := k.ids[projectField]
	if okT && okP && t.ProjectID != projectID {
		k.c.add(taskField, "%s %d is in another project", what, t.ID)
	}
}

// noCycle reports a problem if making parent the parent of t would put t
// among its own ancestors.
func (k *refChecker) noCycle(ctx context.Context, t, parent domain.Task) error {
	cur := parent
	for range maxTaskDepth {
		if cur.ID == t.ID {
			k.c.add("parent_task_id", "would make task %d its own ancestor", t.ID)
			return nil
		}
		if cur.ParentTaskID == nil {
			return nil
		}
		next, err := k.r.GetTask(ctx, *cur.ParentTaskID)
		if err != nil {
			return fmt.Errorf("check parent_task_id: walk parents of task %d: %w", t.ID, err)
		}
		cur = next
	}
	return fmt.Errorf("check parent_task_id: parents of task %d go deeper than %d", t.ID, maxTaskDepth)
}
