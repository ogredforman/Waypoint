package briefing

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/ogredforman/waypoint/internal/changeset"
	"github.com/ogredforman/waypoint/internal/db"
	"github.com/ogredforman/waypoint/internal/domain"
	"github.com/ogredforman/waypoint/internal/store"
)

// updates records BriefingUpdated calls.
type updates struct{ got []int64 }

func (u *updates) BriefingUpdated(projectID int64) { u.got = append(u.got, projectID) }

type fixture struct {
	t       *testing.T
	s       *store.Store
	svc     *Service
	updates *updates
	applier *changeset.Applier
	now     time.Time
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	sqlDB, err := db.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	fsys, err := db.Migrations()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.Migrate(ctx, sqlDB, fsys); err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, updates: &updates{}, now: t0}
	clock := func() time.Time { return f.now }
	f.s = store.New(sqlDB, clock)
	f.svc = NewService(f.s, Heuristic{}, clock, f.updates, slog.New(slog.NewTextHandler(io.Discard, nil)))
	f.applier = changeset.NewApplier(f.s, f.svc)
	return f
}

// tick moves the clock on a minute so rows get distinct timestamps.
func (f *fixture) tick() { f.now = f.now.Add(time.Minute) }

func (f *fixture) write(fn func(tx *store.Tx) error) {
	f.t.Helper()
	if err := f.s.InTx(context.Background(), fn); err != nil {
		f.t.Fatal(err)
	}
	f.tick()
}

// manual applies ops as a manual changeset, refreshing briefings through the
// applier's hook.
func (f *fixture) manual(ops ...changeset.Payload) changeset.Result {
	f.t.Helper()
	res, err := f.applier.ApplyManual(context.Background(), ops...)
	if err != nil {
		f.t.Fatal(err)
	}
	f.tick()
	return res
}

func (f *fixture) project(title string, status domain.ProjectStatus) int64 {
	f.t.Helper()
	res := f.manual(changeset.CreateProject{Title: title})
	id := res.Created[1]
	if status != domain.ProjectParked {
		f.manual(changeset.UpdateProject{ProjectID: changeset.ID(id), Status: &status})
	}
	return id
}

func (f *fixture) task(projectID int64, title string, sz *domain.TaskSize) int64 {
	f.t.Helper()
	return f.manual(changeset.CreateTask{ProjectID: changeset.ID(projectID), Title: title, Size: sz}).Created[1]
}

func (f *fixture) briefing(projectID int64) domain.Briefing {
	f.t.Helper()
	b, err := f.s.GetBriefing(context.Background(), projectID)
	if err != nil {
		f.t.Fatal(err)
	}
	return b
}

func nextOf(b domain.Briefing) int64 {
	if b.NextTaskID == nil {
		return 0
	}
	return *b.NextTaskID
}

// entry stores an entry with one observation over its whole body, linked to
// targetType targetID, from a succeeded run.
func (f *fixture) entry(body, summary string, projectID *int64, targetType domain.TargetType, targetID int64) domain.Entry {
	f.t.Helper()
	ctx := context.Background()
	var e domain.Entry
	f.write(func(tx *store.Tx) error {
		var err error
		if e, err = tx.CreateEntry(ctx, domain.Entry{Body: body, ProjectID: projectID}); err != nil {
			return err
		}
		if _, err := tx.CreateExtractionRun(ctx, domain.ExtractionRun{EntryID: e.ID, Provider: "mock"}); err != nil {
			return err
		}
		run, err := tx.ClaimNextExtractionRun(ctx)
		if err != nil {
			return err
		}
		if _, err := tx.FinishExtractionRun(ctx, run.ID, store.RunOutcome{Status: domain.RunSucceeded}); err != nil {
			return err
		}
		o, err := tx.CreateObservation(ctx, domain.Observation{
			EntryID: e.ID, ExtractionRunID: run.ID, Kind: domain.ObservationProgress,
			Summary: summary, SpanStart: 0, SpanEnd: int64(len(body)),
		})
		if err != nil {
			return err
		}
		_, err = tx.CreateObservationLink(ctx, domain.ObservationLink{ObservationID: o.ID, TargetType: targetType, TargetID: targetID, Confidence: 1})
		return err
	})
	return e
}

