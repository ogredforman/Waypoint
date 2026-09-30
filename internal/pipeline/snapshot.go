package pipeline

import (
	"context"
	"fmt"
	"time"

	"github.com/ogredforman/waypoint/internal/domain"
	"github.com/ogredforman/waypoint/internal/extract"
	"github.com/ogredforman/waypoint/internal/store"
)

// SnapshotBuilder builds the state snapshot an extractor sees for an entry
// (SPEC §7.1, step 3).
type SnapshotBuilder struct {
	store         *store.Store
	now           func() time.Time
	recentEntries int
}

// NewSnapshotBuilder returns a builder over s. now stamps each snapshot's
// TakenAt. recentEntries is how many earlier entries a snapshot includes; 0
// means none.
func NewSnapshotBuilder(s *store.Store, now func() time.Time, recentEntries int) *SnapshotBuilder {
	return &SnapshotBuilder{store: s, now: now, recentEntries: recentEntries}
}

// Build returns the snapshot for entry e:
//   - the active projects, plus e's project whatever its status, each with its
//     open tasks (in position order) and open blockers;
//   - every parked idea, title only;
//   - up to recentEntries entries written before e, newest first: those of
//     e's project, or of any project if e has none. Each carries the
//     summaries of its latest run's observations, not its body.
//
// All reads happen in one read-only transaction, so the snapshot is
// consistent without holding up writers.
func (b *SnapshotBuilder) Build(ctx context.Context, e domain.Entry) (extract.StateSnapshot, error) {
	snap := extract.StateSnapshot{
		TakenAt:       b.now().UTC(),
		Projects:      []extract.SnapshotProject{},
		Ideas:         []extract.SnapshotIdea{},
		RecentEntries: []extract.SnapshotEntry{},
	}
	err := b.store.InReadTx(ctx, func(tx *store.ReadTx) error {
		if err := addProjects(ctx, tx, e, &snap); err != nil {
			return err
		}
		if err := addIdeas(ctx, tx, &snap); err != nil {
			return err
		}
		return b.addRecentEntries(ctx, tx, e, &snap)
	})
	if err != nil {
		return extract.StateSnapshot{}, fmt.Errorf("build snapshot for entry %d: %w", e.ID, err)
	}
	return snap, nil
}

func addProjects(ctx context.Context, tx *store.ReadTx, e domain.Entry, snap *extract.StateSnapshot) error {
	projects, err := tx.ListProjects(ctx)
	if err != nil {
		return err
	}
	for _, p := range projects {
		scoped := e.ProjectID != nil && *e.ProjectID == p.ID
		if p.Status != domain.ProjectActive && !scoped {
			continue
		}
		sp := extract.SnapshotProject{
			ID:       p.ID,
			Title:    p.Title,
			Status:   p.Status,
			Tasks:    []extract.SnapshotTask{},
			Blockers: []extract.SnapshotBlocker{},
		}

		tasks, err := tx.ListTasksByProject(ctx, p.ID)
		if err != nil {
			return err
		}
		for _, t := range tasks {
			if t.Status != domain.TaskTodo && t.Status != domain.TaskInProgress {
				continue
			}
			sp.Tasks = append(sp.Tasks, extract.SnapshotTask{
				ID:           t.ID,
				Title:        t.Title,
				Status:       t.Status,
				ParentTaskID: t.ParentTaskID,
				UpdatedAt:    t.UpdatedAt,
			})
		}

		blockers, err := tx.ListBlockersByProject(ctx, p.ID)
		if err != nil {
			return err
		}
		for _, bl := range blockers {
			if bl.Status != domain.BlockerOpen {
				continue
			}
			sp.Blockers = append(sp.Blockers, extract.SnapshotBlocker{
				ID:          bl.ID,
				TaskID:      bl.TaskID,
				Description: bl.Description,
			})
		}

		snap.Projects = append(snap.Projects, sp)
	}
	return nil
}

func addIdeas(ctx context.Context, tx *store.ReadTx, snap *extract.StateSnapshot) error {
	ideas, err := tx.ListIdeasByStatus(ctx, domain.IdeaParked)
	if err != nil {
		return err
	}
	for _, i := range ideas {
		snap.Ideas = append(snap.Ideas, extract.SnapshotIdea{ID: i.ID, ProjectID: i.ProjectID, Title: i.Title})
	}
	return nil
}

func (b *SnapshotBuilder) addRecentEntries(ctx context.Context, tx *store.ReadTx, e domain.Entry, snap *extract.StateSnapshot) error {
	if b.recentEntries <= 0 {
		return nil
	}
	var (
		entries []domain.Entry
		err     error
	)
	if e.ProjectID != nil {
		entries, err = tx.ListEntriesBeforeByProject(ctx, *e.ProjectID, e.ID, b.recentEntries)
	} else {
		entries, err = tx.ListEntriesBefore(ctx, e.ID, b.recentEntries)
	}
	if err != nil {
		return err
	}
	for _, re := range entries {
		obs, err := tx.ListObservationsByEntry(ctx, re.ID)
		if err != nil {
			return err
		}
		snap.RecentEntries = append(snap.RecentEntries, extract.SnapshotEntry{
			ID:        re.ID,
			CreatedAt: re.CreatedAt,
			Summaries: latestRunSummaries(obs),
		})
	}
	return nil
}

// latestRunSummaries returns the summaries of the entry's latest run's
// observations (domain.LatestRun), in their order.
func latestRunSummaries(obs []domain.Observation) []string {
	out := []string{}
	for _, o := range domain.LatestRun(obs) {
		out = append(out, o.Summary)
	}
	return out
}
