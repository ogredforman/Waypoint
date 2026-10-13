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
	stalledAfter  time.Duration
}

// NewSnapshotBuilder returns a builder over s. now stamps each snapshot's
// TakenAt. recentEntries is how many earlier entries a snapshot includes; 0
// means none.
func NewSnapshotBuilder(s *store.Store, now func() time.Time, recentEntries int) *SnapshotBuilder {
	return &SnapshotBuilder{store: s, now: now, recentEntries: recentEntries}
}

// WithStalledAfter marks an in-progress task stalled when neither it nor any
// of its steps has changed for d (config stalled_after_days). 0, the default,
// marks none.
func (b *SnapshotBuilder) WithStalledAfter(d time.Duration) *SnapshotBuilder {
	b.stalledAfter = d
	return b
}

// Build returns the snapshot for entry e:
//   - the active projects, plus e's project whatever its status (even archived), each with its
//     open milestones (in order), open tasks (in position order, each with its
//     milestone if that is open, and stalled if it is in progress and
//     unchanged, steps included, for stalledAfter), and open blockers;
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
		var stalledBefore time.Time
		if b.stalledAfter > 0 {
			stalledBefore = snap.TakenAt.Add(-b.stalledAfter)
		}
		if err := addProjects(ctx, tx, e, stalledBefore, &snap); err != nil {
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

// addProjects adds the projects an entry's snapshot holds. An in-progress
// task whose last change, or its steps' (done ones too), is before
// stalledBefore is marked stalled; a zero stalledBefore marks none.
func addProjects(ctx context.Context, tx *store.ReadTx, e domain.Entry, stalledBefore time.Time, snap *extract.StateSnapshot) error {
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
			ID:         p.ID,
			Title:      p.Title,
			Status:     p.Status,
			Milestones: []extract.SnapshotMilestone{},
			Tasks:      []extract.SnapshotTask{},
			Blockers:   []extract.SnapshotBlocker{},
		}

		milestones, err := tx.ListMilestonesByProject(ctx, p.ID)
		if err != nil {
			return err
		}
		open := make(map[int64]bool, len(milestones))
		for _, m := range milestones {
			if m.Status != domain.MilestoneOpen {
				continue
			}
			open[m.ID] = true
			sp.Milestones = append(sp.Milestones, extract.SnapshotMilestone{ID: m.ID, Title: m.Title})
		}

		tasks, err := tx.ListTasksByProject(ctx, p.ID)
		if err != nil {
			return err
		}
		changed := make(map[int64]time.Time, len(tasks))
		for _, t := range tasks {
			for _, id := range []*int64{&t.ID, t.ParentTaskID} {
				if id != nil && t.UpdatedAt.After(changed[*id]) {
					changed[*id] = t.UpdatedAt
				}
			}
		}
		for _, t := range tasks {
			if t.Status != domain.TaskTodo && t.Status != domain.TaskInProgress {
				continue
			}
			st := extract.SnapshotTask{
				ID:           t.ID,
				Title:        t.Title,
				Status:       t.Status,
				ParentTaskID: t.ParentTaskID,
				Size:         t.Size,
				UpdatedAt:    t.UpdatedAt,
				Stalled:      t.Status == domain.TaskInProgress && !stalledBefore.IsZero() && changed[t.ID].Before(stalledBefore),
			}
			if t.MilestoneID != nil && open[*t.MilestoneID] {
				st.MilestoneID = t.MilestoneID
			}
			sp.Tasks = append(sp.Tasks, st)
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
