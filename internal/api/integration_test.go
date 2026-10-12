package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ogredforman/waypoint/internal/api/apitypes"
	"github.com/ogredforman/waypoint/internal/briefing"
	"github.com/ogredforman/waypoint/internal/changeset"
	"github.com/ogredforman/waypoint/internal/db"
	"github.com/ogredforman/waypoint/internal/domain"
	"github.com/ogredforman/waypoint/internal/extract"
	"github.com/ogredforman/waypoint/internal/extract/mock"
	"github.com/ogredforman/waypoint/internal/pipeline"
	"github.com/ogredforman/waypoint/internal/store"
)

// app is the whole server wired as cmd/waypoint wires it (hub, store,
// briefings, applier, and a running worker) on a real HTTP server, with one
// SSE stream open from the start.
type app struct {
	t      *testing.T
	srv    *httptest.Server
	stream *stream
	logs   *bytes.Buffer
	stop   func()
}

// startApp runs the app on the database at dbPath with extractor ext. The
// stream is connected before the worker starts, so it sees every event,
// including those of runs resumed at startup.
func startApp(t *testing.T, dbPath string, ext extract.Extractor) *app {
	t.Helper()
	ctx := context.Background()
	sqlDB, err := db.Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	fsys, err := db.Migrations()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.Migrate(ctx, sqlDB, fsys); err != nil {
		t.Fatal(err)
	}

	logger, logs := jsonLogger()
	hub := NewHub(logger)
	st := store.New(sqlDB, time.Now)
	snapshots := pipeline.NewSnapshotBuilder(st, time.Now, 5)
	briefings := briefing.NewService(st, briefing.Heuristic{}, time.Now, hub, logger)
	if err := st.SeedSettings(context.Background(), domain.Settings{ActiveProjectCap: activeCap, QuestionsPerEntry: domain.MaxQuestionsPerEntry, Theme: domain.ThemeMidnight, InterfaceFont: domain.FontInter, DiaryFont: domain.DiaryLiterata}); err != nil {
		t.Fatal(err)
	}
	applier := changeset.NewApplier(st, changeset.AfterApplies{briefings, hub})
	worker := pipeline.NewWorker(st, snapshots, ext, hub, briefings, logger)
	srv := httptest.NewServer(profileRouter(ProfileDeps{
		Logger: logger, Hub: hub,
		Store: st, Applier: applier, Worker: worker, Briefings: briefings,
	}))
	stream := openStream(t, srv)
	waitCount(t, hub, 1)

	runCtx, cancel := context.WithCancel(ctx)
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		if err := worker.Run(runCtx); err != nil {
			t.Errorf("worker.Run = %v", err)
		}
	}()

	var once sync.Once
	stop := func() {
		once.Do(func() {
			cancel()
			<-workerDone
			hub.Close()
			srv.Close()
			if err := sqlDB.Close(); err != nil {
				t.Errorf("close db: %v", err)
			}
		})
	}
	t.Cleanup(stop)
	return &app{t: t, srv: srv, stream: stream, logs: logs, stop: stop}
}

// send makes a request to the app over HTTP, checks the status, and decodes
// the response into T.
func send[T any](a *app, status int, method, target string, body any) T {
	a.t.Helper()
	var r io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			a.t.Fatal(err)
		}
		r = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, a.srv.URL+target, r)
	if err != nil {
		a.t.Fatal(err)
	}
	resp, err := a.srv.Client().Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		a.t.Fatal(err)
	}
	if resp.StatusCode != status {
		a.t.Fatalf("%s %s: status = %d, want %d; body %s", method, target, resp.StatusCode, status, raw)
	}
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		a.t.Fatalf("%s %s: decode %s: %v", method, target, raw, err)
	}
	return v
}

// event waits for the next event named name, skipping keepalives and other
// events, and decodes its data into T.
func event[T any](a *app, name string) T {
	a.t.Helper()
	for {
		f := a.stream.next(a.t)
		if f.event != name {
			continue
		}
		var v T
		if err := json.Unmarshal([]byte(f.data), &v); err != nil {
			a.t.Fatalf("event %s: decode %q: %v", name, f.data, err)
		}
		return v
	}
}

