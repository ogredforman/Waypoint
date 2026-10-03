package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ogredforman/waypoint/internal/changeset"
	"github.com/ogredforman/waypoint/internal/domain"
	"github.com/ogredforman/waypoint/internal/extract"
	"github.com/ogredforman/waypoint/internal/extract/mock"
	"github.com/ogredforman/waypoint/internal/store"
)

// events records what a worker emits. It is safe to use from the worker
// goroutine.
type events struct {
	mu  sync.Mutex
	got []Event
	ch  chan Event
}

func newEvents() *events { return &events{ch: make(chan Event, 16)} }

func (e *events) Notify(ev Event) {
	e.mu.Lock()
	e.got = append(e.got, ev)
	e.mu.Unlock()
	e.ch <- ev
}

func (e *events) list() []Event {
	e.mu.Lock()
	defer e.mu.Unlock()
	return slices.Clone(e.got)
}

// next waits for the next event.
func (e *events) next(t *testing.T) Event {
	t.Helper()
	select {
	case ev := <-e.ch:
		return ev
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for an event")
		return Event{}
	}
}

// fakeExtractor returns a fixed result or error, or blocks until its context
// is cancelled if block is set. during, if set, runs while it "extracts", to
// change state behind the worker's back.
type fakeExtractor struct {
	res     extract.ExtractionResult
	err     error
	block   bool
	started chan struct{}
	during  func()
	panics  bool
}

func (*fakeExtractor) Name() string { return "fake" }

func (f *fakeExtractor) Extract(ctx context.Context, _ extract.ExtractInput) (extract.ExtractionResult, error) {
	if f.during != nil {
		f.during()
	}
	if f.panics {
		panic("extractor bug")
	}
	if f.block {
		close(f.started)
		<-ctx.Done()
		return extract.ExtractionResult{}, ctx.Err()
	}
	return f.res, f.err
}

func (f fixture) worker(x extract.Extractor, n Notifier) *Worker {
	return f.workerWithLog(x, n, io.Discard)
}

