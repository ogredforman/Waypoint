package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/ogredforman/waypoint/internal/api/apitypes"
	"github.com/ogredforman/waypoint/internal/briefing"
	"github.com/ogredforman/waypoint/internal/changeset"
	"github.com/ogredforman/waypoint/internal/db"
	"github.com/ogredforman/waypoint/internal/domain"
	"github.com/ogredforman/waypoint/internal/extract/mock"
	"github.com/ogredforman/waypoint/internal/pipeline"
	"github.com/ogredforman/waypoint/internal/store"
)

// server is the API over a real migrated database, with the applier,
// briefings, and a worker running the mock extractor.
type server struct {
	t       *testing.T
	h       http.Handler
	store   *store.Store
	applier *changeset.Applier
	worker  *pipeline.Worker
	logs    *bytes.Buffer
}

// activeCap is the active-project cap in tests: small, to reach it easily.
const activeCap = 2

func newServer(t *testing.T) *server {
	t.Helper()
	sqlDB := openDB(t)
	fsys, err := db.Migrations()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.Migrate(context.Background(), sqlDB, fsys); err != nil {
		t.Fatal(err)
	}
	logger, logs := jsonLogger()
	st := store.New(sqlDB, time.Now)
	briefings := briefing.NewService(st, briefing.Heuristic{}, time.Now, nil, logger)
	if err := st.SeedSettings(context.Background(), domain.Settings{ActiveProjectCap: activeCap, QuestionsPerEntry: domain.MaxQuestionsPerEntry, Theme: domain.ThemeMidnight, InterfaceFont: domain.FontInter, DiaryFont: domain.DiaryLiterata}); err != nil {
		t.Fatal(err)
	}
	applier := changeset.NewApplier(st, briefings)
	worker := pipeline.NewWorker(st, pipeline.NewSnapshotBuilder(st, time.Now, 5), mock.New(0), nil, briefings, logger)
	h := profileRouter(ProfileDeps{
		Logger: logger,
		Store:  st, Applier: applier, Worker: worker, Briefings: briefings,
	})
	return &server{t: t, h: h, store: st, applier: applier, worker: worker, logs: logs}
}

// runWorker runs the extraction worker until the test ends.
func (s *server) runWorker() {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = s.worker.Run(ctx)
	}()
	s.t.Cleanup(func() {
		cancel()
		<-done
	})
}

// call sends body (a string of JSON, or a value to encode) and returns the
// response.
func (s *server) call(method, target string, body any) *httptest.ResponseRecorder {
	s.t.Helper()
	var r *http.Request
	switch b := body.(type) {
	case nil:
		r = httptest.NewRequest(method, target, nil)
	case string:
		r = httptest.NewRequest(method, target, strings.NewReader(b))
	default:
		raw, err := json.Marshal(b)
		if err != nil {
			s.t.Fatal(err)
		}
		r = httptest.NewRequest(method, target, bytes.NewReader(raw))
	}
	rec := httptest.NewRecorder()
	s.h.ServeHTTP(rec, r)
	return rec
}

// ok sends a request, checks the status, and decodes the response into T.
func ok[T any](s *server, status int, method, target string, body any) T {
	s.t.Helper()
	rec := s.call(method, target, body)
	if rec.Code != status {
		s.t.Fatalf("%s %s: status = %d, want %d; body %s", method, target, rec.Code, status, rec.Body)
	}
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		s.t.Fatalf("%s %s: decode %s: %v", method, target, rec.Body, err)
	}
	return v
}

// fails sends a request and checks it fails with status and code.
func (s *server) fails(status int, code, method, target string, body any) apitypes.ErrorDetail {
	s.t.Helper()
	rec := s.call(method, target, body)
	var e apitypes.ErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		s.t.Fatalf("%s %s: decode %s: %v", method, target, rec.Body, err)
	}
	if rec.Code != status || e.Error.Code != code {
		s.t.Fatalf("%s %s: %d %+v, want %d %s", method, target, rec.Code, e.Error, status, code)
	}
	return e.Error
}

func (s *server) project(title string, status domain.ProjectStatus) apitypes.Project {
	s.t.Helper()
	return ok[apitypes.Project](s, http.StatusCreated, "POST", "/api/profiles/t/projects", map[string]any{"title": title, "status": status})
}

// manualChangesets counts the applied manual changesets: every edit through
// the API must leave one.
func (s *server) manualChangesets() int {
	s.t.Helper()
	cs, err := s.store.ListChangesetsByStatus(context.Background(), domain.ChangesetApplied)
	if err != nil {
		s.t.Fatal(err)
	}
	n := 0
	for _, c := range cs {
		if c.Source == domain.SourceManual {
			n++
		}
	}
	return n
}

