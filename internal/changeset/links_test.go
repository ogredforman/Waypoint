package changeset

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ogredforman/waypoint/internal/domain"
	"github.com/ogredforman/waypoint/internal/store"
)

// observeOps stores one observation on the run's entry with a pending link to
// the entity each op of changeset cs creates, at confidence 0.7.
func observeOps(t *testing.T, f fixture, entryID, runID, cs int64) domain.Observation {
	t.Helper()
	ctx := context.Background()
	var obs domain.Observation
	err := f.s.InTx(ctx, func(tx *store.Tx) (err error) {
		obs, err = tx.CreateObservation(ctx, domain.Observation{
			EntryID: entryID, ExtractionRunID: runID, Kind: domain.ObservationPlan, Summary: "s", SpanStart: 0, SpanEnd: 1,
		})
		if err != nil {
			return err
		}
		ops, err := tx.ListChangeOps(ctx, cs)
		if err != nil {
			return err
		}
		for _, op := range ops {
			p, err := DecodePayload(op.OpType, op.Payload)
			if err != nil {
				return err
			}
			kind, _ := p.Creates()
			if _, err := tx.CreatePendingLink(ctx, domain.PendingObservationLink{
				ObservationID: obs.ID, ChangeOpID: op.ID, TargetType: kind, Confidence: 0.7,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return obs
}

// linkedFrom reports whether observation obs links to the target.
func linkedFrom(t *testing.T, f fixture, obs int64, kind domain.TargetType, id int64) bool {
	t.Helper()
	for _, l := range must(f.s.ListLinksByTarget(context.Background(), kind, id))(t) {
		if l.ObservationID == obs {
			if l.Confidence != 0.7 {
				t.Errorf("link to %s %d has confidence %v, want the pending link's 0.7", kind, id, l.Confidence)
			}
			return true
		}
	}
	return false
}

func pendingLinks(t *testing.T, f fixture) int {
	t.Helper()
	var n int
	if err := f.db.QueryRow(`SELECT count(*) FROM pending_observation_links`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// TestApplyResolvesPendingLinks checks that a pending link becomes a real link
// when its op is applied (accepted or edited), goes when its op is rejected,
// and follows an undecided op into the split-off changeset.
func TestApplyResolvesPendingLinks(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	entryID, runID := newRun(t, f)
	cs := propose(t, f, &runID,
		op(1, domain.OpCreateTask, jsonf(`{"project_id":%d,"title":"accepted"}`, f.projA)),
		op(2, domain.OpParkIdea, `{"title":"rejected"}`),
		op(3, domain.OpCreateBlocker, jsonf(`{"project_id":%d,"description":"proposed"}`, f.projA)),
		op(4, domain.OpCreateTask, jsonf(`{"project_id":%d,"title":"undecided"}`, f.projA)),
	)
	obs := observeOps(t, f, entryID, runID, cs)
	ideasBefore := must(f.s.ListIdeasByStatus(ctx, domain.IdeaParked))(t)

	a := NewApplier(f.s, 3, nil)
	res, err := a.Apply(ctx, cs, map[int64]Decision{
		1: {Action: Accept},
		2: {Action: Reject},
		3: {Action: Edit, Payload: json.RawMessage(jsonf(`{"project_id":%d,"description":"edited"}`, f.projA))},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !linkedFrom(t, f, obs.ID, domain.TargetTask, res.Created[1]) {
		t.Error("no link to the accepted op's task")
	}
	if !linkedFrom(t, f, obs.ID, domain.TargetBlocker, res.Created[3]) {
		t.Error("no link to the edited op's blocker")
	}
	for _, i := range ideasBefore {
		if linkedFrom(t, f, obs.ID, domain.TargetIdea, i.ID) {
			t.Errorf("idea %d linked; the rejected op's link should be dropped", i.ID)
		}
	}
	if n := pendingLinks(t, f); n != 1 {
		t.Fatalf("%d pending links, want 1 for the undecided op", n)
	}

	child := res.SplitOff.ID
	ops := must(f.s.ListChangeOps(ctx, child))(t)
	res, err = a.Apply(ctx, child, acceptAll(ops[0].Position))
	if err != nil {
		t.Fatal(err)
	}
	if !linkedFrom(t, f, obs.ID, domain.TargetTask, res.Created[ops[0].Position]) {
		t.Error("no link to the task created from the split-off changeset")
	}
	if n := pendingLinks(t, f); n != 0 {
		t.Errorf("%d pending links left, want 0", n)
	}
}

func TestRejectPendingDropsPendingLinks(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	entryID, runID := newRun(t, f)
	cs := propose(t, f, &runID, op(1, domain.OpCreateTask, jsonf(`{"project_id":%d,"title":"New"}`, f.projA)))
	observeOps(t, f, entryID, runID, cs)

	err := f.s.InTx(ctx, func(tx *store.Tx) error {
		_, err := RejectPending(ctx, tx, cs)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := pendingLinks(t, f); n != 0 {
		t.Errorf("%d pending links left, want 0", n)
	}
}
