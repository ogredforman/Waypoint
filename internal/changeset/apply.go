package changeset

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/ogredforman/waypoint/internal/domain"
	"github.com/ogredforman/waypoint/internal/store"
)

// Action is what the user decided for one op of a changeset under review.
type Action string

const (
	Accept Action = "accept"
	Reject Action = "reject"
	Edit   Action = "edit" // apply Decision.Payload instead of the proposed payload
)

// Decision is the user's call on one op. Payload is the edited payload, set
// only for Edit; the op type stays the same.
type Decision struct {
	Action  Action
	Payload json.RawMessage
}

// Result is what Apply did.
type Result struct {
	// Changeset is the applied changeset, now resolved.
	Changeset domain.Changeset
	// Created maps the position of each applied creating op to the new row.
	Created TempIDs
	// SplitOff is the new pending changeset holding the ops that weren't
	// decided or were held back by the active-project cap, or nil if none were.
	SplitOff *domain.Changeset
	// HeldByCap lists the positions of the ops moved to SplitOff because the
	// active-project cap is reached.
	HeldByCap []int64
	// Projects lists, in ascending order, the projects whose own row, tasks,
	// or blockers an applied op changed: those whose briefing may be stale.
	// Ideas don't count, since a briefing doesn't show them.
	Projects []int64
}

// ErrNotPending is returned when applying a changeset that was already
// resolved.
var ErrNotPending = errors.New("changeset is not pending")

// OpError is a failure applying one op. Err is a *domain.ValidationError when
// the op doesn't fit the current state, a *domain.ActiveCapError when it
// would go over the active-project cap, or a store error.
type OpError struct {
	Position int64
	OpType   domain.OpType
	Err      error
}

func (e *OpError) Error() string {
	return fmt.Sprintf("op at position %d (%s): %v", e.Position, e.OpType, e.Err)
}

func (e *OpError) Unwrap() error { return e.Err }

// AfterApply is told about every changeset applied, after its transaction
// commits, such as to refresh the briefings of res.Projects. Applied runs on
// the caller's goroutine, so it should be quick; it has no way to fail the
// apply, which is already committed. Its context is never cancelled, so a
// caller that goes away right after the commit doesn't cut it short.
type AfterApply interface {
	Applied(ctx context.Context, res Result)
}

// AfterApplies is an AfterApply that calls each of its hooks in order.
type AfterApplies []AfterApply

// Applied calls Applied on each hook in turn.
func (hs AfterApplies) Applied(ctx context.Context, res Result) {
	for _, h := range hs {
		h.Applied(ctx, res)
	}
}

// Applier applies changesets. It is the only writer of projects, tasks,
// blockers, and ideas.
type Applier struct {
	store     *store.Store
	activeCap int
	after     AfterApply
}

// NewApplier returns an Applier over s that enforces activeCap on projects
// becoming active and tells after about each apply. after may be nil.
func NewApplier(s *store.Store, activeCap int, after AfterApply) *Applier {
	return &Applier{store: s, activeCap: activeCap, after: after}
}

// applied tells the AfterApply hook, if any, about res.
func (a *Applier) applied(ctx context.Context, res Result) {
	if a.after != nil {
		a.after.Applied(context.WithoutCancel(ctx), res)
	}
}

// Apply applies pending changeset id in one transaction, following the user's
// decisions, keyed by op position (ADR 0003):
//
//   - Accepted and edited ops are applied in position order; rejected ops are
//     marked rejected. The changeset is resolved as applied, rejected, or
//     partially_applied.
//   - Ops without a decision move to a new pending changeset split off this
//     one, as do ops of an extraction changeset that would go over the
//     active-project cap, with the user's edit if there was one. Temp IDs
//     of rows created before the split are rewritten to the real IDs.
//
// Ops apply in position order, so the cap is checked against the state at
// that op's position: making C active before parking B is held back even if
// B is parked later in the same changeset.
//
// At least one op must be decided. An op that isn't rejected can't use what a
// rejected op creates, and an applied op can't use what an undecided op
// creates. Any failure rolls the whole apply back: problems with the decisions
// come back as a *domain.ValidationError, and a failed op as an *OpError. On
// a manual changeset, going over the cap is such a failure.
func (a *Applier) Apply(ctx context.Context, id int64, decisions map[int64]Decision) (Result, error) {
	var res Result
	err := a.store.InTx(ctx, func(tx *store.Tx) error {
		var err error
		res, err = a.apply(ctx, tx, id, decisions)
		return err
	})
	if err != nil {
		return Result{}, err
	}
	a.applied(ctx, res)
	return res, nil
}