// processed waits for the entry's run to succeed and for the briefing
// refreshes that follow it, one per project in briefed, and returns the
// pending changeset's ID (0 if the run proposed nothing).
func (a *app) processed(entryID, runID int64, wantChangeset bool, briefed ...int64) int64 {
	a.t.Helper()
	pe := event[apitypes.EntryProcessedEvent](a, string(pipeline.EventEntryProcessed))
	if pe.EntryID != entryID || pe.RunID != runID {
		a.t.Fatalf("entry.processed = %+v, want entry %d run %d", pe, entryID, runID)
	}
	var csID int64
	if wantChangeset {
		ce := event[apitypes.ChangesetReadyEvent](a, string(pipeline.EventChangesetReady))
		if ce.EntryID != entryID || ce.RunID != runID || ce.ChangesetID == 0 {
			a.t.Fatalf("changeset.ready = %+v, want entry %d run %d", ce, entryID, runID)
		}
		csID = ce.ChangesetID
	}
	var got []int64
	for range briefed {
		got = append(got, event[apitypes.BriefingUpdatedEvent](a, eventBriefingUpdated).ProjectID)
	}
	slices.Sort(got)
	if want := slices.Sorted(slices.Values(briefed)); !slices.Equal(got, want) {
		a.t.Fatalf("briefing.updated for projects %v, want %v", got, want)
	}
	return csID
}

// acceptAll applies every op of the changeset and waits for the project's
// briefing to be updated.
func (a *app) acceptAll(cs apitypes.Changeset, projectID int64) apitypes.ApplyResponse {
	a.t.Helper()
	var req apitypes.ApplyRequest
	for _, op := range cs.Ops {
		req.Ops = append(req.Ops, apitypes.OpDecision{ID: op.ID, Decision: apitypes.DecisionAccept})
	}
	res := send[apitypes.ApplyResponse](a, http.StatusOK, "POST", fmt.Sprintf("/api/profiles/t/changesets/%d/apply", cs.ID), req)
	if res.Changeset.Status != domain.ChangesetApplied || res.SplitOff != nil {
		a.t.Fatalf("apply = %+v, want applied with nothing split off", res)
	}
	// The applier refreshes briefings before the response, so the event is
	// already on its way.
	if e := event[apitypes.BriefingUpdatedEvent](a, eventBriefingUpdated); e.ProjectID != projectID {
		a.t.Fatalf("briefing.updated for project %d, want %d", e.ProjectID, projectID)
	}
	return res
}

// noLeaks checks, after the app has stopped, that no line of an entry body
// reached the logs. Lines are checked one by one because the JSON log
// escapes a body's line breaks. The logger is at debug level, so this is
// stricter than the rule (info and above): a body at any level fails it.
func (a *app) noLeaks(bodies ...string) {
	a.t.Helper()
	a.stop()
	for _, b := range bodies {
		for _, line := range strings.Split(b, "\n") {
			if line != "" && bytes.Contains(a.logs.Bytes(), []byte(line)) {
				a.t.Errorf("entry body line %q leaked into logs", line)
			}
		}
	}
}

func newProject(a *app, title string) apitypes.Project {
	a.t.Helper()
	return send[apitypes.Project](a, http.StatusCreated, "POST", "/api/profiles/t/projects",
		apitypes.CreateProjectRequest{Title: title, Status: ptr(domain.ProjectActive)})
}

func ptr[T any](v T) *T { return &v }

func taskTitles(ts []apitypes.Task) []string {
	var titles []string
	for _, t := range ts {
		titles = append(titles, fmt.Sprintf("%s (%s)", t.Title, t.Status))
	}
	return titles
}

