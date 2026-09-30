package changeset

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/ogredforman/waypoint/internal/domain"
	"github.com/ogredforman/waypoint/internal/store"
)

func TestRejectPending(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	_, runID := newRun(t, f)
	cs := propose(t, f, &runID,
		op(1, domain.OpCreateTask, jsonf(`{"project_id":%d,"title":"New"}`, f.projA)),
		op(2, domain.OpDeleteTask, jsonf(`{"task_id":%d}`, f.a1)),
	)
	tasksBefore := taskCount(t, f, f.projA)

	var got domain.Changeset
	err := f.s.InTx(ctx, func(tx *store.Tx) (err error) {
		got, err = RejectPending(ctx, tx, cs)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.ChangesetRejected || got.ResolvedAt == nil {
		t.Errorf("changeset %+v, want rejected and resolved", got)
	}
	if st := opStatuses(t, f, cs); !reflect.DeepEqual(st, []domain.ChangeOpStatus{domain.OpRejected, domain.OpRejected}) {
		t.Errorf("op statuses = %v, want all rejected", st)
	}
	if n := taskCount(t, f, f.projA); n != tasksBefore {
		t.Errorf("%d tasks, want %d: rejecting mustn't apply anything", n, tasksBefore)
	}

	err = f.s.InTx(ctx, func(tx *store.Tx) error {
		_, err := RejectPending(ctx, tx, cs)
		return err
	})
	if !errors.Is(err, ErrNotPending) {
		t.Errorf("rejecting again = %v, want ErrNotPending", err)
	}
}

func TestRejectPendingRollsBackWithCaller(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	cs := propose(t, f, nil, op(1, domain.OpDeleteTask, jsonf(`{"task_id":%d}`, f.a1)))
	boom := errors.New("boom")

	err := f.s.InTx(ctx, func(tx *store.Tx) error {
		if _, err := RejectPending(ctx, tx, cs); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("InTx = %v, want boom", err)
	}
	if got := must(f.s.GetChangeset(ctx, cs))(t); got.Status != domain.ChangesetPending {
		t.Errorf("changeset status = %s, want still pending", got.Status)
	}
	if st := opStatuses(t, f, cs); st[0] != domain.OpPending {
		t.Errorf("op status = %s, want still pending", st[0])
	}
}