func (a *Applier) apply(ctx context.Context, tx *store.Tx, id int64, decisions map[int64]Decision) (Result, error) {
	cs, err := tx.GetChangeset(ctx, id)
	if err != nil {
		return Result{}, fmt.Errorf("apply changeset: %w", err)
	}
	if cs.Status != domain.ChangesetPending {
		return Result{}, fmt.Errorf("apply changeset %d: %w", id, ErrNotPending)
	}
	ops, err := tx.ListChangeOps(ctx, id)
	if err != nil {
		return Result{}, fmt.Errorf("apply changeset %d: %w", id, err)
	}
	effective, payloads, err := checkDecisions(ops, decisions)
	if err != nil {
		return Result{}, err
	}
	entryID, err := tx.ChangesetEntryID(ctx, cs)
	if err != nil {
		return Result{}, fmt.Errorf("apply changeset %d: %w", id, err)
	}

	w := writer{tx: tx, entryID: entryID, activeCap: a.activeCap, ids: TempIDs{}, projects: map[int64]bool{}}
	res := Result{Created: w.ids}
	var carried []int // indexes into ops, in position order
	var applied, rejected int
	var capErr *OpError
	for i, op := range ops {
		d, ok := decisions[op.Position]
		if !ok {
			carried = append(carried, i)
			continue
		}
		if d.Action == Reject {
			if _, err := tx.SetChangeOpResult(ctx, op.ID, domain.OpRejected, nil); err != nil {
				return Result{}, fmt.Errorf("apply changeset %d: %w", id, err)
			}
			rejected++
			continue
		}

		p := payloads[i]
		opErr := func(err error) *OpError { return &OpError{Position: op.Position, OpType: op.OpType, Err: err} }
		if err := CheckRefs(ctx, tx, p, w.ids); err != nil {
			return Result{}, opErr(err)
		}
		deleted, err := w.apply(ctx, op.Position, p)
		var ce *domain.ActiveCapError
		if errors.As(err, &ce) && cs.Source == domain.SourceExtraction {
			carried = append(carried, i)
			res.HeldByCap = append(res.HeldByCap, op.Position)
			if capErr == nil {
				capErr = opErr(err)
			}
			continue
		}
		if err != nil {
			return Result{}, opErr(err)
		}

		status, appliedPayload := domain.OpApplied, json.RawMessage(nil)
		if d.Action == Edit {
			status, appliedPayload = domain.OpEdited, effective[i].Payload
		}
		if deleted != nil {
			if appliedPayload, err = withField(effective[i].Payload, "deleted", deleted); err != nil {
				return Result{}, opErr(err)
			}
		}
		if _, err := tx.SetChangeOpResult(ctx, op.ID, status, appliedPayload); err != nil {
			return Result{}, fmt.Errorf("apply changeset %d: %w", id, err)
		}
		applied++
	}

	// Everything decided was held back by the cap: splitting would just move
	// the whole changeset, so report the cap instead and leave it pending.
	if applied == 0 && rejected == 0 && capErr != nil {
		return Result{}, capErr
	}

	status := domain.ChangesetPartiallyApplied
	switch {
	case rejected == 0:
		status = domain.ChangesetApplied
	case applied == 0:
		status = domain.ChangesetRejected
	}
	if res.Changeset, err = tx.ResolveChangeset(ctx, id, status); err != nil {
		return Result{}, fmt.Errorf("apply changeset %d: %w", id, err)
	}

	if len(carried) > 0 {
		child, _, err := tx.CreateChangeset(ctx, domain.Changeset{
			Source:            cs.Source,
			ExtractionRunID:   cs.ExtractionRunID,
			ParentChangesetID: &cs.ID,
			Status:            domain.ChangesetPending,
		}, nil)
		if err != nil {
			return Result{}, fmt.Errorf("apply changeset %d: split off: %w", id, err)
		}
		for _, i := range carried {
			payload, err := resolveTemps(effective[i].Payload, payloads[i], w.ids)
			if err != nil {
				return Result{}, fmt.Errorf("apply changeset %d: split off op at position %d: %w", id, ops[i].Position, err)
			}
			if _, err := tx.MoveChangeOp(ctx, ops[i].ID, child.ID, payload); err != nil {
				return Result{}, fmt.Errorf("apply changeset %d: split off: %w", id, err)
			}
		}
		res.SplitOff = &child
	}
	for id := range w.projects {
		res.Projects = append(res.Projects, id)
	}
	slices.Sort(res.Projects)
	return res, nil
}