func (f fixture) workerWithLog(x extract.Extractor, n Notifier, log io.Writer) *Worker {
	logger := slog.New(slog.NewTextHandler(log, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return NewWorker(f.s, NewSnapshotBuilder(f.s, f.c.now, 5), x, n, nil, logger)
}

// processNext claims and processes one pending run, as Run's loop does.
func processNext(t *testing.T, w *Worker) domain.ExtractionRun {
	t.Helper()
	run, err := w.claim(context.Background())
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	w.process(context.Background(), run)
	return run
}

func (f fixture) save(w *Worker, body string, projectID *int64) (domain.Entry, domain.ExtractionRun) {
	f.t.Helper()
	e, run, err := w.SaveEntry(context.Background(), domain.Entry{Body: body, ProjectID: projectID})
	if err != nil {
		f.t.Fatal(err)
	}
	return e, run
}

func (f fixture) run(id int64) domain.ExtractionRun {
	f.t.Helper()
	run, err := f.s.GetExtractionRun(context.Background(), id)
	if err != nil {
		f.t.Fatal(err)
	}
	return run
}

// stored is everything a run over an entry left behind.
type stored struct {
	observations []domain.Observation
	links        int
	changesets   []domain.Changeset
	questions    []domain.FollowUpQuestion
}

func (f fixture) stored(entryID int64) stored {
	f.t.Helper()
	ctx := context.Background()
	var s stored
	var err error
	if s.observations, err = f.s.ListObservationsByEntry(ctx, entryID); err != nil {
		f.t.Fatal(err)
	}
	for _, o := range s.observations {
		links, err := f.s.ListLinksByObservation(ctx, o.ID)
		if err != nil {
			f.t.Fatal(err)
		}
		s.links += len(links)
	}
	if s.changesets, err = f.s.ListChangesetsByEntry(ctx, entryID); err != nil {
		f.t.Fatal(err)
	}
	if s.questions, err = f.s.ListQuestionsByEntry(ctx, entryID); err != nil {
		f.t.Fatal(err)
	}
	return s
}

func (s stored) empty() bool {
	return len(s.observations) == 0 && s.links == 0 && len(s.changesets) == 0 && len(s.questions) == 0
}

func TestSaveEntryCreatesPendingRun(t *testing.T) {
	f := newFixture(t)
	w := f.worker(mock.New(0), nil)
	e, run := f.save(w, "feel: fine", nil)

	if e.ID == 0 || e.Body != "feel: fine" {
		t.Errorf("entry = %+v", e)
	}
	want := domain.ExtractionRun{
		ID: run.ID, EntryID: e.ID, Provider: mock.Name, Status: domain.RunPending,
		CreatedAt: f.c.now(), UpdatedAt: f.c.now(),
	}
	if !reflect.DeepEqual(run, want) {
		t.Errorf("run = %+v, want %+v", run, want)
	}
	select {
	case <-w.wake:
	default:
		t.Error("SaveEntry didn't wake the worker")
	}
}

func TestSaveEntryAnswersQuestion(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := f.worker(mock.New(0), nil)
	asked, run := f.save(w, "feel: stuck", nil)
	var q domain.FollowUpQuestion
	if err := f.s.InTx(ctx, func(tx *store.Tx) (err error) {
		q, err = tx.CreateQuestion(ctx, domain.FollowUpQuestion{
			EntryID: asked.ID, ExtractionRunID: run.ID, Text: "What's in the way?", Status: domain.QuestionOpen,
		})
		return err
	}); err != nil {
		t.Fatal(err)
	}

	reply, _, err := w.SaveEntry(ctx, domain.Entry{Body: "the parts", ReplyToQuestionID: &q.ID})
	if err != nil {
		t.Fatal(err)
	}
	got, err := f.s.GetQuestion(ctx, q.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.QuestionAnswered || got.AnswerEntryID == nil || *got.AnswerEntryID != reply.ID {
		t.Errorf("question after reply = %+v, want answered by entry %d", got, reply.ID)
	}

	// Replying again, to a missing question, or in a missing project stores
	// nothing.
	missing := int64(999)
	for name, e := range map[string]domain.Entry{
		"closed question":  {Body: "again", ReplyToQuestionID: &q.ID},
		"missing question": {Body: "again", ReplyToQuestionID: &missing},
		"missing project":  {Body: "again", ProjectID: &missing},
	} {
		_, _, err := w.SaveEntry(ctx, e)
		var ve *domain.ValidationError
		if !errors.As(err, &ve) {
			t.Errorf("%s: SaveEntry = %v, want a validation error", name, err)
		}
	}
	entries, err := f.s.ListRecentEntries(ctx, 10)
	if err != nil || len(entries) != 2 {
		t.Errorf("entries = %d, %v; want only the 2 saved", len(entries), err)
	}
}

func TestProcessReplyDoesNotRepeatQuestion(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := f.worker(mock.New(0), nil)
	asked, _ := f.save(w, "feel: stuck", nil)
	processNext(t, w)
	qs := f.stored(asked.ID).questions
	if len(qs) != 1 || qs[0].Text != "What's the next step?" {
		t.Fatalf("questions on the first entry = %+v, want the next-step question", qs)
	}

	reply, _, err := w.SaveEntry(ctx, domain.Entry{Body: "the parts", ReplyToQuestionID: &qs[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	if run := f.run(processNext(t, w).ID); run.Status != domain.RunSucceeded {
		t.Fatalf("reply run = %+v, want succeeded", run)
	}
	if got := f.stored(reply.ID).questions; len(got) != 0 {
		t.Errorf("questions on the reply = %+v, want none", got)
	}
}

func TestProcessStoresResult(t *testing.T) {
	f := newFixture(t)
	p := f.project("Build Waypoint", domain.ProjectActive)
	f.task(domain.Task{ProjectID: p.ID, Title: "Stub login", Status: domain.TaskTodo})
	ev := newEvents()
	x := mock.New(0)
	w := f.worker(x, ev)
	body := "done: stub login\nblocked: waiting on review\nfeel: good"
	e, _ := f.save(w, body, &p.ID)

	run := f.run(processNext(t, w).ID)

	if run.Status != domain.RunSucceeded || run.Error != nil || run.LatencyMS == nil || *run.LatencyMS < 0 {
		t.Fatalf("run = %+v, want succeeded with latency and no error", run)
	}

	// The stored snapshot is what the extractor saw, and the raw output is
	// what it returned for it.
	var snap extract.StateSnapshot
	if err := json.Unmarshal(run.InputSnapshot, &snap); err != nil {
		t.Fatalf("input snapshot: %v", err)
	}
	if len(snap.Projects) != 1 || snap.Projects[0].ID != p.ID || len(snap.Projects[0].Tasks) != 1 {
		t.Errorf("snapshot projects = %+v", snap.Projects)
	}
	want, err := x.Extract(context.Background(), extract.ExtractInput{Entry: e, Snapshot: snap})
	if err != nil {
		t.Fatal(err)
	}
	wantRaw, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(run.RawOutput, wantRaw) {
		t.Errorf("raw output = %s, want %s", run.RawOutput, wantRaw)
	}
	if len(want.Ops) != 2 || len(want.Questions) != 1 {
		t.Fatalf("mock result has %d ops and %d questions, want 2 and 1; test body needs updating", len(want.Ops), len(want.Questions))
	}

	got := f.stored(e.ID)

	wantObs := make([]domain.Observation, len(want.Observations))
	wantLinks := 0
	for i, o := range want.Observations {
		wantObs[i] = domain.Observation{
			EntryID: e.ID, ExtractionRunID: run.ID, Kind: o.Kind, Summary: o.Summary,
			SpanStart: o.SpanStart, SpanEnd: o.SpanEnd,
		}
		wantLinks += len(o.Links)
	}
	slices.SortStableFunc(wantObs, func(a, b domain.Observation) int { return int(a.SpanStart - b.SpanStart) })
	gotObs := make([]domain.Observation, len(got.observations))
	for i, o := range got.observations {
		o.ID, o.CreatedAt = 0, time.Time{}
		gotObs[i] = o
	}
	if !reflect.DeepEqual(gotObs, wantObs) {
		t.Errorf("observations = %+v, want %+v", gotObs, wantObs)
	}
	if got.links != wantLinks || wantLinks == 0 {
		t.Errorf("%d links, want %d (and more than 0)", got.links, wantLinks)
	}

	if len(got.changesets) != 1 {
		t.Fatalf("%d changesets, want 1", len(got.changesets))
	}
	cs := got.changesets[0]
	if cs.Source != domain.SourceExtraction || cs.ExtractionRunID == nil || *cs.ExtractionRunID != run.ID ||
		cs.Status != domain.ChangesetPending || cs.ParentChangesetID != nil {
		t.Errorf("changeset = %+v", cs)
	}
	ops, err := f.s.ListChangeOps(context.Background(), cs.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != len(want.Ops) {
		t.Fatalf("%d ops, want %d", len(ops), len(want.Ops))
	}
	for i, op := range ops {
		d := want.Ops[i]
		if op.Position != int64(i+1) || op.OpType != d.OpType || !bytes.Equal(op.Payload, d.Payload) ||
			op.Status != domain.OpPending || op.Rationale == nil || *op.Rationale != d.Rationale ||
			!reflect.DeepEqual(op.Confidence, d.Confidence) {
			t.Errorf("op %d = %+v, want position %d from %+v", i, op, i+1, d)
		}
	}

	if len(got.questions) != 1 {
		t.Fatalf("%d questions, want 1", len(got.questions))
	}
	q := got.questions[0]
	if q.EntryID != e.ID || q.ExtractionRunID != run.ID || q.Text != want.Questions[0].Text ||
		q.Reason != want.Questions[0].Reason || q.Status != domain.QuestionOpen || q.AnswerEntryID != nil {
		t.Errorf("question = %+v, want %+v", q, want.Questions[0])
	}

	wantEvents := []Event{
		{Type: EventEntryProcessed, EntryID: e.ID, RunID: run.ID},
		{Type: EventChangesetReady, EntryID: e.ID, RunID: run.ID, ChangesetID: cs.ID},
	}
	if !reflect.DeepEqual(ev.list(), wantEvents) {
		t.Errorf("events = %+v, want %+v", ev.list(), wantEvents)
	}
}

func TestProcessWithoutOpsStoresNoChangeset(t *testing.T) {
	f := newFixture(t)
	p := f.project("Build Waypoint", domain.ProjectActive)
	ev := newEvents()
	w := f.worker(mock.New(0), ev)
	e, _ := f.save(w, "feel: tired today", &p.ID)

	run := f.run(processNext(t, w).ID)

	if run.Status != domain.RunSucceeded {
		t.Fatalf("run status = %s, want succeeded", run.Status)
	}
	got := f.stored(e.ID)
	if len(got.observations) != 1 || len(got.changesets) != 0 {
		t.Errorf("%d observations and %d changesets, want 1 and 0", len(got.observations), len(got.changesets))
	}
	want := []Event{{Type: EventEntryProcessed, EntryID: e.ID, RunID: run.ID}}
	if !reflect.DeepEqual(ev.list(), want) {
		t.Errorf("events = %+v, want %+v", ev.list(), want)
	}
}

func TestProcessRecordsExtractorFailure(t *testing.T) {
	f := newFixture(t)
	ev := newEvents()
	var log bytes.Buffer
	secret := "a secret the extractor echoed"
	w := f.workerWithLog(&fakeExtractor{err: errors.New(secret)}, ev, &log)
	e, _ := f.save(w, "feel: private", nil)

	run := f.run(processNext(t, w).ID)

	if run.Status != domain.RunFailed || run.Error == nil || *run.Error != "extract: "+secret {
		t.Errorf("run = %+v, want failed with the extractor's error", run)
	}
	if run.RawOutput != nil || run.LatencyMS == nil {
		t.Errorf("raw output = %s, latency = %v; want none and set", run.RawOutput, run.LatencyMS)
	}
	if got := f.stored(e.ID); !got.empty() {
		t.Errorf("a failed run stored %+v", got)
	}
	want := []Event{{Type: EventRunFailed, EntryID: e.ID, RunID: run.ID}}
	if !reflect.DeepEqual(ev.list(), want) {
		t.Errorf("events = %+v, want %+v", ev.list(), want)
	}
	if strings.Contains(log.String(), secret) {
		t.Errorf("the extractor's error was logged: %s", log.String())
	}
}

func TestProcessRejectsInvalidOutput(t *testing.T) {
	f := newFixture(t)
	p := f.project("Build Waypoint", domain.ProjectActive)
	ev := newEvents()
	conf := 0.9
	x := &fakeExtractor{res: extract.ExtractionResult{
		// A valid observation, then one linked to a task that isn't in the
		// snapshot: none of it may be stored.
		Observations: []extract.ObservationDraft{
			{Kind: domain.ObservationNote, Summary: "Fine", SpanStart: 0, SpanEnd: 4},
			{Kind: domain.ObservationProgress, Summary: "Made up", SpanStart: 0, SpanEnd: 4,
				Links: []extract.LinkDraft{{TargetType: domain.TargetTask, TargetID: changeset.ID(999), Confidence: &conf}}},
		},
		Questions: []extract.QuestionDraft{{Text: "Next?", Reason: "No next step"}},
	}}
	w := f.worker(x, ev)
	e, _ := f.save(w, "note", &p.ID)

	run := f.run(processNext(t, w).ID)

	if run.Status != domain.RunFailed || run.Error == nil || !strings.HasPrefix(*run.Error, "validate: ") ||
		!strings.Contains(*run.Error, "not in the snapshot") {
		t.Errorf("run = %+v, want failed validation", run)
	}
	if run.RawOutput == nil {
		t.Error("the invalid output wasn't kept on the run")
	}
	if got := f.stored(e.ID); !got.empty() {
		t.Errorf("an invalid result stored %+v", got)
	}
	if got := ev.list(); len(got) != 1 || got[0].Type != EventRunFailed {
		t.Errorf("events = %+v, want one run.failed", got)
	}
}

func TestRunResumesInterruptedRuns(t *testing.T) {
	f := newFixture(t)
	ev := newEvents()
	w := f.worker(mock.New(0), ev)
	e, _ := f.save(w, "feel: fine", nil)
	// An earlier process claimed the run and died.
	claimed, err := w.claim(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	<-w.wake // drain SaveEntry's wake-up: the resume mustn't depend on it

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() { done <- w.Run(ctx) }()

	if got := ev.next(t); got.Type != EventEntryProcessed || got.RunID != claimed.ID {
		t.Errorf("event = %+v, want entry.processed for run %d", got, claimed.ID)
	}
	cancel()
	if err := <-done; err != nil {
		t.Errorf("Run = %v", err)
	}
	if run := f.run(claimed.ID); run.Status != domain.RunSucceeded {
		t.Errorf("resumed run status = %s, want succeeded", run.Status)
	}
	if runs, err := f.s.ListExtractionRunsByEntry(context.Background(), e.ID); err != nil || len(runs) != 1 {
		t.Errorf("runs = %+v, %v; want just the resumed one", runs, err)
	}
}

func TestRunProcessesEntriesSavedWhileIdle(t *testing.T) {
	f := newFixture(t)
	ev := newEvents()
	w := f.worker(mock.New(0), ev)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() { done <- w.Run(ctx) }()

	for _, body := range []string{"feel: one", "feel: two"} {
		e, run := f.save(w, body, nil)
		if got := ev.next(t); got.Type != EventEntryProcessed || got.EntryID != e.ID || got.RunID != run.ID {
			t.Errorf("event = %+v, want entry.processed for entry %d", got, e.ID)
		}
	}
	cancel()
	if err := <-done; err != nil {
		t.Errorf("Run = %v", err)
	}
}

func TestRunLeavesInterruptedRunRunning(t *testing.T) {
	f := newFixture(t)
	ev := newEvents()
	x := &fakeExtractor{block: true, started: make(chan struct{})}
	w := f.worker(x, ev)
	_, run := f.save(w, "feel: slow", nil)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() { done <- w.Run(ctx) }()
	select {
	case <-x.started:
	case <-time.After(5 * time.Second):
		t.Fatal("extraction never started")
	}
	cancel()
	if err := <-done; err != nil {
		t.Errorf("Run = %v", err)
	}

	if got := f.run(run.ID); got.Status != domain.RunRunning || got.Error != nil {
		t.Errorf("run = %+v, want still running for the next start", got)
	}
	if got := ev.list(); len(got) != 0 {
		t.Errorf("events = %+v, want none", got)
	}
}

func TestReprocess(t *testing.T) {
	f := newFixture(t)
	p := f.project("Build Waypoint", domain.ProjectActive)
	w := f.worker(mock.New(0), nil)
	ctx := context.Background()
	e, first := f.save(w, "todo: write tests\nfeel: good", &p.ID)

	if _, err := w.Reprocess(ctx, e.ID); !errors.Is(err, ErrRunInProgress) {
		t.Errorf("Reprocess with a pending run = %v, want ErrRunInProgress", err)
	}
	processNext(t, w)
	// A question to dismiss: the mock asks nothing when there's a todo line.
	f.write(func(tx *store.Tx) error {
		_, err := tx.CreateQuestion(ctx, domain.FollowUpQuestion{
			EntryID: e.ID, ExtractionRunID: first.ID, Text: "Next?", Reason: "r", Status: domain.QuestionOpen,
		})
		return err
	})
	before := f.stored(e.ID)
	if len(before.changesets) != 1 || len(before.questions) != 1 {
		t.Fatalf("before reprocess: %d changesets and %d questions, want 1 and 1", len(before.changesets), len(before.questions))
	}

	run, err := w.Reprocess(ctx, e.ID)
	if err != nil {
		t.Fatal(err)
	}
	if run.EntryID != e.ID || run.Status != domain.RunPending || run.ID == first.ID {
		t.Errorf("reprocess run = %+v, want a new pending run", run)
	}
	after := f.stored(e.ID)
	cs := after.changesets[0]
	if cs.Status != domain.ChangesetRejected || cs.ResolvedAt == nil {
		t.Errorf("old changeset = %+v, want rejected", cs)
	}
	ops, err := f.s.ListChangeOps(ctx, cs.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, op := range ops {
		if op.Status != domain.OpRejected {
			t.Errorf("old op %d status = %s, want rejected", op.Position, op.Status)
		}
	}
	if q := after.questions[0]; q.Status != domain.QuestionDismissed {
		t.Errorf("old question status = %s, want dismissed", q.Status)
	}
	if len(after.observations) != len(before.observations) {
		t.Errorf("reprocess changed observations: %d, want %d", len(after.observations), len(before.observations))
	}

	processNext(t, w)
	final := f.stored(e.ID)
	if len(final.changesets) != 2 || final.changesets[1].Status != domain.ChangesetPending {
		t.Errorf("changesets after the new run = %+v, want the old rejected and a new pending one", final.changesets)
	}

	if _, err := w.Reprocess(ctx, 999); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Reprocess of a missing entry = %v, want ErrNotFound", err)
	}
}

func TestReprocessLeavesResolvedChangesetsAlone(t *testing.T) {
	f := newFixture(t)
	p := f.project("Build Waypoint", domain.ProjectActive)
	w := f.worker(mock.New(0), nil)
	ctx := context.Background()
	e, _ := f.save(w, "todo: write tests\ntodo: write docs", &p.ID)
	processNext(t, w)

	// Accept the first op only: the second splits off into a pending child.
	cs := f.stored(e.ID).changesets[0]
	res, err := changeset.NewApplier(f.s, nil).
		Apply(ctx, cs.ID, map[int64]changeset.Decision{1: {Action: changeset.Accept}})
	if err != nil {
		t.Fatal(err)
	}
	if res.SplitOff == nil {
		t.Fatal("no split-off changeset")
	}

	if _, err := w.Reprocess(ctx, e.ID); err != nil {
		t.Fatal(err)
	}
	got := f.stored(e.ID).changesets
	if len(got) != 2 || got[0].Status != domain.ChangesetApplied || got[1].Status != domain.ChangesetRejected {
		t.Errorf("changesets = %+v, want the applied one kept and the split-off rejected", got)
	}
}

func TestProcessSkipsLinksToRowsDeletedDuringExtraction(t *testing.T) {
	f := newFixture(t)
	p := f.project("Build Waypoint", domain.ProjectActive)
	task := f.task(domain.Task{ProjectID: p.ID, Title: "Stub login", Status: domain.TaskTodo})
	conf := 0.9
	x := &fakeExtractor{
		res: extract.ExtractionResult{Observations: []extract.ObservationDraft{{
			Kind: domain.ObservationProgress, Summary: "Login stubbed", SpanStart: 0, SpanEnd: 4,
			Links: []extract.LinkDraft{
				{TargetType: domain.TargetTask, TargetID: changeset.ID(task.ID), Confidence: &conf},
				{TargetType: domain.TargetProject, TargetID: changeset.ID(p.ID), Confidence: &conf},
			},
		}}},
		// The task is in the snapshot, then deleted before the result is stored.
		during: func() {
			_, err := changeset.NewApplier(f.s, nil).
				ApplyManual(context.Background(), &changeset.DeleteTask{TaskID: changeset.ID(task.ID)})
			if err != nil {
				t.Error(err)
			}
		},
	}
	w := f.worker(x, nil)
	e, _ := f.save(w, "done", &p.ID)

	run := f.run(processNext(t, w).ID)

	if run.Status != domain.RunSucceeded {
		t.Fatalf("run = %+v, want succeeded", run)
	}
	got := f.stored(e.ID)
	if len(got.observations) != 1 || got.links != 1 {
		t.Fatalf("%d observations and %d links, want 1 and 1", len(got.observations), got.links)
	}
	links, err := f.s.ListLinksByObservation(context.Background(), got.observations[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if links[0].TargetType != domain.TargetProject {
		t.Errorf("kept link = %+v, want only the project link", links[0])
	}
}

// TestProcessStoresTempLinksPending checks that a link to an entity the
// changeset creates waits on the creating op instead of becoming a link.
func TestProcessStoresTempLinksPending(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	p := f.project("Build Waypoint", domain.ProjectActive)
	w := f.worker(mock.New(0), nil)
	e, _ := f.save(w, "feel: fine\ntodo: write docs", &p.ID)
	processNext(t, w)

	got := f.stored(e.ID)
	if len(got.changesets) != 1 || len(got.observations) != 2 {
		t.Fatalf("%d changesets and %d observations, want 1 and 2", len(got.changesets), len(got.observations))
	}
	ops, err := f.s.ListChangeOps(ctx, got.changesets[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := f.s.ListPendingLinksByOp(ctx, ops[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	plan := got.observations[1]
	if len(pending) != 1 || pending[0].ObservationID != plan.ID || pending[0].TargetType != domain.TargetTask {
		t.Errorf("pending links on the create_task op = %+v, want one from the plan observation %d", pending, plan.ID)
	}
	links, err := f.s.ListLinksByObservation(ctx, plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 1 || links[0].TargetType != domain.TargetProject {
		t.Errorf("links = %+v, want only the project link until the op is applied", links)
	}
}

func TestProcessRollsBackWhenStoringFails(t *testing.T) {
	f := newFixture(t)
	p := f.project("Build Waypoint", domain.ProjectActive)
	ev := newEvents()
	w := f.worker(mock.New(0), ev)
	e, _ := f.save(w, "blocked: waiting on review\nfeel: meh", &p.ID)
	// The result is valid, but the database refuses its questions, which are
	// stored after the observations and changeset.
	if _, err := f.sqlDB.Exec(`CREATE TRIGGER no_questions BEFORE INSERT ON follow_up_questions
		BEGIN SELECT RAISE(ABORT, 'no questions today'); END`); err != nil {
		t.Fatal(err)
	}

	run := f.run(processNext(t, w).ID)

	if run.Status != domain.RunFailed || run.Error == nil || !strings.HasPrefix(*run.Error, "store results: ") {
		t.Errorf("run = %+v, want failed storing results", run)
	}
	if run.RawOutput == nil {
		t.Error("the valid output wasn't kept on the failed run")
	}
	if got := f.stored(e.ID); !got.empty() {
		t.Errorf("a failed store left %+v", got)
	}
	if got := ev.list(); len(got) != 1 || got[0].Type != EventRunFailed {
		t.Errorf("events = %+v, want one run.failed", got)
	}
}

func TestProcessRecoversFromExtractorPanic(t *testing.T) {
	f := newFixture(t)
	w := f.worker(&fakeExtractor{panics: true}, nil)
	f.save(w, "feel: fine", nil)

	run := f.run(processNext(t, w).ID)

	if run.Status != domain.RunFailed || run.Error == nil || *run.Error != "extract: extractor panicked" {
		t.Errorf("run = %+v, want failed with a panic error", run)
	}
}

// processed records the entries an AfterRun was told about, and whether its
// context could be cancelled.
type processed struct {
	ids         []int64
	cancellable bool
}

func (p *processed) EntryProcessed(ctx context.Context, e domain.Entry) {
	p.ids = append(p.ids, e.ID)
	p.cancellable = p.cancellable || ctx.Done() != nil
}

func TestProcessCallsAfterRunOnSuccessOnly(t *testing.T) {
	f := newFixture(t)
	var after processed
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ok := NewWorker(f.s, NewSnapshotBuilder(f.s, f.c.now, 5), mock.New(0), nil, &after, logger)
	e, _ := f.save(ok, "feel: fine", nil)
	run, err := ok.claim(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ok.process(ctx, run)
	if !slices.Equal(after.ids, []int64{e.ID}) {
		t.Errorf("AfterRun got %v, want [%d]", after.ids, e.ID)
	}
	if after.cancellable {
		t.Error("AfterRun's context can be cancelled")
	}

	after.ids = nil
	failing := NewWorker(f.s, NewSnapshotBuilder(f.s, f.c.now, 5), &fakeExtractor{err: errors.New("down")}, nil, &after, logger)
	f.save(failing, "feel: fine", nil)
	processNext(t, failing)
	if len(after.ids) != 0 {
		t.Errorf("AfterRun called for a failed run: %v", after.ids)
	}
}

// The worker keeps only as many questions as the settings allow; the raw
// output keeps them all.
func TestProcessKeepsQuestionsPerEntrySetting(t *testing.T) {
	for _, limit := range []int{0, 1, 2} {
		t.Run(strconv.Itoa(limit), func(t *testing.T) {
			f := newFixture(t)
			ctx := context.Background()
			err := f.s.InTx(ctx, func(tx *store.Tx) error {
				_, err := tx.UpdateSettings(ctx, domain.Settings{ActiveProjectCap: 3, QuestionsPerEntry: limit, InterfaceFont: domain.FontInter, DiaryFont: domain.DiaryLiterata})
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
			x := &fakeExtractor{res: extract.ExtractionResult{
				Questions: []extract.QuestionDraft{
					{Text: "First?", Reason: "One"},
					{Text: "Second?", Reason: "Two"},
				},
			}}
			w := f.worker(x, newEvents())
			e, _ := f.save(w, "note", nil)
			run := f.run(processNext(t, w).ID)
			if run.Status != domain.RunSucceeded {
				t.Fatalf("run = %+v, want succeeded", run)
			}
			got := f.stored(e.ID).questions
			if len(got) != limit {
				t.Fatalf("%d questions stored, want %d", len(got), limit)
			}
			if limit > 0 && got[0].Text != "First?" {
				t.Errorf("kept %q, want the first question", got[0].Text)
			}
			if run.RawOutput == nil || !strings.Contains(string(run.RawOutput), "Second?") {
				t.Error("the raw output lost the dropped question")
			}
		})
	}
}
