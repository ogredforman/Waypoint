package api

import (
	"cmp"
	"net/http"
	"slices"

	"github.com/ogredforman/waypoint/internal/api/apitypes"
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

// moveMilestone takes an apitypes.MoveMilestoneRequest and swaps the
// milestone's position with its neighbour in the project (see
// milestoneMovePositions), as one changeset. It returns the milestones it
// moved: none when the milestone is already at that end.
func (h *handlers) moveMilestone(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	m, err := h.store.GetMilestone(r.Context(), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var req apitypes.MoveMilestoneRequest
	if err := readJSON(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	if req.Direction != apitypes.MoveDirectionUp && req.Direction != apitypes.MoveDirectionDown {
		h.fail(w, r, invalid("move", "direction", "must be up or down"))
		return
	}
	milestones, err := h.store.ListMilestonesByProject(r.Context(), m.ProjectID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	ops := milestoneMovePositions(milestones, m, req.Direction)
	out := []apitypes.Milestone{}
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
		moved, err := h.store.GetMilestone(r.Context(), op.MilestoneID.ID)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		out = append(out, milestoneDTO(moved))
	}
	writeJSON(w, http.StatusOK, out)
}

// milestoneMovePositions returns the position changes that move m past its
// neighbour among the project's milestones (ordered by position, then id):
// the two positions swap. Equal positions can't swap, so m alone steps one
// past the neighbour. It returns nil when m is already at that end.
func milestoneMovePositions(milestones []domain.Milestone, m domain.Milestone, dir apitypes.MoveDirection) []changeset.UpdateMilestone {
	list := slices.Clone(milestones)
	slices.SortStableFunc(list, func(a, b domain.Milestone) int {
		return cmp.Or(cmp.Compare(a.Position, b.Position), cmp.Compare(a.ID, b.ID))
	})
	i := slices.IndexFunc(list, func(x domain.Milestone) bool { return x.ID == m.ID })
	j := i + 1
	if dir == apitypes.MoveDirectionUp {
		j = i - 1
	}
	if i < 0 || j < 0 || j >= len(list) {
		return nil
	}
	n := list[j]
	if m.Position == n.Position {
		pos := n.Position + 1
		if dir == apitypes.MoveDirectionUp {
			pos = n.Position - 1
		}
		return []changeset.UpdateMilestone{{MilestoneID: changeset.ID(m.ID), Position: &pos}}
	}
	mPos, nPos := n.Position, m.Position
	return []changeset.UpdateMilestone{
		{MilestoneID: changeset.ID(m.ID), Position: &mPos},
		{MilestoneID: changeset.ID(n.ID), Position: &nPos},
	}
}
