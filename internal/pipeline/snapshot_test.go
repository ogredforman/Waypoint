package pipeline

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/ogredforman/waypoint/internal/db"
	"github.com/ogredforman/waypoint/internal/domain"
	"github.com/ogredforman/waypoint/internal/extract"
	"github.com/ogredforman/waypoint/internal/extract/mock"
	"github.com/ogredforman/waypoint/internal/store"
)

var t0 = time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)

// clock is a settable time source; tick moves it on a minute so rows get
// distinct timestamps.
type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }
func (c *clock) tick()          { c.t = c.t.Add(time.Minute) }

type fixture struct {
	t     *testing.T
	s     *store.Store
	sqlDB *sql.DB
	c     *clock
}

func newFixture(t *testing.T) fixture {
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
	c := &clock{t: t0}
	return fixture{t: t, s: store.New(sqlDB, c.now), sqlDB: sqlDB, c: c}
}

// write runs fn in a transaction, then ticks the clock.
func (f fixture) write(fn func(tx *store.Tx) error) {
	f.t.Helper()
	if err := f.s.InTx(context.Background(), fn); err != nil {
		f.t.Fatal(err)
	}
	f.c.tick()
}

func (f fixture) project(title string, status domain.ProjectStatus) (p domain.Project) {
	f.t.Helper()
	f.write(func(tx *store.Tx) (err error) {
		p, err = tx.CreateProject(context.Background(), domain.Project{Title: title, Status: status})
		return err
	})
	return p
}

func (f fixture) task(task domain.Task) domain.Task {
	f.t.Helper()
	f.write(func(tx *store.Tx) (err error) {
		task, err = tx.CreateTask(context.Background(), task)
		return err
	})
	return task
}

func (f fixture) milestone(m domain.Milestone) domain.Milestone {
	f.t.Helper()
	f.write(func(tx *store.Tx) (err error) {
		m, err = tx.CreateMilestone(context.Background(), m)
		return err
	})
	return m
}

func (f fixture) blocker(b domain.Blocker) domain.Blocker {
	f.t.Helper()
	f.write(func(tx *store.Tx) (err error) {
		b, err = tx.CreateBlocker(context.Background(), b)
		return err
	})
	return b
}

func (f fixture) idea(i domain.Idea) domain.Idea {
	f.t.Helper()
	f.write(func(tx *store.Tx) (err error) {
		i, err = tx.CreateIdea(context.Background(), i)
		return err
	})
	return i
}

func (f fixture) entry(body string, projectID *int64) (e domain.Entry) {
	f.t.Helper()
	f.write(func(tx *store.Tx) (err error) {
		e, err = tx.CreateEntry(context.Background(), domain.Entry{Body: body, ProjectID: projectID})
		return err
	})
	return e
}

