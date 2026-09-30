package api

import (
	"net/http"
	"slices"

	"github.com/ogredforman/waypoint/internal/api/apitypes"
	"github.com/ogredforman/waypoint/internal/changeset"
	"github.com/ogredforman/waypoint/internal/domain"
)

// listIdeas lists the ideas with status= (default parked), all of them or
// only those of project_id=, oldest first.
func (h *handlers) listIdeas(w http.ResponseWriter, r *http.Request) {
	status := domain.IdeaParked
	if s := r.URL.Query().Get("status"); s != "" {
		status = domain.IdeaStatus(s)
		if !status.Valid() {
			h.fail(w, r, badRequestf("query parameter status is not an idea status"))
			return
		}
	}
	projectID, err := queryID(r, "project_id")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var ideas []domain.Idea
	if projectID == nil {
		ideas, err = h.store.ListIdeasByStatus(r.Context(), status)
	} else {
		if _, err := h.store.GetProject(r.Context(), *projectID); err != nil {
			h.fail(w, r, err)
			return
		}
		ideas, err = h.store.ListIdeasByProject(r.Context(), *projectID)
		ideas = slices.DeleteFunc(ideas, func(i domain.Idea) bool { return i.Status != status })
	}
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, mapSlice(ideas, ideaDTO))
}

// updateIdea takes an apitypes.UpdateIdeaRequest.
func (h *handlers) updateIdea(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if _, err := h.store.GetIdea(r.Context(), id); err != nil {
		h.fail(w, r, err)
		return
	}
	body, err := readObject(w, r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if status, ok := body["status"]; ok && isJSONString(status, string(domain.IdeaPromoted)) {
		h.fail(w, r, invalid("idea", "status", "promote the idea instead, so it becomes a task or project"))
		return
	}
	op, err := opFrom(domain.OpUpdateIdea, body, map[string]any{"idea_id": id})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if _, err := h.applier.ApplyManual(r.Context(), op); err != nil {
		h.fail(w, r, err)
		return
	}
	i, err := h.store.GetIdea(r.Context(), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ideaDTO(i))
}

func (h *handlers) deleteIdea(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if _, err := h.store.GetIdea(r.Context(), id); err != nil {
		h.fail(w, r, err)
		return
	}
	if _, err := h.applier.ApplyManual(r.Context(), changeset.DeleteIdea{IdeaID: changeset.ID(id)}); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// promoteIdea turns a parked idea into a task or a parked project, taking
// its title and using its body as the task's notes or the project's why. The
// new row and the idea's status change are one changeset.
func (h *handlers) promoteIdea(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	idea, err := h.store.GetIdea(r.Context(), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var req apitypes.PromoteIdeaRequest
	if err := readJSON(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	if idea.Status != domain.IdeaParked {
		h.fail(w, r, invalid("idea", "status", "only a parked idea can be promoted"))
		return
	}
	promoted := domain.IdeaPromoted
	markPromoted := changeset.UpdateIdea{IdeaID: changeset.ID(id), Status: &promoted}
	var create changeset.Payload
	switch req.To {
	case apitypes.PromoteTargetTask:
		projectID := req.ProjectID
		if projectID == nil {
			projectID = idea.ProjectID
		}
		if projectID == nil {
			h.fail(w, r, invalid("promotion", "project_id", "is required: the idea isn't tied to a project"))
			return
		}
		create = changeset.CreateTask{ProjectID: changeset.ID(*projectID), Title: idea.Title, Notes: idea.Body}
	case apitypes.PromoteTargetProject:
		if req.ProjectID != nil {
			h.fail(w, r, invalid("promotion", "project_id", "is only for promoting to a task"))
			return
		}
		create = changeset.CreateProject{Title: idea.Title, Why: idea.Body}
	default:
		h.fail(w, r, invalid("promotion", "to", "must be task or project"))
		return
	}
	res, err := h.applier.ApplyManual(r.Context(), create, markPromoted)
	if err != nil {
		h.fail(w, r, err)
		return
	}

	var out apitypes.PromoteIdeaResponse
	if idea, err = h.store.GetIdea(r.Context(), id); err != nil {
		h.fail(w, r, err)
		return
	}
	out.Idea = ideaDTO(idea)
	if req.To == apitypes.PromoteTargetTask {
		t, err := h.store.GetTask(r.Context(), res.Created[1])
		if err != nil {
			h.fail(w, r, err)
			return
		}
		dto := taskDTO(t)
		out.Task = &dto
	} else {
		p, err := h.store.GetProject(r.Context(), res.Created[1])
		if err != nil {
			h.fail(w, r, err)
			return
		}
		dto := projectDTO(p)
		out.Project = &dto
	}
	writeJSON(w, http.StatusOK, out)
}
