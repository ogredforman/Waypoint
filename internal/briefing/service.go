package briefing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/ogredforman/waypoint/internal/changeset"
	"github.com/ogredforman/waypoint/internal/domain"
	"github.com/ogredforman/waypoint/internal/store"
)

// Notifier is told when a project's briefing was regenerated. It must not
// block.
type Notifier interface {
	BriefingUpdated(projectID int64)
}

// Service keeps the cached briefings current and reads them for the "Now"
// screen. It refreshes active projects' briefings after a changeset is
// applied (as a changeset.AfterApply) and after an entry is processed.
type Service struct {
	store    *store.Store
	briefer  Briefer
	now      func() time.Time
	notifier Notifier
	logger   *slog.Logger

	// mu serializes refreshes, so an older result can't overwrite a newer one.
	mu sync.Mutex
}

// NewService returns a Service that briefs with b. now stamps each
// briefing's GeneratedAt. A nil notifier drops notifications.
func NewService(s *store.Store, b Briefer, now func() time.Time, n Notifier, logger *slog.Logger) *Service {
	return &Service{store: s, briefer: b, now: now, notifier: n, logger: logger}
}

// Refresh regenerates the briefings of the given projects, whatever their
// status. It carries on past a failed project and returns every failure;
// store.ErrNotFound means there is no such project.
func (s *Service) Refresh(ctx context.Context, projectIDs ...int64) error {
	var errs []error
	for _, id := range projectIDs {
		if err := s.refresh(ctx, id); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (s *Service) refresh(ctx context.Context, projectID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var st State
	err := s.store.InReadTx(ctx, func(tx *store.ReadTx) error {
		var err error
		st, err = loadState(ctx, tx, projectID)
		return err
	})
	if err != nil {
		return fmt.Errorf("refresh briefing of project %d: %w", projectID, err)
	}
	// No transaction is open here: a Briefer may call out to a model.
	b, err := s.briefer.Brief(ctx, st)
	if err != nil {
		return fmt.Errorf("refresh briefing of project %d: %w", projectID, err)
	}
	if id := b.NextTaskID; id != nil && !slices.ContainsFunc(st.Tasks, func(t domain.Task) bool { return t.ID == *id && isOpen(t) }) {
		return fmt.Errorf("refresh briefing of project %d: next task %d is not an open task of the project", projectID, *id)
	}
	b.ProjectID = projectID
	b.Recap = knownEntries(b.Recap, st.RecentEntries)
	b.GeneratedAt = s.now().UTC()
	err = s.store.InTx(ctx, func(tx *store.Tx) error {
		_, err := tx.UpsertBriefing(ctx, b)
		return err
	})
	if err != nil {
		return fmt.Errorf("refresh briefing of project %d: %w", projectID, err)
	}
	if s.notifier != nil {
		s.notifier.BriefingUpdated(projectID)
	}
	return nil
}

// knownEntries keeps the recap entries that are among the recent ones the
// Briefer was shown, once each, so a model can't name an entry it wasn't
// given or list one twice.
func knownEntries(recap []domain.RecapEntry, recent []RecentEntry) []domain.RecapEntry {
	out := []domain.RecapEntry{}
	seen := map[int64]bool{}
	for _, e := range recap {
		if !seen[e.EntryID] && slices.ContainsFunc(recent, func(r RecentEntry) bool { return r.ID == e.EntryID }) {
			seen[e.EntryID] = true
			out = append(out, e)
		}
	}
	return out
}

// refreshActive refreshes those of projectIDs that are active; others aren't
// on the "Now" screen. Failures are logged: the change that prompted the
// refresh is already committed, and the next one retries.
func (s *Service) refreshActive(ctx context.Context, projectIDs []int64) {
	for _, id := range projectIDs {
		p, err := s.store.GetProject(ctx, id)
		if err == nil && p.Status != domain.ProjectActive {
			continue
		}
		if err == nil {
			err = s.refresh(ctx, id)
		}
		if err != nil {
			s.logger.Error("refreshing briefing failed", "project_id", id, "err", err)
		}
	}
}

// Applied refreshes the briefings of the active projects a changeset
// changed. It makes the Service a changeset.AfterApply.
func (s *Service) Applied(ctx context.Context, res changeset.Result) {
	s.refreshActive(ctx, res.Projects)
}

// EntryProcessed refreshes briefings after an entry's extraction run
// succeeded, since the recap draws on entries: that of the entry's project,
// or every active project's for an unscoped entry.
func (s *Service) EntryProcessed(ctx context.Context, e domain.Entry) {
	if e.ProjectID != nil {
		s.refreshActive(ctx, []int64{*e.ProjectID})
		return
	}
	projects, err := s.store.ListProjects(ctx)
	if err != nil {
		s.logger.Error("refreshing briefings failed", "entry_id", e.ID, "err", err)
		return
	}
	var ids []int64
	for _, p := range projects {
		if p.Status == domain.ProjectActive {
			ids = append(ids, p.ID)
		}
	}
	s.refreshActive(ctx, ids)
}

// Get returns the project's briefing as the "Now" screen shows it. A project
// without a usable cached briefing (none yet, or one whose next task is no
// longer open, say because a refresh failed) gets one generated first. It
// returns store.ErrNotFound if there is no such project.
func (s *Service) Get(ctx context.Context, projectID int64) (View, error) {
	v, err := s.view(ctx, projectID)
	if errors.Is(err, errStale) {
		if err := s.refresh(ctx, projectID); err != nil {
			return View{}, err
		}
		v, err = s.view(ctx, projectID)
	}
	if err != nil {
		return View{}, fmt.Errorf("get briefing of project %d: %w", projectID, err)
	}
	return v, nil
}

// errStale is view's signal that the project exists but its cached briefing
// is missing or names a task that is no longer open.
var errStale = errors.New("briefing missing or stale")

func (s *Service) view(ctx context.Context, projectID int64) (View, error) {
	var v View
	err := s.store.InReadTx(ctx, func(tx *store.ReadTx) error {
		st, err := loadTasks(ctx, tx, projectID)
		if err != nil {
			return err
		}
		v.Briefing, err = tx.GetBriefing(ctx, projectID)
		if errors.Is(err, store.ErrNotFound) {
			return errStale
		}
		if err != nil {
			return err
		}
		if id := v.Briefing.NextTaskID; id != nil {
			i := slices.IndexFunc(st.Tasks, func(t domain.Task) bool { return t.ID == *id && isOpen(t) })
			if i < 0 {
				return errStale
			}
			v.Next = &st.Tasks[i]
			if v.Excerpt, err = excerpt(ctx, tx, *id); err != nil {
				return err
			}
		}
		v.Blockers = st.Blockers
		v.Alternatives = alternatives(st, v.Briefing.NextTaskID)
		return nil
	})
	return v, err
}

// excerpt returns the text of the newest observation linked to the task, or
// nil if there is none. The observation may come from an earlier run of a
// reprocessed entry; it still quotes the user's words about the task.
func excerpt(ctx context.Context, tx *store.ReadTx, taskID int64) (*Excerpt, error) {
	links, err := tx.ListLinksByTarget(ctx, domain.TargetTask, taskID)
	if err != nil || len(links) == 0 {
		return nil, err
	}
	o, err := tx.GetObservation(ctx, links[len(links)-1].ObservationID)
	if err != nil {
		return nil, err
	}
	e, err := tx.GetEntry(ctx, o.EntryID)
	if err != nil {
		return nil, err
	}
	// The extraction validator checked the span against the body.
	if o.SpanStart < 0 || o.SpanEnd > int64(len(e.Body)) || o.SpanStart >= o.SpanEnd {
		return nil, nil
	}
	return &Excerpt{EntryID: e.ID, CreatedAt: e.CreatedAt, Text: e.Body[o.SpanStart:o.SpanEnd]}, nil
}

// loadState reads what a Briefer sees of the project.
func loadState(ctx context.Context, tx *store.ReadTx, projectID int64) (State, error) {
	st, err := loadTasks(ctx, tx, projectID)
	if err != nil {
		return State{}, err
	}
	entries, err := tx.ListRecentEntriesForProject(ctx, projectID, recapEntries)
	if err != nil {
		return State{}, err
	}
	for _, e := range entries {
		obs, err := tx.ListObservationsByEntry(ctx, e.ID)
		if err != nil {
			return State{}, err
		}
		re := RecentEntry{ID: e.ID, CreatedAt: e.CreatedAt, Observations: []domain.RecapPoint{}}
		for _, o := range domain.LatestRun(obs) {
			re.Observations = append(re.Observations, domain.RecapPoint{Kind: o.Kind, Text: o.Summary})
		}
		st.RecentEntries = append(st.RecentEntries, re)
	}
	return st, nil
}

// loadTasks reads the project with its tasks, dependencies, and open
// blockers: State without the recent entries.
func loadTasks(ctx context.Context, tx *store.ReadTx, projectID int64) (State, error) {
	p, err := tx.GetProject(ctx, projectID)
	if err != nil {
		return State{}, err
	}
	st := State{Project: p, Blockers: []domain.Blocker{}, RecentEntries: []RecentEntry{}}
	if st.Tasks, err = tx.ListTasksByProject(ctx, projectID); err != nil {
		return State{}, err
	}
	if st.Dependencies, err = tx.ListTaskDependenciesByProject(ctx, projectID); err != nil {
		return State{}, err
	}
	blockers, err := tx.ListBlockersByProject(ctx, projectID)
	if err != nil {
		return State{}, err
	}
	for _, b := range blockers {
		if b.Status == domain.BlockerOpen {
			st.Blockers = append(st.Blockers, b)
		}
	}
	return st, nil
}