// waitProcessed waits for the entry's latest run to finish.
func (s *server) waitProcessed(entryID int64) apitypes.EntryDetail {
	s.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		d := ok[apitypes.EntryDetail](s, http.StatusOK, "GET", fmt.Sprintf("/api/profiles/t/entries/%d", entryID), nil)
		if d.Run != nil && (d.Run.Status == domain.RunSucceeded || d.Run.Status == domain.RunFailed) {
			return d
		}
		if time.Now().After(deadline) {
			s.t.Fatalf("entry %d not processed in time: %+v", entryID, d.Run)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestProjectRoutes(t *testing.T) {
	s := newServer(t)

	parked := ok[apitypes.Project](s, http.StatusCreated, "POST", "/api/profiles/t/projects", `{"title": "Learn Go", "why": "fun"}`)
	if parked.Status != domain.ProjectParked || parked.Why != "fun" {
		t.Errorf("created project = %+v, want parked with its why", parked)
	}
	active := ok[apitypes.Project](s, http.StatusCreated, "POST", "/api/profiles/t/projects",
		`{"title": "Waypoint", "definition_of_done": "I use it", "status": "active"}`)
	if active.Status != domain.ProjectActive || active.DefinitionOfDone != "I use it" {
		t.Errorf("created active project = %+v", active)
	}
	if n := s.manualChangesets(); n != 2 {
		t.Errorf("manual changesets = %d, want 2", n)
	}

	got := ok[apitypes.Project](s, http.StatusOK, "GET", fmt.Sprintf("/api/profiles/t/projects/%d", active.ID), nil)
	if got != active {
		t.Errorf("GET project = %+v, want %+v", got, active)
	}
	list := ok[[]apitypes.Project](s, http.StatusOK, "GET", "/api/profiles/t/projects", nil)
	if len(list) != 2 {
		t.Errorf("listed %d projects, want 2", len(list))
	}

	patched := ok[apitypes.Project](s, http.StatusOK, "PATCH", fmt.Sprintf("/api/profiles/t/projects/%d", parked.ID), `{"title": "Learn Go well", "status": "active"}`)
	if patched.Title != "Learn Go well" || patched.Status != domain.ProjectActive {
		t.Errorf("patched project = %+v", patched)
	}
	// The cap is 2, and both are active now.
	e := s.fails(http.StatusConflict, "active_cap", "POST", "/api/profiles/t/projects", `{"title": "Third", "status": "active"}`)
	if !strings.Contains(e.Message, "park") {
		t.Errorf("cap message = %q, want it to suggest parking one", e.Message)
	}
	s.fails(http.StatusNotFound, "not_found", "GET", "/api/profiles/t/projects/999", nil)
	s.fails(http.StatusNotFound, "not_found", "GET", "/api/profiles/t/projects/abc", nil)
	s.fails(http.StatusNotFound, "not_found", "PATCH", "/api/profiles/t/projects/999", `{"title": "x"}`)
	s.fails(http.StatusUnprocessableEntity, "invalid", "PATCH", fmt.Sprintf("/api/profiles/t/projects/%d", parked.ID), `{"project_id": 5}`)
	s.fails(http.StatusUnprocessableEntity, "invalid", "PATCH", fmt.Sprintf("/api/profiles/t/projects/%d", parked.ID), `{"colour": "red"}`)
	s.fails(http.StatusBadRequest, "bad_request", "PATCH", fmt.Sprintf("/api/profiles/t/projects/%d", parked.ID), `{"title": `)
	s.fails(http.StatusBadRequest, "bad_request", "PATCH", fmt.Sprintf("/api/profiles/t/projects/%d", parked.ID), `[]`)
	e = s.fails(http.StatusUnprocessableEntity, "invalid", "PATCH", fmt.Sprintf("/api/profiles/t/projects/%d", parked.ID), `{}`)
	if len(e.Problems) == 0 {
		t.Errorf("empty patch: no problems listed: %+v", e)
	}
	s.fails(http.StatusUnprocessableEntity, "invalid", "POST", "/api/profiles/t/projects", `{"title": " "}`)
}

func TestTaskRoutes(t *testing.T) {
	s := newServer(t)
	p := s.project("Waypoint", domain.ProjectActive)
	tasksURL := fmt.Sprintf("/api/profiles/t/projects/%d/tasks", p.ID)

	todo := ok[apitypes.Task](s, http.StatusCreated, "POST", tasksURL, `{"title": "Write tests", "size": "s"}`)
	if todo.Status != domain.TaskTodo || todo.Size == nil || *todo.Size != domain.SizeS || todo.ProjectID != p.ID {
		t.Errorf("created task = %+v", todo)
	}
	doing := ok[apitypes.Task](s, http.StatusCreated, "POST", tasksURL, `{"title": "Write code", "status": "in_progress"}`)
	if doing.Status != domain.TaskInProgress {
		t.Errorf("created task with status = %+v", doing)
	}
	list := ok[[]apitypes.Task](s, http.StatusOK, "GET", tasksURL, nil)
	if len(list) != 2 {
		t.Errorf("listed %d tasks, want 2", len(list))
	}

	taskURL := fmt.Sprintf("/api/profiles/t/tasks/%d", todo.ID)
	patched := ok[apitypes.Task](s, http.StatusOK, "PATCH", taskURL, `{"size": null, "energy": "low", "status": "done"}`)
	if patched.Size != nil || patched.Energy == nil || *patched.Energy != domain.EnergyLow || patched.Status != domain.TaskDone {
		t.Errorf("patched task = %+v, want size cleared, low energy, done", patched)
	}
	statusOnly := ok[apitypes.Task](s, http.StatusOK, "PATCH", taskURL, `{"status": "todo"}`)
	if statusOnly.Status != domain.TaskTodo {
		t.Errorf("status-only patch = %+v", statusOnly)
	}
	s.fails(http.StatusUnprocessableEntity, "invalid", "PATCH", taskURL, `{"status": "overdue"}`)
	s.fails(http.StatusUnprocessableEntity, "invalid", "PATCH", taskURL, `{}`)
	s.fails(http.StatusUnprocessableEntity, "invalid", "PATCH", taskURL, `{"task_id": 1}`)
	s.fails(http.StatusUnprocessableEntity, "invalid", "POST", tasksURL, `{"title": "x", "milestone_id": 1}`)
	s.fails(http.StatusUnprocessableEntity, "invalid", "POST", tasksURL, `{"title": "x", "project_id": 1}`)
	s.fails(http.StatusNotFound, "not_found", "POST", "/api/profiles/t/projects/999/tasks", `{"title": "x"}`)
	s.fails(http.StatusNotFound, "not_found", "GET", "/api/profiles/t/projects/999/tasks", nil)

	detail := ok[apitypes.TaskDetail](s, http.StatusOK, "GET", taskURL, nil)
	if detail.Task.ID != todo.ID || detail.OriginEntry != nil || detail.Observations == nil || len(detail.Observations) != 0 {
		t.Errorf("task detail = %+v, want no origin and an empty observation list", detail)
	}

	rec := s.call("DELETE", taskURL, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE task: status = %d, want 204", rec.Code)
	}
	s.fails(http.StatusNotFound, "not_found", "GET", taskURL, nil)
	s.fails(http.StatusNotFound, "not_found", "DELETE", taskURL, nil)
	// create ×2 (one with a status), patch ×2, delete.
	if n := s.manualChangesets(); n != 1+2+2+1 {
		t.Errorf("manual changesets = %d, want 6", n)
	}
}

func TestMoveTask(t *testing.T) {
	s := newServer(t)
	p := s.project("Waypoint", domain.ProjectActive)
	tasksURL := fmt.Sprintf("/api/profiles/t/projects/%d/tasks", p.ID)
	newTask := func(title string, position int64, status domain.TaskStatus) apitypes.Task {
		t.Helper()
		created := ok[apitypes.Task](s, http.StatusCreated, "POST", tasksURL, map[string]any{"title": title})
		return ok[apitypes.Task](s, http.StatusOK, "PATCH", fmt.Sprintf("/api/profiles/t/tasks/%d", created.ID), map[string]any{"position": position, "status": status})
	}
	a := newTask("A", 1, domain.TaskTodo)
	newTask("Other status", 2, domain.TaskDone)
	b := newTask("B", 3, domain.TaskTodo)
	c := newTask("C", 3, domain.TaskTodo) // same position as B; after it by id
	before := s.manualChangesets()
	move := func(task apitypes.Task, dir string) map[int64]int64 {
		t.Helper()
		moved := ok[[]apitypes.Task](s, http.StatusOK, "POST", fmt.Sprintf("/api/profiles/t/tasks/%d/move", task.ID), map[string]any{"direction": dir})
		positions := map[int64]int64{}
		for _, m := range moved {
			positions[m.ID] = m.Position
		}
		return positions
	}

	// Down skips the done task and swaps A with B.
	if got, want := move(a, "down"), map[int64]int64{a.ID: 3, b.ID: 1}; !maps.Equal(got, want) {
		t.Errorf("move A down = %v, want %v", got, want)
	}
	// B is first now; A and C share position 3, so C alone steps above A.
	if got, want := move(c, "up"), map[int64]int64{c.ID: 2}; !maps.Equal(got, want) {
		t.Errorf("move C up = %v, want %v", got, want)
	}
	if n := s.manualChangesets() - before; n != 2 {
		t.Errorf("manual changesets for 2 moves = %d, want 2", n)
	}
	// At the end of its group: nothing moves and no changeset is made.
	if got := move(a, "down"); len(got) != 0 {
		t.Errorf("move last task down = %v, want nothing", got)
	}
	if n := s.manualChangesets() - before; n != 2 {
		t.Errorf("manual changesets after a no-op move = %d, want still 2", n)
	}
	list := ok[[]apitypes.Task](s, http.StatusOK, "GET", tasksURL, nil)
	var order []string
	for _, task := range list {
		if task.Status == domain.TaskTodo {
			order = append(order, task.Title)
		}
	}
	if !slices.Equal(order, []string{"B", "C", "A"}) {
		t.Errorf("todo order = %v, want [B C A]", order)
	}

	moveURL := fmt.Sprintf("/api/profiles/t/tasks/%d/move", a.ID)
	s.fails(http.StatusUnprocessableEntity, "invalid", "POST", moveURL, `{"direction": "sideways"}`)
	s.fails(http.StatusUnprocessableEntity, "invalid", "POST", moveURL, `{}`)
	s.fails(http.StatusNotFound, "not_found", "POST", "/api/profiles/t/tasks/999/move", `{"direction": "up"}`)
}

func TestTaskDetailShowsLinkedObservations(t *testing.T) {
	s := newServer(t)
	s.runWorker()
	p := s.project("Waypoint", domain.ProjectActive)
	task := ok[apitypes.Task](s, http.StatusCreated, "POST", fmt.Sprintf("/api/profiles/t/projects/%d/tasks", p.ID), `{"title": "Write tests"}`)

	created := ok[apitypes.CreateEntryResponse](s, http.StatusAccepted, "POST", "/api/profiles/t/entries",
		map[string]any{"body": "doing: Write tests", "project_id": p.ID})
	s.waitProcessed(created.Entry.ID)

	detail := ok[apitypes.TaskDetail](s, http.StatusOK, "GET", fmt.Sprintf("/api/profiles/t/tasks/%d", task.ID), nil)
	if len(detail.Observations) != 1 {
		t.Fatalf("observations = %+v, want the one from the entry", detail.Observations)
	}
	o := detail.Observations[0]
	if o.EntryID != created.Entry.ID || o.Kind != domain.ObservationProgress || o.Quote != "doing: Write tests" {
		t.Errorf("observation = %+v", o)
	}

	// Reprocessing replaces the entry's observations: only the latest run's
	// count.
	ok[apitypes.ExtractionRun](s, http.StatusAccepted, "POST", fmt.Sprintf("/api/profiles/t/entries/%d/reprocess", created.Entry.ID), nil)
	s.waitProcessed(created.Entry.ID)
	detail = ok[apitypes.TaskDetail](s, http.StatusOK, "GET", fmt.Sprintf("/api/profiles/t/tasks/%d", task.ID), nil)
	if len(detail.Observations) != 1 || detail.Observations[0].ID == o.ID {
		t.Errorf("observations after reprocess = %+v, want only the new run's", detail.Observations)
	}
}

func TestBlockerRoutes(t *testing.T) {
	s := newServer(t)
	p := s.project("Waypoint", domain.ProjectActive)
	task := ok[apitypes.Task](s, http.StatusCreated, "POST", fmt.Sprintf("/api/profiles/t/projects/%d/tasks", p.ID), `{"title": "Ship"}`)
	blockersURL := fmt.Sprintf("/api/profiles/t/projects/%d/blockers", p.ID)
	before := s.manualChangesets()
	created := ok[apitypes.Blocker](s, http.StatusCreated, "POST", blockersURL, `{"description": "waiting on parts"}`)
	if created.Description != "waiting on parts" || created.Status != domain.BlockerOpen || created.TaskID != nil || created.ProjectID != p.ID {
		t.Errorf("created blocker = %+v", created)
	}
	if got := s.manualChangesets(); got != before+1 {
		t.Errorf("manual changesets = %d, want %d", got, before+1)
	}
	onTask := ok[apitypes.Blocker](s, http.StatusCreated, "POST", blockersURL, map[string]any{"description": "no key", "task_id": task.ID})
	if onTask.TaskID == nil || *onTask.TaskID != task.ID {
		t.Errorf("blocker on task = %+v", onTask)
	}
	other := s.project("Garden", domain.ProjectParked)
	otherTask := ok[apitypes.Task](s, http.StatusCreated, "POST", fmt.Sprintf("/api/profiles/t/projects/%d/tasks", other.ID), `{"title": "Dig"}`)
	s.fails(http.StatusUnprocessableEntity, "invalid", "POST", blockersURL, `{"description": "  "}`)
	s.fails(http.StatusUnprocessableEntity, "invalid", "POST", blockersURL, map[string]any{"description": "x", "task_id": otherTask.ID})
	s.fails(http.StatusNotFound, "not_found", "POST", "/api/profiles/t/projects/999/blockers", `{"description": "x"}`)
	// Remove the blocker on the task so the checks below see one blocker.
	if rec := s.call("DELETE", fmt.Sprintf("/api/profiles/t/blockers/%d", onTask.ID), nil); rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE blocker: status = %d, want 204", rec.Code)
	}
	blockerURL := fmt.Sprintf("/api/profiles/t/blockers/%d", created.ID)

	list := ok[[]apitypes.Blocker](s, http.StatusOK, "GET", blockersURL, nil)
	if len(list) != 1 || list[0].Status != domain.BlockerOpen || list[0].TaskID != nil {
		t.Errorf("blockers = %+v", list)
	}
	moved := ok[apitypes.Blocker](s, http.StatusOK, "PATCH", blockerURL, map[string]any{"task_id": task.ID, "description": "parts"})
	if moved.TaskID == nil || *moved.TaskID != task.ID || moved.Description != "parts" {
		t.Errorf("patched blocker = %+v", moved)
	}
	cleared := ok[apitypes.Blocker](s, http.StatusOK, "PATCH", blockerURL, `{"task_id": null}`)
	if cleared.TaskID != nil {
		t.Errorf("blocker after clearing task = %+v", cleared)
	}
	s.fails(http.StatusUnprocessableEntity, "invalid", "PATCH", blockerURL, `{"status": "open"}`)
	resolved := ok[apitypes.Blocker](s, http.StatusOK, "PATCH", blockerURL, `{"status": "resolved"}`)
	if resolved.Status != domain.BlockerResolved {
		t.Errorf("resolved blocker = %+v", resolved)
	}
	s.fails(http.StatusNotFound, "not_found", "PATCH", "/api/profiles/t/blockers/999", `{"status": "resolved"}`)
	s.fails(http.StatusNotFound, "not_found", "GET", "/api/profiles/t/projects/999/blockers", nil)

	if rec := s.call("DELETE", blockerURL, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE blocker: status = %d, want 204", rec.Code)
	}
	if list := ok[[]apitypes.Blocker](s, http.StatusOK, "GET", fmt.Sprintf("/api/profiles/t/projects/%d/blockers", p.ID), nil); len(list) != 0 {
		t.Errorf("blockers after delete = %+v", list)
	}
}

func TestIdeaRoutes(t *testing.T) {
	s := newServer(t)
	p := s.project("Waypoint", domain.ProjectActive)
	park := func(projectID *int64, title string) int64 {
		t.Helper()
		op := changeset.ParkIdea{Title: title, Body: "details"}
		if projectID != nil {
			ref := changeset.ID(*projectID)
			op.ProjectID = &ref
		}
		res, err := s.applier.ApplyManual(context.Background(), op)
		if err != nil {
			t.Fatal(err)
		}
		return res.Created[1]
	}
	loose := park(nil, "Dark mode")
	tied := park(&p.ID, "Export")
	spark := park(nil, "A garden planner")

	if list := ok[[]apitypes.Idea](s, http.StatusOK, "GET", "/api/profiles/t/ideas", nil); len(list) != 3 {
		t.Errorf("parked ideas = %+v, want 3", list)
	}
	if list := ok[[]apitypes.Idea](s, http.StatusOK, "GET", fmt.Sprintf("/api/profiles/t/ideas?project_id=%d", p.ID), nil); len(list) != 1 || list[0].ID != tied {
		t.Errorf("ideas of project = %+v, want only %d", list, tied)
	}
	s.fails(http.StatusBadRequest, "bad_request", "GET", "/api/profiles/t/ideas?status=someday", nil)
	s.fails(http.StatusBadRequest, "bad_request", "GET", "/api/profiles/t/ideas?project_id=x", nil)
	s.fails(http.StatusNotFound, "not_found", "GET", "/api/profiles/t/ideas?project_id=999", nil)

	patched := ok[apitypes.Idea](s, http.StatusOK, "PATCH", fmt.Sprintf("/api/profiles/t/ideas/%d", loose), map[string]any{"title": "Dark theme", "project_id": p.ID})
	if patched.Title != "Dark theme" || patched.ProjectID == nil || *patched.ProjectID != p.ID {
		t.Errorf("patched idea = %+v", patched)
	}
	s.fails(http.StatusUnprocessableEntity, "invalid", "PATCH", fmt.Sprintf("/api/profiles/t/ideas/%d", loose), `{"status": "promoted"}`)

	// A loose idea needs a project to become a task.
	s.fails(http.StatusUnprocessableEntity, "invalid", "POST", fmt.Sprintf("/api/profiles/t/ideas/%d/promote", spark), `{"to": "task"}`)
	s.fails(http.StatusUnprocessableEntity, "invalid", "POST", fmt.Sprintf("/api/profiles/t/ideas/%d/promote", spark), `{"to": "someday"}`)
	asTask := ok[apitypes.PromoteIdeaResponse](s, http.StatusOK, "POST", fmt.Sprintf("/api/profiles/t/ideas/%d/promote", tied), `{"to": "task"}`)
	if asTask.Idea.Status != domain.IdeaPromoted || asTask.Task == nil || asTask.Task.Title != "Export" ||
		asTask.Task.Notes != "details" || asTask.Task.ProjectID != p.ID || asTask.Project != nil {
		t.Errorf("promoted to task = %+v", asTask)
	}
	s.fails(http.StatusUnprocessableEntity, "invalid", "POST", fmt.Sprintf("/api/profiles/t/ideas/%d/promote", tied), `{"to": "task"}`)
	// Parking it again would let it be promoted twice.
	s.fails(http.StatusUnprocessableEntity, "invalid", "PATCH", fmt.Sprintf("/api/profiles/t/ideas/%d", tied), `{"status": "parked"}`)
	asProject := ok[apitypes.PromoteIdeaResponse](s, http.StatusOK, "POST", fmt.Sprintf("/api/profiles/t/ideas/%d/promote", spark), `{"to": "project"}`)
	if asProject.Project == nil || asProject.Project.Title != "A garden planner" || asProject.Project.Status != domain.ProjectParked || asProject.Task != nil {
		t.Errorf("promoted to project = %+v", asProject)
	}
	if list := ok[[]apitypes.Idea](s, http.StatusOK, "GET", "/api/profiles/t/ideas?status=promoted", nil); len(list) != 2 {
		t.Errorf("promoted ideas = %+v, want 2", list)
	}

	if rec := s.call("DELETE", fmt.Sprintf("/api/profiles/t/ideas/%d", loose), nil); rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE idea: status = %d, want 204", rec.Code)
	}
	s.fails(http.StatusNotFound, "not_found", "DELETE", fmt.Sprintf("/api/profiles/t/ideas/%d", loose), nil)
}