func TestE2ECoreLoop(t *testing.T) {
	a := startApp(t, filepath.Join(t.TempDir(), "waypoint.db"), mock.New(0))
	p := newProject(a, "Waypoint")
	event[apitypes.BriefingUpdatedEvent](a, eventBriefingUpdated)

	body := "Good session on the pipeline.\ntodo: Write the API tests\ntodo: Draft the README\nblocked: waiting on review\nidea: nightly export\nfeel: steady"
	created := send[apitypes.CreateEntryResponse](a, http.StatusAccepted, "POST", "/api/profiles/t/entries",
		apitypes.CreateEntryRequest{Body: body, ProjectID: &p.ID})
	if created.Run.Status != domain.RunPending {
		t.Errorf("new run status = %s, want pending", created.Run.Status)
	}
	csID := a.processed(created.Entry.ID, created.Run.ID, true, p.ID)

	d := send[apitypes.EntryDetail](a, http.StatusOK, "GET", fmt.Sprintf("/api/profiles/t/entries/%d", created.Entry.ID), nil)
	if d.Run == nil || d.Run.Status != domain.RunSucceeded {
		t.Fatalf("run = %+v, want succeeded", d.Run)
	}
	var kinds []domain.ObservationKind
	for _, o := range d.Observations {
		kinds = append(kinds, o.Kind)
	}
	for _, k := range []domain.ObservationKind{domain.ObservationNote, domain.ObservationBlocker, domain.ObservationIdea, domain.ObservationFeeling} {
		if !slices.Contains(kinds, k) {
			t.Errorf("observation kinds = %v, want %s among them", kinds, k)
		}
	}
	if len(d.Questions) != 0 {
		t.Errorf("questions = %+v, want none for an entry with todos", d.Questions)
	}
	if len(d.Changesets) != 1 || d.Changesets[0].ID != csID || d.Changesets[0].Status != domain.ChangesetPending {
		t.Fatalf("changesets = %+v, want pending changeset %d", d.Changesets, csID)
	}
	var ops []domain.OpType
	for _, op := range d.Changesets[0].Ops {
		ops = append(ops, op.OpType)
	}
	wantOps := []domain.OpType{domain.OpCreateTask, domain.OpCreateTask, domain.OpCreateBlocker, domain.OpParkIdea}
	if !slices.Equal(ops, wantOps) {
		t.Errorf("ops = %v, want %v", ops, wantOps)
	}

	// Nothing changes until the changeset is applied.
	if tasks := send[[]apitypes.Task](a, http.StatusOK, "GET", fmt.Sprintf("/api/profiles/t/projects/%d/tasks", p.ID), nil); len(tasks) != 0 {
		t.Errorf("tasks before apply = %v", taskTitles(tasks))
	}
	a.acceptAll(d.Changesets[0], p.ID)

	tasks := send[[]apitypes.Task](a, http.StatusOK, "GET", fmt.Sprintf("/api/profiles/t/projects/%d/tasks", p.ID), nil)
	if got, want := taskTitles(tasks), []string{"Write the API tests (todo)", "Draft the README (todo)"}; !slices.Equal(got, want) {
		t.Fatalf("tasks = %v, want %v", got, want)
	}
	blockers := send[[]apitypes.Blocker](a, http.StatusOK, "GET", fmt.Sprintf("/api/profiles/t/projects/%d/blockers", p.ID), nil)
	if len(blockers) != 1 || blockers[0].Description != "waiting on review" || blockers[0].Status != domain.BlockerOpen ||
		blockers[0].OriginEntryID == nil || *blockers[0].OriginEntryID != created.Entry.ID {
		t.Errorf("blockers = %+v", blockers)
	}
	ideas := send[[]apitypes.Idea](a, http.StatusOK, "GET", "/api/profiles/t/ideas?status=parked", nil)
	if len(ideas) != 1 || ideas[0].Title != "nightly export" || ideas[0].ProjectID == nil || *ideas[0].ProjectID != p.ID {
		t.Errorf("parked ideas = %+v", ideas)
	}

	// Every task shows the entry it came from, and the "todo:" line's plan
	// observation, linked when the changeset was applied.
	for _, task := range tasks {
		td := send[apitypes.TaskDetail](a, http.StatusOK, "GET", fmt.Sprintf("/api/profiles/t/tasks/%d", task.ID), nil)
		if td.OriginEntry == nil || td.OriginEntry.ID != created.Entry.ID || td.OriginEntry.Body != body {
			t.Errorf("task %q origin = %+v, want entry %d", task.Title, td.OriginEntry, created.Entry.ID)
		}
		if len(td.Observations) != 1 || td.Observations[0].Kind != domain.ObservationPlan ||
			td.Observations[0].Quote != "todo: "+task.Title {
			t.Errorf("task %q observations = %+v, want its todo line", task.Title, td.Observations)
		}
	}

	b := send[apitypes.Briefing](a, http.StatusOK, "GET", fmt.Sprintf("/api/profiles/t/projects/%d/briefing", p.ID), nil)
	if b.NextTask == nil || b.NextTask.ID != tasks[0].ID || b.NextReason == "" {
		t.Errorf("briefing next = %+v (%q), want %q", b.NextTask, b.NextReason, tasks[0].Title)
	}
	if len(b.Blockers) != 1 || b.Blockers[0].ID != blockers[0].ID {
		t.Errorf("briefing blockers = %+v, want the open blocker", b.Blockers)
	}
	if len(b.Recap) == 0 || len(b.Recap[0].Points) == 0 {
		t.Errorf("briefing recap = %+v, want the entry with its points", b.Recap)
	}

	a.noLeaks(body)
}