// opState is where an op stands under the user's decisions.
type opState int

const (
	undecided opState = iota
	applying
	rejecting
)

// checkDecisions checks the decisions against the changeset's ops, then
// validates the ops as they'll be applied, with edited payloads in place. It
// returns those ops and their decoded payloads.
func checkDecisions(ops []domain.ChangeOp, decisions map[int64]Decision) ([]domain.ChangeOp, []Payload, error) {
	var c checker
	if len(decisions) == 0 {
		c.add("decisions", "must decide at least one op")
	}
	byPos := map[int64]int{}
	for i, op := range ops {
		if op.Status != domain.OpPending {
			return nil, nil, fmt.Errorf("op at position %d is %s in a pending changeset", op.Position, op.Status)
		}
		byPos[op.Position] = i
	}
	positions := make([]int64, 0, len(decisions))
	for pos := range decisions {
		positions = append(positions, pos)
	}
	slices.Sort(positions)

	effective := slices.Clone(ops)
	state := make(map[int64]opState, len(ops))
	for _, pos := range positions {
		d := decisions[pos]
		field := fmt.Sprintf("decisions[%d]", pos)
		i, ok := byPos[pos]
		if !ok {
			c.add(field, "no op has this position")
			continue
		}
		switch d.Action {
		case Accept, Reject:
			if d.Payload != nil {
				c.add(field, "only an edit has a payload")
			}
		case Edit:
			if len(d.Payload) == 0 {
				c.add(field, "an edit needs a payload")
			}
			effective[i].Payload = d.Payload
		default:
			c.add(field, "is not an allowed action")
		}
		state[pos] = applying
		if d.Action == Reject {
			state[pos] = rejecting
		}
	}
	if err := c.errEntity("decisions"); err != nil {
		return nil, nil, err
	}

	payloads, err := ValidateOps(effective)
	if err != nil {
		return nil, nil, err
	}

	// ValidateOps has checked that every temp ref names an earlier creating op.
	for i, p := range payloads {
		pos := ops[i].Position
		if state[pos] == rejecting {
			continue
		}
		for _, r := range p.refs() {
			if !r.ref.IsTemp() {
				continue
			}
			field := fmt.Sprintf("decisions[%d]", pos)
			switch state[r.ref.Temp] {
			case rejecting:
				c.add(field, "%s uses what the op at position %d creates, which is rejected", r.field, r.ref.Temp)
			case undecided:
				if state[pos] == applying {
					c.add(field, "%s uses what the op at position %d creates, which isn't decided", r.field, r.ref.Temp)
				}
			}
		}
	}
	if err := c.errEntity("decisions"); err != nil {
		return nil, nil, err
	}
	return effective, payloads, nil
}