func TestEntryRoutes(t *testing.T) {
	s := newServer(t)
	p := s.project("Waypoint", domain.ProjectActive)

	created := ok[apitypes.CreateEntryResponse](s, http.StatusAccepted, "POST", "/api/profiles/t/entries",
		map[string]any{"body": "secret diary words", "project_id": p.ID})
	if created.Entry.Body != "secret diary words" || created.Run.Status != domain.RunPending || created.Run.EntryID != created.Entry.ID {
		t.Errorf("created entry = %+v", created)
	}
	s.fails(http.StatusUnprocessableEntity, "invalid", "POST", "/api/profiles/t/entries", `{"body": "  "}`)
	s.fails(http.StatusUnprocessableEntity, "invalid", "POST", "/api/profiles/t/entries", `{"body": "x", "project_id": 999}`)
	e := s.fails(http.StatusUnprocessableEntity, "invalid", "POST", "/api/profiles/t/entries", `{"body": "x", "mood": "ok"}`)
	if len(e.Problems) != 1 || e.Problems[0].Field != "mood" {
		t.Errorf("unknown field problems = %+v", e.Problems)
	}
	// The run is still pending: nothing is processing entries in this test.
	s.fails(http.StatusConflict, "run_in_progress", "POST", fmt.Sprintf("/api/profiles/t/entries/%d/reprocess", created.Entry.ID), nil)
	s.fails(http.StatusNotFound, "not_found", "POST", "/api/profiles/t/entries/999/reprocess", nil)

	// Pages, newest first.
	var ids []int64
	for i := range 4 {
		e := ok[apitypes.CreateEntryResponse](s, http.StatusAccepted, "POST", "/api/profiles/t/entries", map[string]any{"body": fmt.Sprintf("entry %d", i)})
		ids = append(ids, e.Entry.ID)
	}
	page := ok[apitypes.EntryPage](s, http.StatusOK, "GET", "/api/profiles/t/entries?limit=2", nil)
	if len(page.Entries) != 2 || page.Entries[0].ID != ids[3] || page.NextBefore == nil || *page.NextBefore != ids[2] {
		t.Fatalf("first page = %+v", page)
	}
	page = ok[apitypes.EntryPage](s, http.StatusOK, "GET", fmt.Sprintf("/api/profiles/t/entries?limit=2&before=%d", *page.NextBefore), nil)
	if len(page.Entries) != 2 || page.Entries[0].ID != ids[1] {
		t.Fatalf("second page = %+v", page)
	}
	page = ok[apitypes.EntryPage](s, http.StatusOK, "GET", fmt.Sprintf("/api/profiles/t/entries?limit=2&before=%d", *page.NextBefore), nil)
	if len(page.Entries) != 1 || page.Entries[0].ID != created.Entry.ID || page.NextBefore != nil {
		t.Fatalf("last page = %+v", page)
	}
	page = ok[apitypes.EntryPage](s, http.StatusOK, "GET", fmt.Sprintf("/api/profiles/t/entries?project_id=%d", p.ID), nil)
	if len(page.Entries) != 1 || page.Entries[0].ID != created.Entry.ID {
		t.Errorf("entries about the project = %+v", page)
	}
	s.fails(http.StatusBadRequest, "bad_request", "GET", "/api/profiles/t/entries?limit=0", nil)
	s.fails(http.StatusBadRequest, "bad_request", "GET", "/api/profiles/t/entries?limit=101", nil)
	s.fails(http.StatusBadRequest, "bad_request", "GET", "/api/profiles/t/entries?before=-1", nil)
	s.fails(http.StatusNotFound, "not_found", "GET", "/api/profiles/t/entries?project_id=999", nil)
	s.fails(http.StatusNotFound, "not_found", "GET", "/api/profiles/t/entries/999", nil)

	if bytes.Contains(s.logs.Bytes(), []byte("secret diary")) {
		t.Error("entry body leaked into logs")
	}
}