func TestE2EManualEditPublishesApplied(t *testing.T) {
	a := startApp(t, filepath.Join(t.TempDir(), "waypoint.db"), mock.New(0))
	p := newProject(a, "Waypoint")
	if e := event[apitypes.ChangesetAppliedEvent](a, eventChangesetApplied); !slices.Equal(e.ProjectIDs, []int64{p.ID}) || e.ChangesetID == 0 {
		t.Errorf("after create: event = %+v, want project %d", e, p.ID)
	}
	send[apitypes.Project](a, http.StatusOK, "PATCH", fmt.Sprintf("/api/profiles/t/projects/%d", p.ID), map[string]any{"title": "Waypoint v1"})
	if e := event[apitypes.ChangesetAppliedEvent](a, eventChangesetApplied); !slices.Equal(e.ProjectIDs, []int64{p.ID}) {
		t.Errorf("after edit: event = %+v, want project %d", e, p.ID)
	}
}

func TestE2EDeleteEntry(t *testing.T) {
	a := startApp(t, filepath.Join(t.TempDir(), "waypoint.db"), mock.New(0))
	p := newProject(a, "Waypoint")
	event[apitypes.BriefingUpdatedEvent](a, eventBriefingUpdated)
	created := send[apitypes.CreateEntryResponse](a, http.StatusAccepted, "POST", "/api/profiles/t/entries",
		apitypes.CreateEntryRequest{Body: "todo: Write the API tests", ProjectID: &p.ID})
	csID := a.processed(created.Entry.ID, created.Run.ID, true, p.ID)
	d := send[apitypes.EntryDetail](a, http.StatusOK, "GET", fmt.Sprintf("/api/profiles/t/entries/%d", created.Entry.ID), nil)
	for _, cs := range d.Changesets {
		if cs.ID == csID {
			a.acceptAll(cs, p.ID)
		}
	}

	target := fmt.Sprintf("/api/profiles/t/entries/%d", created.Entry.ID)
	req, err := http.NewRequest("DELETE", a.srv.URL+target, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := a.srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE entry: status = %d, want 204", resp.StatusCode)
	}
	if e := event[apitypes.EntryDeletedEvent](a, eventEntryDeleted); e.EntryID != created.Entry.ID {
		t.Errorf("entry.deleted = %+v, want entry %d", e, created.Entry.ID)
	}
	send[map[string]any](a, http.StatusNotFound, "GET", target, nil)

	tasks := send[[]apitypes.Task](a, http.StatusOK, "GET", fmt.Sprintf("/api/profiles/t/projects/%d/tasks", p.ID), nil)
	if len(tasks) != 1 || tasks[0].OriginEntryID != nil {
		t.Errorf("tasks = %+v, want the applied task kept with no origin", tasks)
	}
	a.noLeaks("todo: Write the API tests")
}

