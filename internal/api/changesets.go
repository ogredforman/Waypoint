package api

import (
	"bytes"
	"fmt"
	"net/http"

	"github.com/ogredforman/waypoint/internal/api/apitypes"
	"github.com/ogredforman/waypoint/internal/changeset"
	"github.com/ogredforman/waypoint/internal/domain"
	"github.com/ogredforman/waypoint/internal/store"
)

// listChangesets lists the changesets with status= (default pending), oldest
// first, with their ops.
func (h *handlers) listChangesets(w http.ResponseWriter, r *http.Request) {
	status := domain.ChangesetPending
	if s := r.URL.Query().Get("status"); s != "" {
		status = domain.ChangesetStatus(s)
		if !status.Valid() {
			h.fail(w, r, badRequestf("query parameter status is not a changeset status"))
			return
		}
	}
	ctx := r.Context()
	var out []apitypes.Changeset
	err := h.store.InReadTx(ctx, func(tx *store.ReadTx) error {
		changesets, err := tx.ListChangesetsByStatus(ctx, status)
		if err != nil {
			return err
		}
		out = make([]apitypes.Changeset, len(changesets))
		for i, cs := range changesets {
			if out[i], err = changesetDTO(ctx, tx, cs); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

var actions = map[apitypes.Decision]changeset.Action{
	apitypes.DecisionAccept: changeset.Accept,
	apitypes.DecisionReject: changeset.Reject,
	apitypes.DecisionEdit:   changeset.Edit,
}

// applyChangeset takes an apitypes.ApplyRequest, deciding ops by ID; the
// applier keys decisions by position.
func (h *handlers) applyChangeset(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	ctx := r.Context()
	if _, err := h.store.GetChangeset(ctx, id); err != nil {
		h.fail(w, r, err)
		return
	}
	var req apitypes.ApplyRequest
	if err := readJSON(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	ops, err := h.store.ListChangeOps(ctx, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	positions := make(map[int64]int64, len(ops))
	ids := make(map[int64]int64, len(ops))
	for _, op := range ops {
		positions[op.ID] = op.Position
		ids[op.Position] = op.ID
	}

	var problems []domain.FieldError
	decisions := make(map[int64]changeset.Decision, len(req.Ops))
	for i, d := range req.Ops {
		field := fmt.Sprintf("ops[%d]", i)
		pos, ok := positions[d.ID]
		if !ok {
			problems = append(problems, domain.FieldError{Field: field + ".id", Problem: "is not an op of this changeset"})
			continue
		}
		if _, dup := decisions[pos]; dup {
			problems = append(problems, domain.FieldError{Field: field + ".id", Problem: "is decided more than once"})
			continue
		}
		action, ok := actions[d.Decision]
		if !ok {
			problems = append(problems, domain.FieldError{Field: field + ".decision", Problem: "must be accept, reject, or edit"})
			continue
		}
		// The applier checks that edit has a payload and the others don't;
		// "payload": null counts as none.
		payload := d.Payload
		if bytes.Equal(bytes.TrimSpace(payload), []byte("null")) {
			payload = nil
		}
		decisions[pos] = changeset.Decision{Action: action, Payload: payload}
	}
	if len(problems) > 0 {
		h.fail(w, r, &domain.ValidationError{Entity: "apply request", Problems: problems})
		return
	}

	res, err := h.applier.Apply(ctx, id, decisions)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := apitypes.ApplyResponse{HeldByCap: make([]int64, len(res.HeldByCap))}
	for i, pos := range res.HeldByCap {
		out.HeldByCap[i] = ids[pos] // ops keep their ID when split off
	}
	err = h.store.InReadTx(ctx, func(tx *store.ReadTx) error {
		var err error
		if out.Changeset, err = changesetDTO(ctx, tx, res.Changeset); err != nil {
			return err
		}
		if res.SplitOff != nil {
			split, err := changesetDTO(ctx, tx, *res.SplitOff)
			if err != nil {
				return err
			}
			out.SplitOff = &split
		}
		return nil
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
