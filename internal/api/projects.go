package api

import (
	"encoding/json"
	"net/http"

	"github.com/ogredforman/waypoint/internal/api/apitypes"
	"github.com/ogredforman/waypoint/internal/changeset"
	"github.com/ogredforman/waypoint/internal/domain"
	"github.com/ogredforman/waypoint/internal/store"
)

func (h *handlers) listProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := h.store.ListProjects(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, mapSlice(projects, projectDTO))
}

func (h *handlers) getProject(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	p, err := h.store.GetProject(r.Context(), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, projectDTO(p))
}

// projectDeletion says what deleting the project would take with it, for the
// confirmation.
func (h *handlers) projectDeletion(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	n, err := h.store.CountProjectContents(r.Context(), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, apitypes.ProjectDeletion{
		Tasks: n.Tasks, Blockers: n.Blockers, Ideas: n.Ideas, Milestones: n.Milestones, Entries: n.Entries,
	})
}

// deleteProject deletes the project with everything in it (ADR 0008).
// entries=keep keeps its diary entries, unscoped; entries=delete deletes them
// too. There is no default: the UI asks. The changeset hooks refresh the
// briefings and tell the clients, including about each deleted entry.
func (h *handlers) deleteProject(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var deleteEntries bool
	switch r.URL.Query().Get("entries") {
	case "keep":
	case "delete":
		deleteEntries = true
	default:
		h.fail(w, r, badRequestf("entries must be keep or delete"))
		return
	}
	if _, err := h.store.GetProject(r.Context(), id); err != nil {
		h.fail(w, r, err)
		return
	}
	op := changeset.DeleteProject{ProjectID: changeset.ID(id), DeleteEntries: deleteEntries}
	if _, err := h.applier.ApplyManual(r.Context(), op); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

const (
	defaultObservationPage = 10 // entries, not observations
	maxObservationPage     = 50
)

// listProjectObservations pages through what the diary says about a project,
// a page of entries at a time, newest first. before= continues from an
// earlier page's next_before; limit= sets how many entries a page covers.
func (h *handlers) listProjectObservations(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	beforeID, limit, err := pageQuery(r, defaultObservationPage, maxObservationPage)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var rows []store.ProjectObservation
	err = h.store.InReadTx(r.Context(), func(tx *store.ReadTx) error {
		if _, err := tx.GetProject(r.Context(), id); err != nil {
			return err
		}
		rows, err = tx.ListProjectObservationsBefore(r.Context(), id, beforeID, limit)
		return err
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	page := apitypes.ProjectObservations{Observations: make([]apitypes.LinkedObservation, len(rows))}
	entries := 0
	for i, row := range rows {
		if i == 0 || row.Observation.EntryID != rows[i-1].Observation.EntryID {
			entries++
		}
		page.Observations[i] = apitypes.LinkedObservation{
			ID:             row.Observation.ID,
			EntryID:        row.Observation.EntryID,
			EntryCreatedAt: row.EntryCreatedAt,
			Kind:           row.Observation.Kind,
			Summary:        row.Observation.Summary,
			Quote:          quote(row.EntryBody, row.Observation),
			Confidence:     row.Confidence,
		}
	}
	if entries == limit {
		next := rows[len(rows)-1].Observation.EntryID
		page.NextBefore = &next
	}
	writeJSON(w, http.StatusOK, page)
}

// createProject takes an apitypes.CreateProjectRequest. create_project
// always makes a parked project, so a definition of done or another status
// is set by an update_project in the same changeset.
func (h *handlers) createProject(w http.ResponseWriter, r *http.Request) {
	body, err := readObject(w, r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	update := map[string]json.RawMessage{}
	if v, ok := take(body, "definition_of_done"); ok {
		update["definition_of_done"] = v
	}
	if v, ok := take(body, "status"); ok && !isJSONString(v, string(domain.ProjectParked)) {
		update["status"] = v
	}
	ops := []changeset.Payload{}
	create, err := opFrom(domain.OpCreateProject, body, nil)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	ops = append(ops, create)
	if len(update) > 0 {
		op, err := opFrom(domain.OpUpdateProject, update, map[string]any{"project_id": changeset.Temp(1)})
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
	p, err := h.store.GetProject(r.Context(), res.Created[1])
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, projectDTO(p))
}

// updateProject takes an apitypes.UpdateProjectRequest.
func (h *handlers) updateProject(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if _, err := h.store.GetProject(r.Context(), id); err != nil {
		h.fail(w, r, err)
		return
	}
	body, err := readObject(w, r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	op, err := opFrom(domain.OpUpdateProject, body, map[string]any{"project_id": id})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if _, err := h.applier.ApplyManual(r.Context(), op); err != nil {
		h.fail(w, r, err)
		return
	}
	p, err := h.store.GetProject(r.Context(), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, projectDTO(p))
}

func (h *handlers) listTasks(w http.ResponseWriter, r *http.Request) {
	id, err := h.projectFromPath(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	tasks, err := h.store.ListTasksByProject(r.Context(), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, mapSlice(tasks, taskDTO))
}

func (h *handlers) listBlockers(w http.ResponseWriter, r *http.Request) {
	id, err := h.projectFromPath(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	blockers, err := h.store.ListBlockersByProject(r.Context(), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, mapSlice(blockers, blockerDTO))
}

func (h *handlers) getBriefing(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	v, err := h.briefings.Get(r.Context(), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, briefingDTO(v))
}

// refreshBriefing regenerates the project's briefing and returns it.
func (h *handlers) refreshBriefing(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if err := h.briefings.Refresh(r.Context(), id); err != nil {
		h.fail(w, r, err)
		return
	}
	v, err := h.briefings.Get(r.Context(), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, briefingDTO(v))
}

// projectFromPath returns the {id} path parameter if it names a project.
func (h *handlers) projectFromPath(r *http.Request) (int64, error) {
	id, err := pathID(r, "id")
	if err != nil {
		return 0, err
	}
	if _, err := h.store.GetProject(r.Context(), id); err != nil {
		return 0, err
	}
	return id, nil
}