func TestE2EFollowUpEntryWithGuessedProject(t *testing.T) {
	a := startApp(t, filepath.Join(t.TempDir(), "waypoint.db"), mock.New(0))
	p := newProject(a, "Waypoint")
	other := newProject(a, "Garden")

	first := "todo: Write the API tests\ntodo: Draft the README\nblocked: waiting on review"
	c1 := send[apitypes.CreateEntryResponse](a, http.StatusAccepted, "POST", "/api/profiles/t/entries",
		apitypes.CreateEntryRequest{Body: first, ProjectID: &p.ID})
	cs1 := a.processed(c1.Entry.ID, c1.Run.ID, true, p.ID)
	d1 := send[apitypes.EntryDetail](a, http.StatusOK, "GET", fmt.Sprintf("/api/profiles/t/entries/%d", c1.Entry.ID), nil)
	if len(d1.Changesets) != 1 || d1.Changesets[0].ID != cs1 {
		t.Fatalf("changesets = %+v, want %d", d1.Changesets, cs1)
	}
	a.acceptAll(d1.Changesets[0], p.ID)

	// An unscoped entry naming Waypoint's task and blocker: the mock guesses
	// Waypoint for the new idea, and matches the task and blocker there.
	second := "done: write the api tests\nunblocked: waiting on review\nidea: nightly export"
	c2 := send[apitypes.CreateEntryResponse](a, http.StatusAccepted, "POST", "/api/profiles/t/entries",
		apitypes.CreateEntryRequest{Body: second})
	cs2 := a.processed(c2.Entry.ID, c2.Run.ID, true, p.ID, other.ID)
	d2 := send[apitypes.EntryDetail](a, http.StatusOK, "GET", fmt.Sprintf("/api/profiles/t/entries/%d", c2.Entry.ID), nil)
	if len(d2.Changesets) != 1 || d2.Changesets[0].ID != cs2 {
		t.Fatalf("changesets = %+v, want %d", d2.Changesets, cs2)
	}
	var parked bool
	for _, op := range d2.Changesets[0].Ops {
		if op.OpType != domain.OpParkIdea {
			continue
		}
		parked = true
		var payload struct {
			ProjectID int64 `json:"project_id"`
		}
		if err := json.Unmarshal(op.Payload, &payload); err != nil || payload.ProjectID != p.ID {
			t.Errorf("park_idea payload %s, want project %d", op.Payload, p.ID)
		}
		if op.Confidence == nil || *op.Confidence >= 1 {
			t.Errorf("park_idea on a guessed project has confidence %v, want below 1", op.Confidence)
		}
	}
	if !parked {
		t.Error("no park_idea op for the idea")
	}

	// No todo or doing line: the mock asks for the next step.
	if len(d2.Questions) != 1 || d2.Questions[0].Status != domain.QuestionOpen {
		t.Fatalf("questions = %+v, want one open", d2.Questions)
	}
	a.acceptAll(d2.Changesets[0], p.ID)

	tasks := send[[]apitypes.Task](a, http.StatusOK, "GET", fmt.Sprintf("/api/profiles/t/projects/%d/tasks", p.ID), nil)
	if got, want := taskTitles(tasks), []string{"Write the API tests (done)", "Draft the README (todo)"}; !slices.Equal(got, want) {
		t.Errorf("tasks = %v, want %v", got, want)
	}
	blockers := send[[]apitypes.Blocker](a, http.StatusOK, "GET", fmt.Sprintf("/api/profiles/t/projects/%d/blockers", p.ID), nil)
	if len(blockers) != 1 || blockers[0].Status != domain.BlockerResolved ||
		blockers[0].ResolvedEntryID == nil || *blockers[0].ResolvedEntryID != c2.Entry.ID {
		t.Errorf("blockers = %+v, want resolved by entry %d", blockers, c2.Entry.ID)
	}
	ideas := send[[]apitypes.Idea](a, http.StatusOK, "GET", fmt.Sprintf("/api/profiles/t/ideas?project_id=%d", p.ID), nil)
	if len(ideas) != 1 || ideas[0].Title != "nightly export" {
		t.Errorf("Waypoint's ideas = %+v", ideas)
	}
	if got := send[[]apitypes.Task](a, http.StatusOK, "GET", fmt.Sprintf("/api/profiles/t/projects/%d/tasks", other.ID), nil); len(got) != 0 {
		t.Errorf("Garden's tasks = %v, want none", taskTitles(got))
	}

	// The briefing moves on to the next task.
	b := send[apitypes.Briefing](a, http.StatusOK, "GET", fmt.Sprintf("/api/profiles/t/projects/%d/briefing", p.ID), nil)
	if b.NextTask == nil || b.NextTask.Title != "Draft the README" || len(b.Blockers) != 0 {
		t.Errorf("briefing = next %+v, blockers %+v; want Draft the README and no blockers", b.NextTask, b.Blockers)
	}

	// The unscoped entry is listed under the project it's about.
	page := send[apitypes.EntryPage](a, http.StatusOK, "GET", fmt.Sprintf("/api/profiles/t/entries?project_id=%d", p.ID), nil)
	if len(page.Entries) != 2 || page.Entries[0].ID != c2.Entry.ID {
		t.Errorf("Waypoint's entries = %+v, want both, newest first", page.Entries)
	}

	q := send[apitypes.Question](a, http.StatusOK, "POST", fmt.Sprintf("/api/profiles/t/questions/%d/dismiss", d2.Questions[0].ID), nil)
	if q.Status != domain.QuestionDismissed {
		t.Errorf("dismissed question = %+v", q)
	}

	a.noLeaks(first, second)
}

