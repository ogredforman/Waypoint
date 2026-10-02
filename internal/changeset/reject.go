package changeset

import (
	"context"
	"fmt"

	"github.com/ogredforman/waypoint/internal/domain"
	"github.com/ogredforman/waypoint/internal/store"
)

// RejectPending rejects pending changeset id and all of its ops inside the
// caller's transaction, without applying anything. Reprocessing an entry uses
// it to retire the changesets of earlier runs, so the same proposal can't be
// applied twice. Observation links waiting on its ops are dropped. It
// returns ErrNotPending if the changeset is already resolved.
func RejectPending(ctx context.Context, tx *store.Tx, id int64) (domain.Changeset, error) {
	cs, err := tx.GetChangeset(ctx, id)
	if err != nil {
		return domain.Changeset{}, fmt.Errorf("reject changeset: %w", err)
	}
	if cs.Status != domain.ChangesetPending {
		return domain.Changeset{}, fmt.Errorf("reject changeset %d: %w", id, ErrNotPending)
	}
	ops, err := tx.ListChangeOps(ctx, id)
	if err != nil {
		return domain.Changeset{}, fmt.Errorf("reject changeset %d: %w", id, err)
	}
	for _, op := range ops {
		if _, err := tx.SetChangeOpResult(ctx, op.ID, domain.OpRejected, nil); err != nil {
			return domain.Changeset{}, fmt.Errorf("reject changeset %d: %w", id, err)
		}
		if err := tx.DropPendingLinks(ctx, op.ID); err != nil {
			return domain.Changeset{}, fmt.Errorf("reject changeset %d: %w", id, err)
		}
	}
	cs, err = tx.ResolveChangeset(ctx, id, domain.ChangesetRejected)
	if err != nil {
		return domain.Changeset{}, fmt.Errorf("reject changeset %d: %w", id, err)
	}
	return cs, nil
}
