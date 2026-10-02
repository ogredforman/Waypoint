package api

import (
	"encoding/json"
	"net/http"

	"github.com/ogredforman/waypoint/internal/changeset"
	"github.com/ogredforman/waypoint/internal/domain"
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