// resolveTemps rewrites the temp refs in raw whose rows were created, so the
// op still means the same thing after it moves to another changeset. Temp
// refs to ops that moved with it stay as they are.
func resolveTemps(raw json.RawMessage, p Payload, ids TempIDs) (json.RawMessage, error) {
	var fields map[string]json.RawMessage
	for _, r := range p.refs() {
		if !r.ref.IsTemp() {
			continue
		}
		id, ok := ids[r.ref.Temp]
		if !ok {
			continue
		}
		if fields == nil {
			if err := json.Unmarshal(raw, &fields); err != nil {
				return nil, fmt.Errorf("rewrite temp IDs: %w", err)
			}
		}
		b, err := json.Marshal(id)
		if err != nil {
			return nil, err
		}
		fields[r.field] = b
	}
	if fields == nil {
		return raw, nil
	}
	return json.Marshal(fields)
}

// withField returns the JSON object raw with field name set to v.
func withField(raw json.RawMessage, name string, v any) (json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, fmt.Errorf("add %s to payload: %w", name, err)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("add %s to payload: %w", name, err)
	}
	fields[name] = b
	return json.Marshal(fields)
}

// writer applies ops inside the apply transaction.
type writer struct {
	tx        *store.Tx
	entryID   *int64 // the entry an extraction changeset came from; nil for manual
	activeCap int
	ids       TempIDs
	// projects collects the projects applied ops changed (Result.Projects).
	projects map[int64]bool
}

// id resolves r. CheckRefs has already checked that it resolves.
func (w writer) id(r Ref) int64 {
	id, _ := w.ids.Resolve(r)
	return id
}

func (w writer) optID(r *Ref) *int64 {
	if r == nil {
		return nil
	}
	id := w.id(*r)
	return &id
}