// observe records a run over entry e with one note observation per summary,
// spanning the whole body, in reverse so the snapshot's span order matters.
func (f fixture) observe(e domain.Entry, summaries ...string) {
	f.t.Helper()
	res, err := f.sqlDB.ExecContext(context.Background(),
		`INSERT INTO extraction_runs (entry_id, provider, status, created_at, updated_at)
		 VALUES (?, 'mock', 'succeeded', 0, 0)`, e.ID)
	if err != nil {
		f.t.Fatal(err)
	}
	runID, err := res.LastInsertId()
	if err != nil {
		f.t.Fatal(err)
	}
	f.write(func(tx *store.Tx) error {
		for i := len(summaries) - 1; i >= 0; i-- {
			if _, err := tx.CreateObservation(context.Background(), domain.Observation{
				EntryID: e.ID, ExtractionRunID: runID, Kind: domain.ObservationNote,
				Summary: summaries[i], SpanStart: int64(i), SpanEnd: int64(i + 1),
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

// world is the state the tests build snapshots over.
type world struct {
	waypoint, shed, piano, done  domain.Project
	alpha, mvp, beta             domain.Milestone
	spec, stub, piece, polish    domain.Task
	driver                       domain.Blocker
	themes, garden               domain.Idea
	early, loose, shedDay, fresh domain.Entry
	current, later               domain.Entry
}

// seed creates active projects Waypoint and Shed, parked Piano, and a done
// project; Waypoint milestones done Alpha, then open MVP and Beta; tasks and
// blockers of every status; ideas of every status; and
// entries around the current Waypoint entry.
func seed(f fixture) world {
	var w world
	w.waypoint = f.project("Build Waypoint", domain.ProjectActive)
	w.shed = f.project("Garden shed", domain.ProjectActive)
	w.piano = f.project("Learn piano", domain.ProjectParked)
	w.done = f.project("Old thing", domain.ProjectDone)

	w.alpha = f.milestone(domain.Milestone{ProjectID: w.waypoint.ID, Title: "Alpha", Status: domain.MilestoneDone})
	w.mvp = f.milestone(domain.Milestone{ProjectID: w.waypoint.ID, Title: "MVP", Status: domain.MilestoneOpen})
	w.beta = f.milestone(domain.Milestone{ProjectID: w.waypoint.ID, Title: "Beta", Status: domain.MilestoneOpen})

	w.spec = f.task(domain.Task{ProjectID: w.waypoint.ID, MilestoneID: &w.mvp.ID, Title: "Write the spec", Status: domain.TaskTodo})
	f.task(domain.Task{ProjectID: w.waypoint.ID, Title: "Pick a name", Status: domain.TaskDone})
	w.stub = f.task(domain.Task{ProjectID: w.waypoint.ID, ParentTaskID: &w.spec.ID, Title: "Stub the outline", Status: domain.TaskInProgress})
	f.task(domain.Task{ProjectID: w.waypoint.ID, Title: "Use Postgres", Status: domain.TaskDropped})
	w.polish = f.task(domain.Task{ProjectID: w.waypoint.ID, MilestoneID: &w.alpha.ID, Title: "Polish the alpha", Status: domain.TaskTodo})
	w.piece = f.task(domain.Task{ProjectID: w.piano.ID, Title: "Learn a scale", Status: domain.TaskTodo})
	f.task(domain.Task{ProjectID: w.done.ID, Title: "Finish it", Status: domain.TaskTodo})

	w.driver = f.blocker(domain.Blocker{ProjectID: w.waypoint.ID, TaskID: &w.spec.ID, Description: "waiting on the driver", Status: domain.BlockerOpen})
	f.blocker(domain.Blocker{ProjectID: w.waypoint.ID, Description: "no laptop", Status: domain.BlockerResolved})

	w.themes = f.idea(domain.Idea{ProjectID: &w.waypoint.ID, Title: "Dark theme", Status: domain.IdeaParked})
	w.garden = f.idea(domain.Idea{Title: "Grow tomatoes", Status: domain.IdeaParked})
	f.idea(domain.Idea{Title: "Promoted one", Status: domain.IdeaPromoted})
	f.idea(domain.Idea{Title: "Discarded one", Status: domain.IdeaDiscarded})

	w.early = f.entry("early", &w.waypoint.ID)
	f.observe(w.early, "first run")
	f.observe(w.early, "second run a", "second run b")
	w.loose = f.entry("loose", nil)
	w.shedDay = f.entry("shed day", &w.shed.ID)
	f.observe(w.shedDay, "shed note")
	w.fresh = f.entry("fresh", &w.waypoint.ID)
	w.current = f.entry("done: Write the spec", &w.waypoint.ID)
	w.later = f.entry("later", &w.waypoint.ID)
	f.observe(w.later, "from the future")
	return w
}

func build(t *testing.T, f fixture, recent int, e domain.Entry) extract.StateSnapshot {
	t.Helper()
	snap, err := NewSnapshotBuilder(f.s, f.c.now, recent).Build(context.Background(), e)
	if err != nil {
		t.Fatal(err)
	}
	return snap
}

func TestBuildScopedToActiveProject(t *testing.T) {
	f := newFixture(t)
	w := seed(f)
	snap := build(t, f, 5, w.current)

	want := extract.StateSnapshot{
		TakenAt: f.c.t,
		Projects: []extract.SnapshotProject{
			{
				ID: w.waypoint.ID, Title: "Build Waypoint", Status: domain.ProjectActive,
				// Open milestones only; a task in done Alpha shows no milestone.
				Milestones: []extract.SnapshotMilestone{{ID: w.mvp.ID, Title: "MVP"}, {ID: w.beta.ID, Title: "Beta"}},
				Tasks: []extract.SnapshotTask{
					{ID: w.spec.ID, Title: "Write the spec", Status: domain.TaskTodo, MilestoneID: &w.mvp.ID, UpdatedAt: w.spec.UpdatedAt},
					{ID: w.stub.ID, Title: "Stub the outline", Status: domain.TaskInProgress, ParentTaskID: &w.spec.ID, UpdatedAt: w.stub.UpdatedAt},
					{ID: w.polish.ID, Title: "Polish the alpha", Status: domain.TaskTodo, UpdatedAt: w.polish.UpdatedAt},
				},
				Blockers: []extract.SnapshotBlocker{{ID: w.driver.ID, TaskID: &w.spec.ID, Description: "waiting on the driver"}},
			},
			{
				ID: w.shed.ID, Title: "Garden shed", Status: domain.ProjectActive,
				Milestones: []extract.SnapshotMilestone{}, Tasks: []extract.SnapshotTask{}, Blockers: []extract.SnapshotBlocker{},
			},
		},
		Ideas: []extract.SnapshotIdea{
			{ID: w.themes.ID, ProjectID: &w.waypoint.ID, Title: "Dark theme"},
			{ID: w.garden.ID, Title: "Grow tomatoes"},
		},
		// Only Waypoint's entries, before the current one; the latest run of
		// a reprocessed entry, in span order.
		RecentEntries: []extract.SnapshotEntry{
			{ID: w.fresh.ID, CreatedAt: w.fresh.CreatedAt, Summaries: []string{}},
			{ID: w.early.ID, CreatedAt: w.early.CreatedAt, Summaries: []string{"second run a", "second run b"}},
		},
	}
	if !reflect.DeepEqual(snap, want) {
		t.Errorf("snapshot:\n got %+v\nwant %+v", snap, want)
	}
}

func TestBuildIncludesScopedInactiveProject(t *testing.T) {
	f := newFixture(t)
	w := seed(f)
	e := f.entry("practice", &w.piano.ID)
	snap := build(t, f, 5, e)

	var ids []int64
	for _, p := range snap.Projects {
		ids = append(ids, p.ID)
	}
	if want := []int64{w.waypoint.ID, w.shed.ID, w.piano.ID}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("projects = %v, want %v", ids, want)
	}
	piano := snap.Projects[2]
	if piano.Status != domain.ProjectParked || len(piano.Tasks) != 1 || piano.Tasks[0].ID != w.piece.ID {
		t.Errorf("piano project = %+v, want parked with task %d", piano, w.piece.ID)
	}
	if len(snap.RecentEntries) != 0 {
		t.Errorf("recent entries = %+v, want none: piano has no earlier entries", snap.RecentEntries)
	}
}

// An archived project is hidden from the snapshot, unless the entry is about
// it (reading an old entry again).
func TestBuildArchivedProject(t *testing.T) {
	f := newFixture(t)
	w := seed(f)
	old := f.project("Old shop", domain.ProjectArchived)

	projectIDs := func(snap extract.StateSnapshot) []int64 {
		var ids []int64
		for _, p := range snap.Projects {
			ids = append(ids, p.ID)
		}
		return ids
	}
	unscoped := build(t, f, 5, f.entry("today", nil))
	if got, want := projectIDs(unscoped), []int64{w.waypoint.ID, w.shed.ID}; !reflect.DeepEqual(got, want) {
		t.Errorf("unscoped projects = %v, want %v", got, want)
	}
	scoped := build(t, f, 5, f.entry("about the shop", &old.ID))
	if got, want := projectIDs(scoped), []int64{w.waypoint.ID, w.shed.ID, old.ID}; !reflect.DeepEqual(got, want) {
		t.Errorf("scoped projects = %v, want %v", got, want)
	}
}

func TestBuildUnscopedUsesAllProjectsEntries(t *testing.T) {
	f := newFixture(t)
	w := seed(f)

	// An unscoped entry sees earlier entries of any project, up to the limit.
	e := f.entry("where was I", nil)
	snap := build(t, f, 3, e)
	var ids []int64
	for _, re := range snap.RecentEntries {
		ids = append(ids, re.ID)
	}
	if want := []int64{w.later.ID, w.current.ID, w.fresh.ID}; !reflect.DeepEqual(ids, want) {
		t.Errorf("recent entries = %v, want %v", ids, want)
	}

	// Reprocessing an old unscoped entry doesn't see entries written after it.
	snap = build(t, f, 5, w.loose)
	ids = nil
	for _, re := range snap.RecentEntries {
		ids = append(ids, re.ID)
	}
	if want := []int64{w.early.ID}; !reflect.DeepEqual(ids, want) {
		t.Errorf("recent entries for the loose entry = %v, want %v", ids, want)
	}
	if len(snap.Projects) != 2 {
		t.Errorf("%d projects, want the 2 active ones", len(snap.Projects))
	}
}

func TestBuildNoRecentEntries(t *testing.T) {
	f := newFixture(t)
	w := seed(f)
	snap := build(t, f, 0, w.current)
	if snap.RecentEntries == nil || len(snap.RecentEntries) != 0 {
		t.Errorf("recent entries = %#v, want an empty slice", snap.RecentEntries)
	}
}

func TestBuildEmptyStateMarshalsEmptyLists(t *testing.T) {
	f := newFixture(t)
	e := f.entry("first ever", nil)
	snap := build(t, f, 5, e)
	got, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"taken_at":"` + f.c.t.Format(time.RFC3339Nano) + `","projects":[],"ideas":[],"recent_entries":[]}`
	if string(got) != want {
		t.Errorf("snapshot JSON = %s, want %s", got, want)
	}
}

// The mock's output over a built snapshot passes the shared validator, with
// the done: line matched to a task from the snapshot.
func TestBuildFeedsMockExtractor(t *testing.T) {
	f := newFixture(t)
	w := seed(f)
	in := extract.ExtractInput{Entry: w.current, Snapshot: build(t, f, 5, w.current)}
	res, err := mock.New(0).Extract(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if err := extract.Validate(in, res); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	var p struct {
		TaskID int64 `json:"task_id"`
	}
	if len(res.Ops) == 0 || res.Ops[0].OpType != domain.OpSetTaskStatus {
		t.Fatalf("ops = %+v, want set_task_status first", res.Ops)
	}
	if err := json.Unmarshal(res.Ops[0].Payload, &p); err != nil || p.TaskID != w.spec.ID {
		t.Errorf("set_task_status payload = %s, %v; want task %d", res.Ops[0].Payload, err, w.spec.ID)
	}
}