func TestE2EResumeAfterRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "waypoint.db")

	// The first process stops mid-run: its extraction never finishes.
	a1 := startApp(t, path, mock.New(time.Hour))
	p := newProject(a1, "Waypoint")
	body := "todo: Survive a restart"
	created := send[apitypes.CreateEntryResponse](a1, http.StatusAccepted, "POST", "/api/profiles/t/entries",
		apitypes.CreateEntryRequest{Body: body, ProjectID: &p.ID})
	target := fmt.Sprintf("/api/profiles/t/entries/%d", created.Entry.ID)
	deadline := time.Now().Add(5 * time.Second)
	for {
		d := send[apitypes.EntryDetail](a1, http.StatusOK, "GET", target, nil)
		if d.Run != nil && d.Run.Status == domain.RunRunning {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("run not claimed in time: %+v", d.Run)
		}
		time.Sleep(10 * time.Millisecond)
	}
	a1.stop()

	// The next process resumes the same run.
	a2 := startApp(t, path, mock.New(0))
	a2.processed(created.Entry.ID, created.Run.ID, true, p.ID)
	d := send[apitypes.EntryDetail](a2, http.StatusOK, "GET", target, nil)
	if d.Run == nil || d.Run.ID != created.Run.ID || d.Run.Status != domain.RunSucceeded {
		t.Errorf("run after restart = %+v, want run %d succeeded", d.Run, created.Run.ID)
	}
	if len(d.Changesets) != 1 || d.Changesets[0].Status != domain.ChangesetPending {
		t.Errorf("changesets = %+v, want one pending", d.Changesets)
	}

	a1.noLeaks(body)
	a2.noLeaks(body)
}

// flakyExtractor is the mock extractor, except that its first call fails.
type flakyExtractor struct {
	*mock.Extractor
	calls atomic.Int32
}

func (f *flakyExtractor) Extract(ctx context.Context, in extract.ExtractInput) (extract.ExtractionResult, error) {
	if f.calls.Add(1) == 1 {
		return extract.ExtractionResult{}, errors.New("provider unavailable")
	}
	return f.Extractor.Extract(ctx, in)
}

