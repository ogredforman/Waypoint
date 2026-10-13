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
//
// A refresh briefs with the fast Briefer at once. With a model (WithModel),
// it also queues the project, and Run asks the model in the background and
// replaces the briefing, unless the project was refreshed again meanwhile.
// So a slow model never holds up an apply or the "Now" screen.
type Service struct {
	store    *store.Store
	briefer  Briefer
	model    Briefer
	now      func() time.Time
	notifier Notifier
	logger   *slog.Logger

	// mu serializes refreshes, so an older result can't overwrite a newer one.
	mu sync.Mutex
	// gen counts each project's refreshes, under mu. A model briefing is
	// saved only if no refresh happened while the model was thinking.
	gen map[int64]uint64

	// qmu guards the model queue: project IDs in order, each once.
	qmu    sync.Mutex
	queue  []int64
	queued map[int64]bool
	wake   chan struct{}
}

// NewService returns a Service that briefs with b. now stamps each
// briefing's GeneratedAt. A nil notifier drops notifications.
func NewService(s *store.Store, b Briefer, now func() time.Time, n Notifier, logger *slog.Logger) *Service {
	return &Service{
		store: s, briefer: b, now: now, notifier: n, logger: logger,
		gen: map[int64]uint64{}, queued: map[int64]bool{}, wake: make(chan struct{}, 1),
	}
}

// WithModel makes m the Briefer Run uses after each refresh, and returns s.
// Call it before the Service is used. m should fall back to b's judgment
// itself; a briefing it returns with a Source other than llm is dropped,
// since b's already stands.
func (s *Service) WithModel(m Briefer) *Service {
	s.model = m
	return s
}

// Run asks the model for the queued projects' briefings, one at a time,
// until ctx is done. Without a model it returns at once.
func (s *Service) Run(ctx context.Context) {
	if s.model == nil {
		return
	}
	for ctx.Err() == nil {
		id, ok := s.dequeue()
		if !ok {
			select {
			case <-ctx.Done():
			case <-s.wake:
			}
			continue
		}
		if err := s.refreshModel(ctx, id); err != nil && ctx.Err() == nil {
			s.logger.Error("model briefing failed", "project_id", id, "err", err)
		}
	}
}

func (s *Service) enqueue(projectID int64) {
	s.qmu.Lock()
	defer s.qmu.Unlock()
	if !s.queued[projectID] {
		s.queued[projectID] = true
		s.queue = append(s.queue, projectID)
	}
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Service) dequeue() (int64, bool) {
	s.qmu.Lock()
	defer s.qmu.Unlock()
	if len(s.queue) == 0 {
		return 0, false
	}
	id := s.queue[0]
	s.queue = s.queue[1:]
	delete(s.queued, id)
	return id, true
}

// refreshModel briefs the project with the model and saves the result if
// it came from the model and the project wasn't refreshed in the meantime
// (that refresh queued it again, so the model will see the newer state).
func (s *Service) refreshModel(ctx context.Context, projectID int64) error {
	s.mu.Lock()
	gen := s.gen[projectID]
	st, err := s.loadState(ctx, projectID)
	s.mu.Unlock()
	if errors.Is(err, store.ErrNotFound) {
		return nil // deleted since it was queued
	}
	if err != nil {
		return fmt.Errorf("model briefing of project %d: %w", projectID, err)
	}
	// No lock or transaction is held while the model thinks.
	b, err := s.model.Brief(ctx, st)
	if err != nil {
		return fmt.Errorf("model briefing of project %d: %w", projectID, err)
	}
	if b.Source != domain.BriefingLLM {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gen[projectID] != gen {
		return nil
	}
	if err := s.save(ctx, st, b); err != nil {
		return fmt.Errorf("model briefing of project %d: %w", projectID, err)
	}
	return nil
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

	st, err := s.loadState(ctx, projectID)
	if err != nil {
		return fmt.Errorf("refresh briefing of project %d: %w", projectID, err)
	}
	// No transaction is open here: a Briefer may call out to a model.
	b, err := s.briefer.Brief(ctx, st)
	if err != nil {
		return fmt.Errorf("refresh briefing of project %d: %w", projectID, err)
	}
	s.gen[projectID]++
	if err := s.save(ctx, st, b); err != nil {
		return fmt.Errorf("refresh briefing of project %d: %w", projectID, err)
	}
	if s.model != nil {
		s.enqueue(projectID)
	}
	return nil
}

func (s *Service) loadState(ctx context.Context, projectID int64) (State, error) {
	var st State
	err := s.store.InReadTx(ctx, func(tx *store.ReadTx) error {
		var err error
		st, err = loadState(ctx, tx, projectID)
		return err
	})
	return st, err
}

// save checks b against st, the state it was made from, caches it, and
// tells the notifier. The caller holds mu.
func (s *Service) save(ctx context.Context, st State, b domain.Briefing) error {
	if id := b.NextTaskID; id != nil && !slices.ContainsFunc(st.Tasks, func(t domain.Task) bool { return t.ID == *id && isOpen(t) }) {
		return fmt.Errorf("next task %d is not an open task of the project", *id)
	}
	b.ProjectID = st.Project.ID
	b.Recap = knownEntries(b.Recap, st.RecentEntries)
	b.GeneratedAt = s.now().UTC()
	err := s.store.InTx(ctx, func(tx *store.Tx) error {
		_, err := tx.UpsertBriefing(ctx, b)
		return err
	})
	if err != nil {
		return err
	}
	if s.notifier != nil {
		s.notifier.BriefingUpdated(st.Project.ID)
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
		if errors.Is(err, store.ErrNotFound) {
			continue // deleted (ADR 0008); its briefing went with it
		}
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
	if len(res.DeletedEntries) > 0 {
		// Entries deleted with a project may be in any project's recap.
		s.refreshAllActive(ctx, res.DeletedEntries[0])
		return
	}
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
	s.refreshAllActive(ctx, e.ID)
}

// EntryDeleted refreshes every active project's briefing after entry e was
// deleted: its points may be in any project's recap.
func (s *Service) EntryDeleted(ctx context.Context, e domain.Entry) {
	s.refreshAllActive(ctx, e.ID)
}

func (s *Service) refreshAllActive(ctx context.Context, entryID int64) {
	projects, err := s.store.ListProjects(ctx)
	if err != nil {
		s.logger.Error("refreshing briefings failed", "entry_id", entryID, "err", err)
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
			v.Step = stepOf(st.Tasks, *v.Next)
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

// stepOf places t among its parent's subtasks, or returns nil if t has no
// parent in tasks. tasks are in position order, ties by id.
func stepOf(tasks []domain.Task, t domain.Task) *Step {
	if t.ParentTaskID == nil {
		return nil
	}
	i := slices.IndexFunc(tasks, func(x domain.Task) bool { return x.ID == *t.ParentTaskID })
	if i < 0 {
		return nil
	}
	step := Step{Parent: tasks[i]}
	for _, x := range tasks {
		if x.ParentTaskID != nil && *x.ParentTaskID == *t.ParentTaskID {
			step.Number++
		}
		if x.ID == t.ID {
			break
		}
	}
	return &step
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
// blockers, and milestones: State without the recent entries.
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
	if st.Milestones, err = tx.ListMilestonesByProject(ctx, projectID); err != nil {
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