// apply applies p, the op at position pos. For a delete op it returns the row
// as it was, to record in the op's applied payload.
func (w writer) apply(ctx context.Context, pos int64, p Payload) (deleted any, err error) {
	tx := w.tx
	switch p := p.(type) {
	case CreateTask:
		t, err := tx.CreateTask(ctx, domain.Task{
			ProjectID:     w.id(p.ProjectID),
			ParentTaskID:  w.optID(p.ParentTaskID),
			Title:         p.Title,
			Notes:         p.Notes,
			Status:        domain.TaskTodo,
			Size:          p.Size,
			Energy:        p.Energy,
			OriginEntryID: w.entryID,
		})
		w.ids[pos] = t.ID
		if err != nil {
			return nil, err
		}
		w.projects[t.ProjectID] = true
		return nil, nil

	case UpdateTask:
		t, err := tx.GetTask(ctx, w.id(p.TaskID))
		if err != nil {
			return nil, err
		}
		if p.Title != nil {
			t.Title = *p.Title
		}
		if p.Notes != nil {
			t.Notes = *p.Notes
		}
		if p.Size.Set {
			t.Size = p.Size.Value
		}
		if p.Energy.Set {
			t.Energy = p.Energy.Value
		}
		if p.ParentTaskID.Set {
			t.ParentTaskID = w.optID(p.ParentTaskID.Value)
		}
		if p.Position != nil {
			t.Position = *p.Position
		}
		if _, err = tx.UpdateTask(ctx, t); err != nil {
			return nil, err
		}
		w.projects[t.ProjectID] = true
		return nil, nil

	case SetTaskStatus:
		t, err := tx.GetTask(ctx, w.id(p.TaskID))
		if err != nil {
			return nil, err
		}
		t.Status = p.Status
		if _, err = tx.UpdateTask(ctx, t); err != nil {
			return nil, err
		}
		w.projects[t.ProjectID] = true
		return nil, nil

	case SplitTask:
		parent, err := tx.GetTask(ctx, w.id(p.TaskID))
		if err != nil {
			return nil, err
		}
		for _, s := range p.Subtasks {
			_, err := tx.CreateTask(ctx, domain.Task{
				ProjectID:     parent.ProjectID,
				ParentTaskID:  &parent.ID,
				Title:         s.Title,
				Notes:         s.Notes,
				Status:        domain.TaskTodo,
				Size:          s.Size,
				Energy:        s.Energy,
				OriginEntryID: w.entryID,
			})
			if err != nil {
				return nil, err
			}
		}
		w.projects[parent.ProjectID] = true
		return nil, nil

	case AddDependency:
		t, err := tx.GetTask(ctx, w.id(p.TaskID))
		if err != nil {
			return nil, err
		}
		if err := tx.AddTaskDependency(ctx, t.ID, w.id(p.BlockedByTaskID)); err != nil {
			return nil, err
		}
		w.projects[t.ProjectID] = true
		return nil, nil

	case DeleteTask:
		t, err := tx.DeleteTask(ctx, w.id(p.TaskID))
		if err != nil {
			return nil, err
		}
		w.projects[t.ProjectID] = true
		return deletedTaskOf(t), nil

	case CreateBlocker:
		b, err := tx.CreateBlocker(ctx, domain.Blocker{
			ProjectID:     w.id(p.ProjectID),
			TaskID:        w.optID(p.TaskID),
			Description:   p.Description,
			Status:        domain.BlockerOpen,
			OriginEntryID: w.entryID,
		})
		w.ids[pos] = b.ID
		if err != nil {
			return nil, err
		}
		w.projects[b.ProjectID] = true
		return nil, nil

	case UpdateBlocker:
		b, err := tx.GetBlocker(ctx, w.id(p.BlockerID))
		if err != nil {
			return nil, err
		}
		if p.Description != nil {
			b.Description = *p.Description
		}
		if p.TaskID.Set {
			b.TaskID = w.optID(p.TaskID.Value)
		}
		if _, err = tx.UpdateBlocker(ctx, b); err != nil {
			return nil, err
		}
		w.projects[b.ProjectID] = true
		return nil, nil

	case ResolveBlocker:
		b, err := tx.GetBlocker(ctx, w.id(p.BlockerID))
		if err != nil {
			return nil, err
		}
		b.Status = domain.BlockerResolved
		if w.entryID != nil {
			b.ResolvedEntryID = w.entryID
		}
		if _, err = tx.UpdateBlocker(ctx, b); err != nil {
			return nil, err
		}
		w.projects[b.ProjectID] = true
		return nil, nil

	case DeleteBlocker:
		b, err := tx.DeleteBlocker(ctx, w.id(p.BlockerID))
		if err != nil {
			return nil, err
		}
		w.projects[b.ProjectID] = true
		return deletedBlockerOf(b), nil

	case ParkIdea:
		i, err := tx.CreateIdea(ctx, domain.Idea{
			ProjectID:     w.optID(p.ProjectID),
			Title:         p.Title,
			Body:          p.Body,
			Status:        domain.IdeaParked,
			OriginEntryID: w.entryID,
		})
		w.ids[pos] = i.ID
		return nil, err

	case UpdateIdea:
		i, err := tx.GetIdea(ctx, w.id(p.IdeaID))
		if err != nil {
			return nil, err
		}
		if p.Title != nil {
			i.Title = *p.Title
		}
		if p.Body != nil {
			i.Body = *p.Body
		}
		if p.ProjectID.Set {
			i.ProjectID = w.optID(p.ProjectID.Value)
		}
		if p.Status != nil {
			i.Status = *p.Status
		}
		_, err = tx.UpdateIdea(ctx, i)
		return nil, err

	case DeleteIdea:
		i, err := tx.DeleteIdea(ctx, w.id(p.IdeaID))
		if err != nil {
			return nil, err
		}
		return deletedIdeaOf(i), nil

	case CreateProject:
		// Always created parked (SPEC §6.2), so the active-project cap
		// doesn't apply.
		pr, err := tx.CreateProject(ctx, domain.Project{
			Title:  p.Title,
			Why:    p.Why,
			Status: domain.ProjectParked,
		})
		w.ids[pos] = pr.ID
		if err != nil {
			return nil, err
		}
		w.projects[pr.ID] = true
		return nil, nil

	case UpdateProject:
		pr, err := tx.GetProject(ctx, w.id(p.ProjectID))
		if err != nil {
			return nil, err
		}
		if p.Status != nil && *p.Status != pr.Status {
			active, err := tx.CountActiveProjects(ctx)
			if err != nil {
				return nil, err
			}
			if err := domain.CheckProjectStatusChange(pr.Status, *p.Status, active, w.activeCap); err != nil {
				return nil, err
			}
			pr.Status = *p.Status
		}
		if p.Title != nil {
			pr.Title = *p.Title
		}
		if p.Why != nil {
			pr.Why = *p.Why
		}
		if p.DefinitionOfDone != nil {
			pr.DefinitionOfDone = *p.DefinitionOfDone
		}
		if _, err = tx.UpdateProject(ctx, pr); err != nil {
			return nil, err
		}
		w.projects[pr.ID] = true
		return nil, nil
	}
	return nil, fmt.Errorf("apply: unexpected payload %T", p)
}