func TestE2EFailedRunThenReprocess(t *testing.T) {
	a := startApp(t, filepath.Join(t.TempDir(), "waypoint.db"), &flakyExtractor{Extractor: mock.New(0)})
	p := newProject(a, "Waypoint")
	body := "todo: Try again"
	created := send[apitypes.CreateEntryResponse](a, http.StatusAccepted, "POST", "/api/profiles/t/entries",
		apitypes.CreateEntryRequest{Body: body, ProjectID: &p.ID})

	fe := event[apitypes.RunFailedEvent](a, string(pipeline.EventRunFailed))
	if fe.EntryID != created.Entry.ID || fe.RunID != created.Run.ID {
		t.Fatalf("run.failed = %+v, want entry %d run %d", fe, created.Entry.ID, created.Run.ID)
	}
	target := fmt.Sprintf("/api/profiles/t/entries/%d", created.Entry.ID)
	d := send[apitypes.EntryDetail](a, http.StatusOK, "GET", target, nil)
	if d.Run == nil || d.Run.Status != domain.RunFailed || d.Run.Error == nil || len(d.Changesets) != 0 {
		t.Fatalf("entry after failure = run %+v, changesets %+v", d.Run, d.Changesets)
	}
	if d.Run.Failure == nil || *d.Run.Failure != "error" {
		t.Errorf("failure = %v, want \"error\"", d.Run.Failure)
	}

	run := send[apitypes.ExtractionRun](a, http.StatusAccepted, "POST", target+"/reprocess", nil)
	if run.ID == created.Run.ID || run.Status != domain.RunPending {
		t.Errorf("reprocess run = %+v, want a new pending run", run)
	}
	a.processed(created.Entry.ID, run.ID, true, p.ID)
	d = send[apitypes.EntryDetail](a, http.StatusOK, "GET", target, nil)
	if d.Run == nil || d.Run.ID != run.ID || d.Run.Status != domain.RunSucceeded || len(d.Changesets) != 1 || d.Run.Failure != nil {
		t.Errorf("entry after reprocess = run %+v, changesets %+v", d.Run, d.Changesets)
	}

	a.noLeaks(body)
}

// TestE2EDeleteProjectWithEntries deletes a project with its diary entries and
// checks that the clients hear about each deleted entry.
func TestE2EDeleteProjectWithEntries(t *testing.T) {
	a := startApp(t, filepath.Join(t.TempDir(), "waypoint.db"), mock.New(0))
	p := newProject(a, "Waypoint")
	event[apitypes.BriefingUpdatedEvent](a, eventBriefingUpdated)
	created := send[apitypes.CreateEntryResponse](a, http.StatusAccepted, "POST", "/api/profiles/t/entries",
		apitypes.CreateEntryRequest{Body: "todo: Write the API tests", ProjectID: &p.ID})
	a.processed(created.Entry.ID, created.Run.ID, true, p.ID)

	target := fmt.Sprintf("/api/profiles/t/projects/%d?entries=delete", p.ID)
	req, err := http.NewRequest("DELETE", a.srv.URL+target, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := a.srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE project: status = %d, want 204", resp.StatusCode)
	}
	if e := event[apitypes.ChangesetAppliedEvent](a, eventChangesetApplied); len(e.ProjectIDs) != 1 || e.ProjectIDs[0] != p.ID {
		t.Errorf("changeset.applied = %+v, want project %d", e, p.ID)
	}
	if e := event[apitypes.EntryDeletedEvent](a, eventEntryDeleted); e.EntryID != created.Entry.ID {
		t.Errorf("entry.deleted = %+v, want entry %d", e, created.Entry.ID)
	}
	send[map[string]any](a, http.StatusNotFound, "GET", fmt.Sprintf("/api/profiles/t/entries/%d", created.Entry.ID), nil)
	send[map[string]any](a, http.StatusNotFound, "GET", fmt.Sprintf("/api/profiles/t/projects/%d", p.ID), nil)
	a.noLeaks("todo: Write the API tests")
}