func TestRefreshOnApply(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	p := f.project("Waypoint", domain.ProjectActive)
	if b := f.briefing(p); b.NextTaskID != nil || b.NextReason != reasonNoOpenTasks {
		t.Errorf("new active project briefing = %+v, want no next task", b)
	}

	a := f.task(p, "Write the briefer", nil)
	if got := nextOf(f.briefing(p)); got != a {
		t.Errorf("after create_task next = %d, want %d", got, a)
	}

	// A task edit (a manual changeset) moves the next action.
	b := f.task(p, "Stub the handler", nil)
	f.manual(changeset.SetTaskStatus{TaskID: changeset.ID(b), Status: domain.TaskInProgress})
	br := f.briefing(p)
	if nextOf(br) != b || br.NextReason != reasonInProgress || br.Source != domain.BriefingHeuristic || !br.GeneratedAt.Equal(f.now.Add(-time.Minute)) {
		t.Errorf("after status change briefing = %+v, want %d in progress", br, b)
	}

	// An extraction changeset refreshes it too.
	e := f.entry("blocked on the handler", "Blocked on the handler", &p, domain.TargetTask, b)
	var cs int64
	f.write(func(tx *store.Tx) error {
		runs, err := tx.ListExtractionRunsByEntry(ctx, e.ID)
		if err != nil {
			return err
		}
		c, _, err := tx.CreateChangeset(ctx, domain.Changeset{Source: domain.SourceExtraction, ExtractionRunID: &runs[0].ID, Status: domain.ChangesetPending},
			[]domain.ChangeOp{{Position: 1, OpType: domain.OpCreateBlocker, Status: domain.OpPending,
				Payload: []byte(fmt.Sprintf(`{"project_id":%d,"task_id":%d,"description":"Waiting on review"}`, p, b))}})
		cs = c.ID
		return err
	})
	if _, err := f.applier.Apply(ctx, cs, map[int64]changeset.Decision{1: {Action: changeset.Accept}}); err != nil {
		t.Fatal(err)
	}
	if got := nextOf(f.briefing(p)); got != a {
		t.Errorf("after blocking %d next = %d, want %d", b, got, a)
	}

	// Deleting the next task clears it and picks another.
	f.manual(changeset.DeleteTask{TaskID: changeset.ID(a)})
	if br := f.briefing(p); br.NextTaskID != nil || br.NextReason != reasonAllWaiting {
		t.Errorf("after deleting %d briefing = %+v, want everything waiting", a, br)
	}
}

func TestRefreshSkipsInactiveProjects(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	parked := f.project("Someday", domain.ProjectParked)
	f.task(parked, "Dream", nil)
	if _, err := f.s.GetBriefing(ctx, parked); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("parked project briefing: err = %v, want ErrNotFound", err)
	}
	for _, id := range f.updates.got {
		if id == parked {
			t.Errorf("notified about parked project %d", parked)
		}
	}

	// An explicit refresh briefs it whatever its status.
	if err := f.svc.Refresh(ctx, parked); err != nil {
		t.Fatal(err)
	}
	if nextOf(f.briefing(parked)) == 0 {
		t.Error("explicit refresh gave no next task")
	}
	if err := f.svc.Refresh(ctx, 999); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("refresh of a missing project: err = %v, want ErrNotFound", err)
	}
}

// TestDeletedProject checks that a deleted project isn't briefed, and that
// deleting its entries too refreshes the other active projects, whose recaps
// may have shown them.
func TestDeletedProject(t *testing.T) {
	for _, deleteEntries := range []bool{false, true} {
		f := newFixture(t)
		p := f.project("Waypoint", domain.ProjectActive)
		q := f.project("Garden", domain.ProjectActive)
		f.entry("todo: ship", "ship", &p, domain.TargetProject, p)
		f.updates.got = nil
		f.manual(changeset.DeleteProject{ProjectID: changeset.ID(p), DeleteEntries: deleteEntries})
		var want []int64
		if deleteEntries {
			want = []int64{q}
		}
		if !slices.Equal(f.updates.got, want) {
			t.Errorf("delete_entries %v: notified %v, want %v", deleteEntries, f.updates.got, want)
		}
	}
}

