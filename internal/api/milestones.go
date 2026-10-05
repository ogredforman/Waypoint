package api

import (
	"net/http"

	"github.com/ogredforman/waypoint/internal/changeset"
	"github.com/ogredforman/waypoint/internal/domain"
)

func (h *handlers) listMilestones(w http.ResponseWriter, r *http.Request) {
	id, err := h.projectFromPath(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	milestones, err := h.store.ListMilestonesByProject(r.Context(), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, mapSlice(milestones, milestoneDTO))
}

// createMilestone takes an apitypes.CreateMilestoneRequest as one
// create_milestone.
func (h *handlers) createMilestone(w http.ResponseWriter, r *http.Request) {
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
	create, err := opFrom(domain.OpCreateMilestone, body, map[string]any{"project_id": projectID})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	res, err := h.applier.ApplyManual(r.Context(), create)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	m, err := h.store.GetMilestone(r.Context(), res.Created[1])
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, milestoneDTO(m))
}

// updateMilestone takes an apitypes.UpdateMilestoneRequest as one
// update_milestone.
func (h *handlers) updateMilestone(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if _, err := h.store.GetMilestone(r.Context(), id); err != nil {
		h.fail(w, r, err)
		return
	}
	body, err := readObject(w, r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	op, err := opFrom(domain.OpUpdateMilestone, body, map[string]any{"milestone_id": id})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if _, err := h.applier.ApplyManual(r.Context(), op); err != nil {
		h.fail(w, r, err)
		return
	}
	m, err := h.store.GetMilestone(r.Context(), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, milestoneDTO(m))
}

// deleteMilestone removes a milestone; its tasks stay with no milestone.
func (h *handlers) deleteMilestone(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if _, err := h.store.GetMilestone(r.Context(), id); err != nil {
		h.fail(w, r, err)
		return
	}
	if _, err := h.applier.ApplyManual(r.Context(), changeset.DeleteMilestone{MilestoneID: changeset.ID(id)}); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
