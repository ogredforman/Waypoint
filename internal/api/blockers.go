package api

import (
	"net/http"

	"github.com/ogredforman/waypoint/internal/changeset"
	"github.com/ogredforman/waypoint/internal/domain"
)

// updateBlocker takes an apitypes.UpdateBlockerRequest: an update_blocker
// for the fields other than status, then a resolve_blocker if status is
// resolved, as one changeset. Reopening isn't an op, so no other status is
// accepted.
func (h *handlers) updateBlocker(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if _, err := h.store.GetBlocker(r.Context(), id); err != nil {
		h.fail(w, r, err)
		return
	}
	body, err := readObject(w, r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	status, hasStatus := take(body, "status")
	if hasStatus && !isJSONString(status, string(domain.BlockerResolved)) {
		h.fail(w, r, invalid("blocker", "status", "can only be set to resolved"))
		return
	}
	var ops []changeset.Payload
	if len(body) > 0 || !hasStatus {
		op, err := opFrom(domain.OpUpdateBlocker, body, map[string]any{"blocker_id": id})
		if err != nil {
			h.fail(w, r, err)
			return
		}
		ops = append(ops, op)
	}
	if hasStatus {
		ops = append(ops, changeset.ResolveBlocker{BlockerID: changeset.ID(id)})
	}
	if _, err := h.applier.ApplyManual(r.Context(), ops...); err != nil {
		h.fail(w, r, err)
		return
	}
	b, err := h.store.GetBlocker(r.Context(), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, blockerDTO(b))
}

func (h *handlers) deleteBlocker(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if _, err := h.store.GetBlocker(r.Context(), id); err != nil {
		h.fail(w, r, err)
		return
	}
	if _, err := h.applier.ApplyManual(r.Context(), changeset.DeleteBlocker{BlockerID: changeset.ID(id)}); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
