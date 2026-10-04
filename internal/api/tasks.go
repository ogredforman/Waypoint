package api

import (
	"cmp"
	"context"
	"encoding/json"
	"net/http"
	"slices"

	"github.com/ogredforman/waypoint/internal/api/apitypes"
	"github.com/ogredforman/waypoint/internal/changeset"
	"github.com/ogredforman/waypoint/internal/domain"
	"github.com/ogredforman/waypoint/internal/store"
)

// createTask takes an apitypes.CreateTaskRequest. New tasks start as todo,
// so another status is set by a set_task_status in the same changeset.
func (h *handlers) createTask(w http.ResponseWriter, r *http.Request) {
	projectID, err := h.projectFromPath(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	body, err := readObject(w, r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	status, hasStatus := take(body, "status")
	create, err := opFrom(domain.OpCreateTask, body, map[string]any{"project_id": projectID})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	ops := []changeset.Payload{create}
	if hasStatus && !isJSONString(status, string(domain.TaskTodo)) {
		op, err := opFrom(domain.OpSetTaskStatus, map[string]json.RawMessage{"status": status}, map[string]any{"task_id": changeset.Temp(1)})
		if err != nil {
			h.fail(w, r, err)
			return
		}
		ops = append(ops, op)
	}
	res, err := h.applier.ApplyManual(r.Context(), ops...)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	t, err := h.store.GetTask(r.Context(), res.Created[1])
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, taskDTO(t))
}

// updateTask takes an apitypes.UpdateTaskRequest: an update_task for the
// fields other than status, then a set_task_status, as one changeset.
func (h *handlers) updateTask(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if _, err := h.store.GetTask(r.Context(), id); err != nil {
		h.fail(w, r, err)
		return
	}
	body, err := readObject(w, r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	status, hasStatus := take(body, "status")
	var ops []changeset.Payload
	// An empty body still goes to update_task, which says it changes nothing.
	if len(body) > 0 || !hasStatus {
		op, err := opFrom(domain.OpUpdateTask, body, map[string]any{"task_id": id})
		if err != nil {
			h.fail(w, r, err)
			return
		}
		ops = append(ops, op)
	}
	if hasStatus {
		op, err := opFrom(domain.OpSetTaskStatus, map[string]json.RawMessage{"status": status}, map[string]any{"task_id": id})
		if err != nil {
			h.fail(w, r, err)
			return
		}
		ops = append(ops, op)
	}
	if _, err := h.applier.ApplyManual(r.Context(), ops...); err != nil {
		h.fail(w, r, err)
		return
	}
	t, err := h.store.GetTask(r.Context(), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, taskDTO(t))
}

func (h *handlers) deleteTask(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if _, err := h.store.GetTask(r.Context(), id); err != nil {
		h.fail(w, r, err)
		return
	}
	if _, err := h.applier.ApplyManual(r.Context(), changeset.DeleteTask{TaskID: changeset.ID(id)}); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handlers) getTask(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var detail apitypes.TaskDetail
	err = h.store.InReadTx(r.Context(), func(tx *store.ReadTx) error {
		t, err := tx.GetTask(r.Context(), id)
		if err != nil {
			return err
		}
		detail.Task = taskDTO(t)
		if t.OriginEntryID != nil {
			e, err := tx.GetEntry(r.Context(), *t.OriginEntryID)
			if err != nil {
				return err
			}
			origin := entryDTO(e)
			detail.OriginEntry = &origin
		}
		detail.Observations, err = linkedObservations(r.Context(), tx, domain.TargetTask, id)
		if err != nil {
			return err
		}
		if detail.Blockers, err = taskBlockers(r.Context(), tx, t); err != nil {
			return err
		}
		detail.Steps, err = taskSteps(r.Context(), tx, t)
		return err
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

// taskBlockers returns the open blockers on the task, oldest first.
func taskBlockers(ctx context.Context, tx *store.ReadTx, t domain.Task) ([]apitypes.Blocker, error) {
	bs, err := tx.ListBlockersByProject(ctx, t.ProjectID)
	if err != nil {
		return nil, err
	}
	out := []apitypes.Blocker{}
	for _, b := range bs {
		if b.Status == domain.BlockerOpen && b.TaskID != nil && *b.TaskID == t.ID {
			out = append(out, blockerDTO(b))
		}
	}
	return out, nil
}

// taskSteps returns the task's direct subtasks, by position (ties by id).
func taskSteps(ctx context.Context, tx *store.ReadTx, t domain.Task) ([]apitypes.Task, error) {
	ts, err := tx.ListTasksByProject(ctx, t.ProjectID)
	if err != nil {
		return nil, err
	}
	out := []apitypes.Task{}
	for _, x := range ts {
		if x.ParentTaskID != nil && *x.ParentTaskID == t.ID {
			out = append(out, taskDTO(x))
		}
	}
	return out, nil
}

// linkedObservations returns the observations linked to a target, oldest
// first. Observations from an entry's earlier runs are left out: once an
// entry is reprocessed, only its latest run describes it.
func linkedObservations(ctx context.Context, tx *store.ReadTx, targetType domain.TargetType, targetID int64) ([]apitypes.LinkedObservation, error) {
	links, err := tx.ListLinksByTarget(ctx, targetType, targetID)
	if err != nil {
		return nil, err
	}
	type entryInfo struct {
		entry  domain.Entry
		latest map[int64]bool // observation IDs of the entry's latest run
	}
	entries := map[int64]*entryInfo{}
	out := []apitypes.LinkedObservation{}
	for _, l := range links {
		o, err := tx.GetObservation(ctx, l.ObservationID)
		if err != nil {
			return nil, err
		}
		info, ok := entries[o.EntryID]
		if !ok {
			e, err := tx.GetEntry(ctx, o.EntryID)
			if err != nil {
				return nil, err
			}
			all, err := tx.ListObservationsByEntry(ctx, o.EntryID)
			if err != nil {
				return nil, err
			}
			info = &entryInfo{entry: e, latest: map[int64]bool{}}
			for _, lo := range domain.LatestRun(all) {
				info.latest[lo.ID] = true
			}
			entries[o.EntryID] = info
		}
		if !info.latest[o.ID] {
			continue
		}
		out = append(out, apitypes.LinkedObservation{
			ID:             o.ID,
			EntryID:        o.EntryID,
			EntryCreatedAt: info.entry.CreatedAt,
			Kind:           o.Kind,
			Summary:        o.Summary,
			Quote:          quote(info.entry.Body, o),
			Confidence:     l.Confidence,
		})
	}
	return out, nil
}

// moveTask takes an apitypes.MoveTaskRequest and swaps the task's position
// with its neighbour among its siblings (see movePositions), as one changeset. It returns
// the tasks it moved: none when the task is already at that end, so a
// repeated click on a stale screen is harmless. The neighbour is read just
// before the apply, not in its transaction; with a single user that's fine.
func (h *handlers) moveTask(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	t, err := h.store.GetTask(r.Context(), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var req apitypes.MoveTaskRequest
	if err := readJSON(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	if req.Direction != apitypes.MoveDirectionUp && req.Direction != apitypes.MoveDirectionDown {
		h.fail(w, r, invalid("move", "direction", "must be up or down"))
		return
	}
	tasks, err := h.store.ListTasksByProject(r.Context(), t.ProjectID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	ops := movePositions(tasks, t, req.Direction)
	out := []apitypes.Task{}
	if len(ops) == 0 {
		writeJSON(w, http.StatusOK, out)
		return
	}
	payloads := make([]changeset.Payload, len(ops))
	for i, op := range ops {
		payloads[i] = op
	}
	if _, err := h.applier.ApplyManual(r.Context(), payloads...); err != nil {
		h.fail(w, r, err)
		return
	}
	for _, op := range ops {
		moved, err := h.store.GetTask(r.Context(), op.TaskID.ID)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		out = append(out, taskDTO(moved))
	}
	writeJSON(w, http.StatusOK, out)
}

// movePositions returns the position changes that move t past its
// neighbour among its siblings (ordered by position, then id): the two
// positions swap. Siblings share t's parent; top-level tasks must also share
// its status (the Project screen groups them by status), while a task's
// steps are one checklist whatever their status. Equal positions can't swap, so t alone steps one past
// the neighbour. It returns nil when t is already at that end.
func movePositions(tasks []domain.Task, t domain.Task, dir apitypes.MoveDirection) []changeset.UpdateTask {
	var group []domain.Task
	for _, x := range tasks {
		if sameParent(x, t) && (t.ParentTaskID != nil || x.Status == t.Status) {
			group = append(group, x)
		}
	}
	slices.SortStableFunc(group, func(a, b domain.Task) int {
		return cmp.Or(cmp.Compare(a.Position, b.Position), cmp.Compare(a.ID, b.ID))
	})
	i := slices.IndexFunc(group, func(x domain.Task) bool { return x.ID == t.ID })
	j := i + 1
	if dir == apitypes.MoveDirectionUp {
		j = i - 1
	}
	if i < 0 || j < 0 || j >= len(group) {
		return nil
	}
	n := group[j]
	if t.Position == n.Position {
		pos := n.Position + 1
		if dir == apitypes.MoveDirectionUp {
			pos = n.Position - 1
		}
		return []changeset.UpdateTask{{TaskID: changeset.ID(t.ID), Position: &pos}}
	}
	tPos, nPos := n.Position, t.Position
	return []changeset.UpdateTask{
		{TaskID: changeset.ID(t.ID), Position: &tPos},
		{TaskID: changeset.ID(n.ID), Position: &nPos},
	}
}

func sameParent(a, b domain.Task) bool {
	if a.ParentTaskID == nil || b.ParentTaskID == nil {
		return a.ParentTaskID == nil && b.ParentTaskID == nil
	}
	return *a.ParentTaskID == *b.ParentTaskID
}