// Deleted rows as recorded in a delete op's applied payload (ADR 0002), so
// history stays readable after the row is gone.

type deletedTask struct {
	ID            int64             `json:"id"`
	ProjectID     int64             `json:"project_id"`
	ParentTaskID  *int64            `json:"parent_task_id"`
	Title         string            `json:"title"`
	Notes         string            `json:"notes"`
	Status        domain.TaskStatus `json:"status"`
	Size          *domain.TaskSize  `json:"size"`
	Energy        *domain.Energy    `json:"energy"`
	Position      int64             `json:"position"`
	OriginEntryID *int64            `json:"origin_entry_id"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

func deletedTaskOf(t domain.Task) deletedTask {
	return deletedTask{
		ID: t.ID, ProjectID: t.ProjectID, ParentTaskID: t.ParentTaskID, Title: t.Title, Notes: t.Notes,
		Status: t.Status, Size: t.Size, Energy: t.Energy, Position: t.Position,
		OriginEntryID: t.OriginEntryID, CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt,
	}
}

type deletedBlocker struct {
	ID              int64                `json:"id"`
	ProjectID       int64                `json:"project_id"`
	TaskID          *int64               `json:"task_id"`
	Description     string               `json:"description"`
	Status          domain.BlockerStatus `json:"status"`
	OriginEntryID   *int64               `json:"origin_entry_id"`
	ResolvedEntryID *int64               `json:"resolved_entry_id"`
	CreatedAt       time.Time            `json:"created_at"`
	UpdatedAt       time.Time            `json:"updated_at"`
}

func deletedBlockerOf(b domain.Blocker) deletedBlocker {
	return deletedBlocker{
		ID: b.ID, ProjectID: b.ProjectID, TaskID: b.TaskID, Description: b.Description, Status: b.Status,
		OriginEntryID: b.OriginEntryID, ResolvedEntryID: b.ResolvedEntryID,
		CreatedAt: b.CreatedAt, UpdatedAt: b.UpdatedAt,
	}
}

type deletedIdea struct {
	ID            int64             `json:"id"`
	ProjectID     *int64            `json:"project_id"`
	Title         string            `json:"title"`
	Body          string            `json:"body"`
	Status        domain.IdeaStatus `json:"status"`
	OriginEntryID *int64            `json:"origin_entry_id"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

func deletedIdeaOf(i domain.Idea) deletedIdea {
	return deletedIdea{
		ID: i.ID, ProjectID: i.ProjectID, Title: i.Title, Body: i.Body, Status: i.Status,
		OriginEntryID: i.OriginEntryID, CreatedAt: i.CreatedAt, UpdatedAt: i.UpdatedAt,
	}
}