func TestEntryDetailAndReview(t *testing.T) {
	s := newServer(t)
	s.runWorker()
	p := s.project("Waypoint", domain.ProjectActive)

	// Accents and emoji take more bytes in UTF-8 than units in UTF-16, so
	// spans after them only line up if the API converts them.
	body := "Café day ☕🙂\ntodo: Write the API tests\nblocked: no coffee left"
	created := ok[apitypes.CreateEntryResponse](s, http.StatusAccepted, "POST", "/api/profiles/t/entries", map[string]any{"body": body, "project_id": p.ID})
	d := s.waitProcessed(created.Entry.ID)

	if d.Run.Status != domain.RunSucceeded || len(d.Observations) == 0 {
		t.Fatalf("entry detail = %+v", d)
	}
	units := utf16.Encode([]rune(body))
	var quotes []string
	for _, o := range d.Observations {
		quotes = append(quotes, string(utf16.Decode(units[o.SpanStart:o.SpanEnd])))
	}
	for _, want := range []string{"Café day ☕🙂", "blocked: no coffee left"} {
		if !slices.Contains(quotes, want) {
			t.Errorf("observation quotes in UTF-16 = %q, want one to be %q", quotes, want)
		}
	}
	if len(d.Changesets) != 1 || d.Changesets[0].Status != domain.ChangesetPending || d.Changesets[0].EntryID == nil {
		t.Fatalf("changesets = %+v, want one pending", d.Changesets)
	}

	pending := ok[[]apitypes.Changeset](s, http.StatusOK, "GET", "/api/profiles/t/changesets?status=pending", nil)
	if len(pending) != 1 || !reflect.DeepEqual(pending[0], d.Changesets[0]) {
		t.Fatalf("pending changesets = %+v, want %+v", pending, d.Changesets)
	}
	cs := pending[0]
	applyURL := fmt.Sprintf("/api/profiles/t/changesets/%d/apply", cs.ID)
	s.fails(http.StatusUnprocessableEntity, "invalid", "POST", applyURL, `{"ops": [{"id": 999, "decision": "accept"}]}`)
	s.fails(http.StatusUnprocessableEntity, "invalid", "POST", applyURL,
		fmt.Sprintf(`{"ops": [{"id": %d, "decision": "maybe"}]}`, cs.Ops[0].ID))
	s.fails(http.StatusUnprocessableEntity, "invalid", "POST", applyURL, `{"ops": []}`)
	s.fails(http.StatusNotFound, "not_found", "POST", "/api/profiles/t/changesets/999/apply", `{"ops": []}`)

	// Accept the first op, leave the rest: they split off, still pending.
	res := ok[apitypes.ApplyResponse](s, http.StatusOK, "POST", applyURL,
		fmt.Sprintf(`{"ops": [{"id": %d, "decision": "accept"}]}`, cs.Ops[0].ID))
	if res.Changeset.Status != domain.ChangesetApplied || res.Changeset.ResolvedAt == nil {
		t.Errorf("applied changeset = %+v", res.Changeset)
	}
	if len(cs.Ops) > 1 && (res.SplitOff == nil || len(res.SplitOff.Ops) != len(cs.Ops)-1 || res.HeldByCap == nil) {
		t.Errorf("split off = %+v, held by cap %v", res.SplitOff, res.HeldByCap)
	}
	s.fails(http.StatusConflict, "not_pending", "POST", applyURL, fmt.Sprintf(`{"ops": [{"id": %d, "decision": "accept"}]}`, cs.Ops[0].ID))
	applied := ok[[]apitypes.Changeset](s, http.StatusOK, "GET", "/api/profiles/t/changesets?status=applied", nil)
	if !slices.ContainsFunc(applied, func(c apitypes.Changeset) bool { return c.ID == cs.ID }) {
		t.Errorf("applied changesets = %+v, want %d among them", applied, cs.ID)
	}
	s.fails(http.StatusBadRequest, "bad_request", "GET", "/api/profiles/t/changesets?status=done", nil)
	tasks := ok[[]apitypes.Task](s, http.StatusOK, "GET", fmt.Sprintf("/api/profiles/t/projects/%d/tasks", p.ID), nil)
	if len(tasks) != 1 || tasks[0].Title != "Write the API tests" {
		t.Errorf("tasks after apply = %+v", tasks)
	}

	// Reprocessing rejects the split-off changeset and runs again.
	ok[apitypes.ExtractionRun](s, http.StatusAccepted, "POST", fmt.Sprintf("/api/profiles/t/entries/%d/reprocess", created.Entry.ID), nil)
	d = s.waitProcessed(created.Entry.ID)
	if d.Run.ID == created.Run.ID {
		t.Errorf("latest run is still %d after reprocess", d.Run.ID)
	}
}