func TestRefreshNotifies(t *testing.T) {
	f := newFixture(t)
	p := f.project("Waypoint", domain.ProjectActive)
	q := f.project("Garden", domain.ProjectActive)
	f.updates.got = nil
	f.manual(
		changeset.CreateTask{ProjectID: changeset.ID(q), Title: "Buy seeds"},
		changeset.CreateTask{ProjectID: changeset.ID(p), Title: "Ship"},
	)
	if want := []int64{p, q}; !slices.Equal(f.updates.got, want) {
		t.Errorf("notified %v, want %v", f.updates.got, want)
	}
}

func TestEntryProcessed(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	p := f.project("Waypoint", domain.ProjectActive)
	q := f.project("Garden", domain.ProjectActive)
	parked := f.project("Someday", domain.ProjectParked)

	e := f.entry("wired the hub", "Wired the SSE hub", &p, domain.TargetProject, p)
	f.updates.got = nil
	f.svc.EntryProcessed(ctx, e)
	if !slices.Equal(f.updates.got, []int64{p}) {
		t.Errorf("scoped entry refreshed %v, want [%d]", f.updates.got, p)
	}
	if got := recapTexts(f.briefing(p)); !slices.Equal(got, []string{"Wired the SSE hub"}) {
		t.Errorf("recap = %q", got)
	}

	u := f.entry("planted beans", "Planted beans", nil, domain.TargetProject, q)
	f.updates.got = nil
	f.svc.EntryProcessed(ctx, u)
	if !slices.Equal(f.updates.got, []int64{p, q}) {
		t.Errorf("unscoped entry refreshed %v, want every active project [%d %d]", f.updates.got, p, q)
	}
	if got := recapTexts(f.briefing(q)); !slices.Equal(got, []string{"Planted beans"}) {
		t.Errorf("recap of the project the unscoped entry is about = %q", got)
	}
	if got := recapTexts(f.briefing(p)); !slices.Equal(got, []string{"Wired the SSE hub"}) {
		t.Errorf("recap of another project = %q", got)
	}

	f.updates.got = nil
	f.svc.EntryProcessed(ctx, f.entry("x", "X", &parked, domain.TargetProject, parked))
	if len(f.updates.got) != 0 {
		t.Errorf("entry scoped to a parked project refreshed %v", f.updates.got)
	}
}

func TestGet(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	p := f.project("Waypoint", domain.ProjectActive)
	a := f.task(p, "Write the briefer", nil)
	small := f.task(p, "Fix a typo", ptr(domain.SizeXS))
	blocked := f.task(p, "Deploy", ptr(domain.SizeS))
	blk := f.manual(changeset.CreateBlocker{ProjectID: changeset.ID(p), TaskID: ptr(changeset.ID(blocked)), Description: "No server"}).Created[1]
	f.manual(changeset.SetTaskStatus{TaskID: changeset.ID(a), Status: domain.TaskInProgress})
	f.entry("old words", "Old", &p, domain.TargetTask, a)
	e := f.entry("halfway through the briefer", "Halfway", &p, domain.TargetTask, a)

	v, err := f.svc.Get(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if v.Next == nil || v.Next.ID != a || nextOf(v.Briefing) != a {
		t.Errorf("next = %+v, want task %d", v.Next, a)
	}
	if v.Excerpt == nil || v.Excerpt.EntryID != e.ID || v.Excerpt.Text != "halfway through the briefer" {
		t.Errorf("excerpt = %+v, want the newest entry about the task", v.Excerpt)
	}
	if len(v.Blockers) != 1 || v.Blockers[0].ID != blk {
		t.Errorf("blockers = %+v, want %d", v.Blockers, blk)
	}
	if len(v.Alternatives) != 1 || v.Alternatives[0].ID != small {
		t.Errorf("alternatives = %+v, want only %d", v.Alternatives, small)
	}

	// Resolving the blocker shows up without a new briefing: the facts are
	// read fresh.
	f.manual(changeset.ResolveBlocker{BlockerID: changeset.ID(blk)})
	if v, err = f.svc.Get(ctx, p); err != nil {
		t.Fatal(err)
	}
	if len(v.Blockers) != 0 || len(v.Alternatives) != 2 {
		t.Errorf("after resolving: blockers %+v, alternatives %+v", v.Blockers, v.Alternatives)
	}
}

func TestGetGeneratesMissingBriefing(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	parked := f.project("Someday", domain.ProjectParked)
	tk := f.task(parked, "Dream", nil)

	v, err := f.svc.Get(ctx, parked)
	if err != nil {
		t.Fatal(err)
	}
	if v.Next == nil || v.Next.ID != tk || v.Excerpt != nil {
		t.Errorf("view = %+v, want next %d and no excerpt", v, tk)
	}
	if _, err := f.svc.Get(ctx, 999); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Get of a missing project: err = %v, want ErrNotFound", err)
	}
}

