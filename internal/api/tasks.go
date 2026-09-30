package api

import (
	"context"
	"encoding/json"
	"net/http"

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
		return err
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
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
