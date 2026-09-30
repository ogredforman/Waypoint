package changeset

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ogredforman/waypoint/internal/domain"
	"github.com/ogredforman/waypoint/internal/store"
)

// ApplyManual records ops as a manual changeset (SPEC §7.4) and applies all
// of them in one transaction. The ops take positions 1..N in the order given,
// so a later op can use a temp ID ("$new:N") for a row an earlier one creates.
// Nothing is stored if any op fails: no rows change and no changeset is
// recorded. Going over the active-project cap is such a failure. Errors are
// the same as Apply's.
func (a *Applier) ApplyManual(ctx context.Context, ops ...Payload) (Result, error) {
	var c checker
	if len(ops) == 0 {
		c.add("ops", "a manual edit needs at least one op")
	}
	for i, p := range ops {
		if p == nil {
			c.add(fmt.Sprintf("ops[%d]", i+1), "is missing")
		}
	}
	if err := c.errEntity("manual changeset"); err != nil {
		return Result{}, err
	}
	changeOps := make([]domain.ChangeOp, len(ops))
	decisions := make(map[int64]Decision, len(ops))
	for i, p := range ops {
		raw, err := json.Marshal(p)
		if err != nil {
			return Result{}, fmt.Errorf("manual changeset: encode op %d: %w", i+1, err)
		}
		pos := int64(i + 1)
		changeOps[i] = domain.ChangeOp{Position: pos, OpType: p.OpType(), Payload: raw, Status: domain.OpPending}
		decisions[pos] = Decision{Action: Accept}
	}

	var res Result
	err := a.store.InTx(ctx, func(tx *store.Tx) error {
		cs, _, err := tx.CreateChangeset(ctx, domain.Changeset{
			Source: domain.SourceManual,
			Status: domain.ChangesetPending,
		}, changeOps)
		if err != nil {
			return fmt.Errorf("manual changeset: %w", err)
		}
		res, err = a.apply(ctx, tx, cs.ID, decisions)
		return err
	})
	if err != nil {
		return Result{}, err
	}
	return res, nil
}