func TestApplyReportsOpsHeldByCap(t *testing.T) {
	s := newServer(t)
	ctx := context.Background()
	for i := range activeCap {
		s.project(fmt.Sprintf("Active %d", i), domain.ProjectActive)
	}
	parked := s.project("Someday", domain.ProjectParked)
	created := ok[apitypes.CreateEntryResponse](s, http.StatusAccepted, "POST", "/api/profiles/t/entries", `{"body": "let's start Someday"}`)

	// An extraction changeset that would make a third project active.
	var ops []domain.ChangeOp
	err := s.store.InTx(ctx, func(tx *store.Tx) error {
		_, stored, err := tx.CreateChangeset(ctx,
			domain.Changeset{Source: domain.SourceExtraction, ExtractionRunID: &created.Run.ID, Status: domain.ChangesetPending},
			[]domain.ChangeOp{
				{Position: 1, OpType: domain.OpCreateTask, Status: domain.OpPending,
					Payload: json.RawMessage(fmt.Sprintf(`{"project_id":%d,"title":"Plan it"}`, parked.ID))},
				{Position: 2, OpType: domain.OpUpdateProject, Status: domain.OpPending,
					Payload: json.RawMessage(fmt.Sprintf(`{"project_id":%d,"status":"active"}`, parked.ID))},
			})
		ops = stored
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	// Op IDs and positions differ, so mixing them up shows.
	if ops[1].ID == ops[1].Position {
		t.Fatalf("op %d has ID equal to its position; the test can't tell them apart", ops[1].ID)
	}

	res := ok[apitypes.ApplyResponse](s, http.StatusOK, "POST", fmt.Sprintf("/api/profiles/t/changesets/%d/apply", ops[0].ChangesetID),
		fmt.Sprintf(`{"ops": [{"id": %d, "decision": "accept"}, {"id": %d, "decision": "accept", "payload": null}]}`, ops[0].ID, ops[1].ID))
	if !slices.Equal(res.HeldByCap, []int64{ops[1].ID}) {
		t.Errorf("held_by_cap = %v, want [%d]", res.HeldByCap, ops[1].ID)
	}
	if res.SplitOff == nil || len(res.SplitOff.Ops) != 1 || res.SplitOff.Ops[0].ID != ops[1].ID || res.SplitOff.Status != domain.ChangesetPending {
		t.Errorf("split off = %+v, want pending with op %d", res.SplitOff, ops[1].ID)
	}
}

func TestQuestionRoutes(t *testing.T) {
	s := newServer(t)
	s.runWorker()

	// With no todo: or doing: line, the mock asks for the next step.
	first := ok[apitypes.CreateEntryResponse](s, http.StatusAccepted, "POST", "/api/profiles/t/entries", `{"body": "feel: stuck"}`)
	d := s.waitProcessed(first.Entry.ID)
	second := ok[apitypes.CreateEntryResponse](s, http.StatusAccepted, "POST", "/api/profiles/t/entries", `{"body": "feel: still stuck"}`)
	d2 := s.waitProcessed(second.Entry.ID)
	if len(d.Questions) == 0 || len(d2.Questions) == 0 {
		t.Fatalf("questions = %+v and %+v, want one on each entry", d.Questions, d2.Questions)
	}

	q := d.Questions[0]
	reply := ok[apitypes.CreateEntryResponse](s, http.StatusAccepted, "POST", "/api/profiles/t/entries",
		map[string]any{"body": "todo: order parts", "reply_to_question_id": q.ID})
	if reply.Entry.ReplyToQuestionID == nil || *reply.Entry.ReplyToQuestionID != q.ID {
		t.Errorf("reply = %+v", reply.Entry)
	}
	d = ok[apitypes.EntryDetail](s, http.StatusOK, "GET", fmt.Sprintf("/api/profiles/t/entries/%d", first.Entry.ID), nil)
	if got := d.Questions[0]; got.Status != domain.QuestionAnswered || got.AnswerEntryID == nil || *got.AnswerEntryID != reply.Entry.ID {
		t.Errorf("answered question = %+v", got)
	}
	s.fails(http.StatusUnprocessableEntity, "invalid", "POST", "/api/profiles/t/entries", map[string]any{"body": "again", "reply_to_question_id": q.ID})
	s.fails(http.StatusUnprocessableEntity, "invalid", "POST", fmt.Sprintf("/api/profiles/t/questions/%d/dismiss", q.ID), nil)

	dismissed := ok[apitypes.Question](s, http.StatusOK, "POST", fmt.Sprintf("/api/profiles/t/questions/%d/dismiss", d2.Questions[0].ID), nil)
	if dismissed.Status != domain.QuestionDismissed {
		t.Errorf("dismissed question = %+v", dismissed)
	}
	s.fails(http.StatusNotFound, "not_found", "POST", "/api/profiles/t/questions/999/dismiss", nil)
}

func TestBriefingRoutes(t *testing.T) {
	s := newServer(t)
	p := s.project("Waypoint", domain.ProjectActive)
	task := ok[apitypes.Task](s, http.StatusCreated, "POST", fmt.Sprintf("/api/profiles/t/projects/%d/tasks", p.ID), `{"title": "Start small", "size": "s"}`)

	b := ok[apitypes.Briefing](s, http.StatusOK, "GET", fmt.Sprintf("/api/profiles/t/projects/%d/briefing", p.ID), nil)
	if b.ProjectID != p.ID || b.NextTask == nil || b.NextTask.ID != task.ID || b.Blockers == nil || b.Alternatives == nil {
		t.Errorf("briefing = %+v", b)
	}
	refreshed := ok[apitypes.Briefing](s, http.StatusOK, "POST", fmt.Sprintf("/api/profiles/t/projects/%d/briefing/refresh", p.ID), nil)
	if refreshed.NextTask == nil || refreshed.NextTask.ID != task.ID {
		t.Errorf("refreshed briefing = %+v", refreshed)
	}
	s.fails(http.StatusNotFound, "not_found", "GET", "/api/profiles/t/projects/999/briefing", nil)
	s.fails(http.StatusNotFound, "not_found", "POST", "/api/profiles/t/projects/999/briefing/refresh", nil)

}

func TestSettingsRoutes(t *testing.T) {
	s := newServer(t)
	settings := ok[apitypes.Settings](s, http.StatusOK, "GET", "/api/profiles/t/settings", nil)
	if want := (apitypes.Settings{ActiveProjectCap: activeCap, QuestionsPerEntry: 2, Theme: "midnight", InterfaceFont: "inter", DiaryFont: "literata"}); settings != want {
		t.Errorf("settings = %+v, want %+v", settings, want)
	}

	// Absent fields stay as they are.
	settings = ok[apitypes.Settings](s, http.StatusOK, "PATCH", "/api/profiles/t/settings", `{"questions_per_entry": 0}`)
	if want := (apitypes.Settings{ActiveProjectCap: activeCap, QuestionsPerEntry: 0, Theme: "midnight", InterfaceFont: "inter", DiaryFont: "literata"}); settings != want {
		t.Errorf("after PATCH = %+v, want %+v", settings, want)
	}
	settings = ok[apitypes.Settings](s, http.StatusOK, "PATCH", "/api/profiles/t/settings", `{"active_project_cap": 5, "questions_per_entry": 1}`)
	if want := (apitypes.Settings{ActiveProjectCap: 5, QuestionsPerEntry: 1, Theme: "midnight", InterfaceFont: "inter", DiaryFont: "literata"}); settings != want {
		t.Errorf("after PATCH = %+v, want %+v", settings, want)
	}
	settings = ok[apitypes.Settings](s, http.StatusOK, "PATCH", "/api/profiles/t/settings", `{"theme": "dawn", "interface_font": "atkinson-hyperlegible-next", "diary_font": "geist"}`)
	if want := (apitypes.Settings{ActiveProjectCap: 5, QuestionsPerEntry: 1, Theme: "dawn", InterfaceFont: "atkinson-hyperlegible-next", DiaryFont: "geist"}); settings != want {
		t.Errorf("after PATCH = %+v, want %+v", settings, want)
	}
	if got := ok[apitypes.Settings](s, http.StatusOK, "GET", "/api/profiles/t/settings", nil); got != settings {
		t.Errorf("GET after PATCH = %+v, want %+v", got, settings)
	}

	for _, body := range []string{
		`{}`,
		`{"active_project_cap": 0}`,
		`{"questions_per_entry": 3}`,
		`{"questions_per_entry": -1}`,
		`{"active_project_cap": null}`,
		`{"active_project_cap": 2.5}`,
		`{"active_project_cap": "4"}`,
		`{"theme": "solarized"}`,
		`{"theme": ""}`,
		`{"theme": null}`,
		`{"theme": 1}`,
		`{"active_project_cap": 4, "port": 80}`,
		`{"interface_font": "literata"}`,
		`{"interface_font": "comic-sans"}`,
		`{"interface_font": null}`,
		`{"interface_font": 3}`,
		`{"diary_font": ""}`,
		`{"diary_font": null}`,
	} {
		e := s.fails(http.StatusUnprocessableEntity, "invalid", "PATCH", "/api/profiles/t/settings", body)
		if len(e.Problems) == 0 {
			t.Errorf("PATCH %s: no problems listed", body)
		}
	}
	s.fails(http.StatusBadRequest, "bad_request", "PATCH", "/api/profiles/t/settings", `{"active_project_cap":`)
	s.fails(http.StatusBadRequest, "bad_request", "PATCH", "/api/profiles/t/settings", `[1]`)
	if got := ok[apitypes.Settings](s, http.StatusOK, "GET", "/api/profiles/t/settings", nil); got != settings {
		t.Errorf("rejected PATCHes changed the settings: %+v, want %+v", got, settings)
	}
}

// A lower cap deactivates nothing and applies to the next activation.
func TestSettingsCapAppliesToNextActivation(t *testing.T) {
	s := newServer(t)
	a := s.project("A", domain.ProjectActive)
	s.project("B", domain.ProjectActive)
	parked := s.project("C", domain.ProjectParked)

	ok[apitypes.Settings](s, http.StatusOK, "PATCH", "/api/profiles/t/settings", `{"active_project_cap": 1}`)
	projects := ok[[]apitypes.Project](s, http.StatusOK, "GET", "/api/profiles/t/projects", nil)
	active := 0
	for _, p := range projects {
		if p.Status == domain.ProjectActive {
			active++
		}
	}
	if active != 2 {
		t.Errorf("%d active projects after lowering the cap, want 2", active)
	}
	ok[apitypes.Project](s, http.StatusOK, "PATCH", fmt.Sprintf("/api/profiles/t/projects/%d", a.ID), `{"title": "A, renamed"}`)
	s.fails(http.StatusConflict, "active_cap", "PATCH", fmt.Sprintf("/api/profiles/t/projects/%d", parked.ID), `{"status": "active"}`)

	ok[apitypes.Settings](s, http.StatusOK, "PATCH", "/api/profiles/t/settings", `{"active_project_cap": 3}`)
	ok[apitypes.Project](s, http.StatusOK, "PATCH", fmt.Sprintf("/api/profiles/t/projects/%d", parked.ID), `{"status": "active"}`)
}

// TestRequestTypesMatchOps checks that each request DTO, which only exists
// to type the frontend, lists the fields its route passes on to the op
// payload, so the generated TS types can't drift from what the route
// accepts.
func TestRequestTypesMatchOps(t *testing.T) {
	jsonFields := func(v any) []string {
		var out []string
		typ := reflect.TypeOf(v)
		for i := range typ.NumField() {
			name, _, _ := strings.Cut(typ.Field(i).Tag.Get("json"), ",")
			out = append(out, name)
		}
		slices.Sort(out)
		return out
	}
	without := func(fields []string, drop ...string) []string {
		return slices.DeleteFunc(slices.Clone(fields), func(f string) bool { return slices.Contains(drop, f) })
	}
	with := func(fields []string, add ...string) []string {
		out := append(slices.Clone(fields), add...)
		slices.Sort(out)
		return out
	}
	tests := []struct {
		name      string
		request   any
		wantField []string
	}{
		{"CreateProjectRequest", apitypes.CreateProjectRequest{},
			with(jsonFields(changeset.CreateProject{}), "definition_of_done", "status")},
		{"UpdateProjectRequest", apitypes.UpdateProjectRequest{},
			without(jsonFields(changeset.UpdateProject{}), "project_id")},
		{"CreateTaskRequest", apitypes.CreateTaskRequest{},
			with(without(jsonFields(changeset.CreateTask{}), "project_id", "milestone_id"), "status")},
		{"UpdateTaskRequest", apitypes.UpdateTaskRequest{},
			with(without(jsonFields(changeset.UpdateTask{}), "task_id"), "status")},
		{"CreateBlockerRequest", apitypes.CreateBlockerRequest{},
			without(jsonFields(changeset.CreateBlocker{}), "project_id")},
		{"UpdateBlockerRequest", apitypes.UpdateBlockerRequest{},
			with(without(jsonFields(changeset.UpdateBlocker{}), "blocker_id"), "status")},
		{"UpdateIdeaRequest", apitypes.UpdateIdeaRequest{},
			without(jsonFields(changeset.UpdateIdea{}), "idea_id")},
	}
	for _, tt := range tests {
		if got := jsonFields(tt.request); !slices.Equal(got, tt.wantField) {
			t.Errorf("%s fields = %v, want %v", tt.name, got, tt.wantField)
		}
	}
}

func TestUTF16Offset(t *testing.T) {
	s := "aé☕🙂b"
	tests := []struct{ bytes, units int64 }{
		{0, 0}, {1, 1}, {3, 2}, {6, 3}, {10, 5}, {11, 6}, {99, 6}, {-1, 0},
	}
	for _, tt := range tests {
		if got := utf16Offset(s, tt.bytes); got != tt.units {
			t.Errorf("utf16Offset(%q, %d) = %d, want %d", s, tt.bytes, got, tt.units)
		}
	}
}

func TestChangeOpRefsNameExistingRows(t *testing.T) {
	s := newServer(t)
	s.runWorker()
	p := s.project("Waypoint", domain.ProjectActive)
	task := ok[apitypes.Task](s, http.StatusCreated, "POST", fmt.Sprintf("/api/profiles/t/projects/%d/tasks", p.ID), `{"title": "Set up CI"}`)

	body := "done: Set up CI\ntodo: Write docs\ndone: Something brand new"
	created := ok[apitypes.CreateEntryResponse](s, http.StatusAccepted, "POST", "/api/profiles/t/entries", map[string]any{"body": body, "project_id": p.ID})
	d := s.waitProcessed(created.Entry.ID)
	if len(d.Changesets) != 1 {
		t.Fatalf("changesets = %+v, want one", d.Changesets)
	}
	var got []map[string]string
	for _, op := range d.Changesets[0].Ops {
		got = append(got, op.Refs)
	}
	taskKey, projectKey := fmt.Sprintf("task:%d", task.ID), fmt.Sprintf("project:%d", p.ID)
	want := []map[string]string{
		{taskKey: "Set up CI"},   // set_task_status on the existing task
		{projectKey: "Waypoint"}, // create_task "Write docs"
		{projectKey: "Waypoint"}, // create_task "Something brand new"
		{},                       // set_task_status on $new: no row yet
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("refs = %v, want %v", got, want)
	}
}