// recapTexts lists a briefing's recap points, newest entry first.
func recapTexts(b domain.Briefing) []string {
	out := []string{}
	for _, e := range b.Recap {
		for _, p := range e.Points {
			out = append(out, p.Text)
		}
	}
	return out
}

// recapBriefer recaps the given entries, whether or not they're recent.
type recapBriefer struct{ entries []int64 }

func (b recapBriefer) Brief(_ context.Context, st State) (domain.Briefing, error) {
	br := domain.Briefing{ProjectID: st.Project.ID, Source: domain.BriefingLLM}
	for _, id := range b.entries {
		br.Recap = append(br.Recap, domain.RecapEntry{EntryID: id, Points: []domain.RecapPoint{{Kind: domain.ObservationNote, Text: "Said something"}}})
	}
	return br, nil
}

func TestRefreshDropsUnknownAndRepeatedRecapEntries(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	p := f.project("Waypoint", domain.ProjectActive)
	q := f.project("Garden", domain.ProjectActive)
	here := f.entry("wired the hub", "Wired the SSE hub", &p, domain.TargetProject, p)
	elsewhere := f.entry("planted beans", "Planted beans", &q, domain.TargetProject, q)

	svc := NewService(f.s, recapBriefer{entries: []int64{elsewhere.ID, here.ID, 999, here.ID}}, time.Now, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := svc.Refresh(ctx, p); err != nil {
		t.Fatal(err)
	}
	got := f.briefing(p).Recap
	if len(got) != 1 || got[0].EntryID != here.ID || !got[0].CreatedAt.Equal(here.CreatedAt) {
		t.Errorf("recap = %+v, want only entry %d, once, with its date", got, here.ID)
	}
}

// fixedBriefer always names the same next task.
type fixedBriefer struct{ next int64 }

func (b fixedBriefer) Brief(_ context.Context, st State) (domain.Briefing, error) {
	return domain.Briefing{ProjectID: st.Project.ID, NextTaskID: &b.next, Source: domain.BriefingLLM}, nil
}

func TestRefreshRejectsUnknownNextTask(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	p := f.project("Waypoint", domain.ProjectActive)
	q := f.project("Garden", domain.ProjectActive)
	done := f.task(p, "Done already", nil)
	f.manual(changeset.SetTaskStatus{TaskID: changeset.ID(done), Status: domain.TaskDone})
	elsewhere := f.task(q, "Buy seeds", nil)

	for _, next := range []int64{999, done, elsewhere} {
		svc := NewService(f.s, fixedBriefer{next: next}, time.Now, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
		if err := svc.Refresh(ctx, p); err == nil {
			t.Errorf("briefing naming task %d accepted", next)
		}
	}
	if br := f.briefing(p); br.Source != domain.BriefingHeuristic {
		t.Errorf("briefing replaced: %+v", br)
	}
}

func TestGetRefreshesStaleBriefing(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	p := f.project("Waypoint", domain.ProjectActive)
	a := f.task(p, "Write the briefer", nil)
	b := f.task(p, "Stub the handler", nil)
	if got := nextOf(f.briefing(p)); got != a {
		t.Fatalf("next = %d, want %d", got, a)
	}

	// Finish the next task without the hook, as if the refresh had failed.
	done := domain.TaskDone
	if _, err := changeset.NewApplier(f.s, nil).
		ApplyManual(ctx, changeset.SetTaskStatus{TaskID: changeset.ID(a), Status: done}); err != nil {
		t.Fatal(err)
	}
	v, err := f.svc.Get(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if v.Next == nil || v.Next.ID != b || nextOf(f.briefing(p)) != b {
		t.Errorf("next = %+v, want the stale briefing regenerated with %d", v.Next, b)
	}
}
