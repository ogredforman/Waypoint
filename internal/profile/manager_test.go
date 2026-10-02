package profile_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/ogredforman/waypoint/internal/api"
	"github.com/ogredforman/waypoint/internal/api/apitypes"
	"github.com/ogredforman/waypoint/internal/db"
	"github.com/ogredforman/waypoint/internal/domain"
	"github.com/ogredforman/waypoint/internal/extract/mock"
	"github.com/ogredforman/waypoint/internal/pipeline"
	"github.com/ogredforman/waypoint/internal/profile"
	"github.com/ogredforman/waypoint/internal/store"
)

// seed is the config's settings, which every new profile starts with.
var seed = domain.Settings{ActiveProjectCap: 4, QuestionsPerEntry: 1}

// env is a data dir that managers are opened on in turn, as server restarts.
type env struct {
	t       *testing.T
	dataDir string
	logs    *syncBuffer // every manager's log
}

func newEnv(t *testing.T) *env {
	return &env{t: t, dataDir: t.TempDir(), logs: &syncBuffer{}}
}

// syncBuffer is a bytes.Buffer safe for the workers' concurrent logging.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (e *env) dir() string    { return filepath.Join(e.dataDir, "profiles") }
func (e *env) legacy() string { return filepath.Join(e.dataDir, "waypoint.db") }

// app is a running manager behind the real router.
type app struct {
	t   *testing.T
	m   *profile.Manager
	srv *httptest.Server
}

// open starts a manager on the data dir, served over HTTP. It is stopped
// when the test ends, or earlier with close.
func (e *env) open() *app {
	e.t.Helper()
	m, err := profile.Open(context.Background(), profile.Options{
		Dir:                   e.dir(),
		LegacyDB:              e.legacy(),
		Seed:                  seed,
		Extractor:             mock.New(0),
		SnapshotRecentEntries: 5,
		Logger:                slog.New(slog.NewTextHandler(e.logs, nil)),
	})
	if err != nil {
		e.t.Fatal(err)
	}
	a := &app{t: e.t, m: m, srv: httptest.NewServer(api.NewRouter(api.Deps{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Profiles: m}))}
	e.t.Cleanup(a.close)
	return a
}

func (a *app) close() {
	if a.srv == nil {
		return
	}
	a.m.EndStreams()
	a.srv.Close()
	a.m.Close()
	a.srv = nil
}

// call makes a request and decodes the response into out (if not nil),
// checking the status.
func (a *app) call(status int, method, path, body string, out any) {
	a.t.Helper()
	req, err := http.NewRequest(method, a.srv.URL+path, strings.NewReader(body))
	if err != nil {
		a.t.Fatal(err)
	}
	resp, err := a.srv.Client().Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != status {
		a.t.Fatalf("%s %s: status %d, want %d; body %s", method, path, resp.StatusCode, status, raw)
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			a.t.Fatalf("%s %s: decode %s: %v", method, path, raw, err)
		}
	}
}

func (a *app) profiles() []apitypes.Profile {
	var list []apitypes.Profile
	a.call(http.StatusOK, "GET", "/api/profiles", "", &list)
	return list
}

func (a *app) projectTitles(id string) []string {
	var list []apitypes.Project
	a.call(http.StatusOK, "GET", "/api/profiles/"+id+"/projects", "", &list)
	var titles []string
	for _, p := range list {
		titles = append(titles, p.Title)
	}
	return titles
}

func exists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Stat(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	return err == nil
}

func TestFreshDataDirGetsPersonal(t *testing.T) {
	e := newEnv(t)
	a := e.open()
	if got, want := a.profiles(), []apitypes.Profile{{ID: "personal", Name: "Personal"}}; !slices.Equal(got, want) {
		t.Fatalf("profiles = %+v, want %+v", got, want)
	}
	if !exists(t, filepath.Join(e.dir(), "personal.db")) {
		t.Error("personal.db not created")
	}
	var s apitypes.Settings
	a.call(http.StatusOK, "GET", "/api/profiles/personal/settings", "", &s)
	if s.ActiveProjectCap != seed.ActiveProjectCap || s.QuestionsPerEntry != seed.QuestionsPerEntry {
		t.Errorf("settings = %+v, want the seed %+v", s, seed)
	}
	var h apitypes.Health
	a.call(http.StatusOK, "GET", "/api/health", "", &h)
}

// legacyMigrations are the migrations from before profiles.
func legacyMigrations(t *testing.T) fs.FS {
	t.Helper()
	fsys, err := db.Migrations()
	if err != nil {
		t.Fatal(err)
	}
	out := fstest.MapFS{}
	err = fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".sql") || path >= "00007" {
			return err
		}
		b, err := fs.ReadFile(fsys, path)
		out[path] = &fstest.MapFile{Data: b}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestAdoptLegacyDatabase moves a pre-profile database, with rows still only
// in its write-ahead log (as after a crash), into the profile "Personal".
func TestAdoptLegacyDatabase(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)

	// Build an old-schema database elsewhere and copy its files while it is
	// open, so the copy's rows are in the -wal file, not yet checkpointed.
	src := filepath.Join(t.TempDir(), "old.db")
	sqlDB, err := db.Open(ctx, src)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.Migrate(ctx, sqlDB, legacyMigrations(t)); err != nil {
		t.Fatal(err)
	}
	st := store.New(sqlDB, time.Now)
	if err := st.SeedSettings(ctx, domain.Settings{ActiveProjectCap: 7, QuestionsPerEntry: 0}); err != nil {
		t.Fatal(err)
	}
	if err := st.InTx(ctx, func(tx *store.Tx) error {
		_, err := tx.CreateProject(ctx, domain.Project{Title: "Old project", Status: domain.ProjectParked})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"", "-wal"} {
		b, err := os.ReadFile(src + suffix)
		if err != nil {
			t.Fatal(err)
		}
		if suffix == "-wal" && len(b) == 0 {
			t.Fatal("test setup: the write-ahead log is empty")
		}
		if err := os.WriteFile(e.legacy()+suffix, b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}

	a := e.open()
	if got, want := a.profiles(), []apitypes.Profile{{ID: "personal", Name: "Personal"}}; !slices.Equal(got, want) {
		t.Fatalf("profiles = %+v, want %+v", got, want)
	}
	if got := a.projectTitles("personal"); !slices.Equal(got, []string{"Old project"}) {
		t.Errorf("projects = %v, want the legacy database's", got)
	}
	var s apitypes.Settings
	a.call(http.StatusOK, "GET", "/api/profiles/personal/settings", "", &s)
	if s.ActiveProjectCap != 7 || s.QuestionsPerEntry != 0 {
		t.Errorf("settings = %+v, want the stored ones kept, not the seed", s)
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if exists(t, e.legacy()+suffix) {
			t.Errorf("waypoint.db%s still in the data dir", suffix)
		}
	}

	// Next start: nothing to move, same profile.
	a.close()
	a = e.open()
	if got := a.profiles(); len(got) != 1 || a.projectTitles("personal")[0] != "Old project" {
		t.Errorf("after restart: profiles %+v", got)
	}
}

// stream is an open /events response.
type stream struct {
	lines  *bufio.Scanner
	cancel context.CancelFunc
}

func (a *app) events(id string) *stream {
	a.t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	a.t.Cleanup(cancel)
	req, _ := http.NewRequestWithContext(ctx, "GET", a.srv.URL+"/api/profiles/"+id+"/events", nil)
	resp, err := a.srv.Client().Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		a.t.Fatalf("events %s: status %d", id, resp.StatusCode)
	}
	a.t.Cleanup(func() { _ = resp.Body.Close() })
	return &stream{lines: bufio.NewScanner(resp.Body), cancel: cancel}
}

// next returns the next event name on s, or "" when the stream ends.
func (s *stream) next(t *testing.T) string {
	t.Helper()
	done := make(chan string, 1)
	go func() {
		for s.lines.Scan() {
			if name, ok := strings.CutPrefix(s.lines.Text(), "event: "); ok {
				done <- name
				return
			}
		}
		done <- ""
	}()
	select {
	case name := <-done:
		return name
	case <-time.After(5 * time.Second):
		t.Fatal("no event within 5s")
		return ""
	}
}

func TestCreateRenameDelete(t *testing.T) {
	e := newEnv(t)
	a := e.open()

	var created apitypes.Profile
	a.call(http.StatusCreated, "POST", "/api/profiles", `{"name":"  My Portfolio "}`, &created)
	if created != (apitypes.Profile{ID: "my-portfolio", Name: "My Portfolio"}) {
		t.Fatalf("created = %+v", created)
	}
	path := filepath.Join(e.dir(), "my-portfolio.db")
	if !exists(t, path) || exists(t, path+".new") {
		t.Fatal("my-portfolio.db not in place")
	}
	var s apitypes.Settings
	a.call(http.StatusOK, "GET", "/api/profiles/my-portfolio/settings", "", &s)
	if s.ActiveProjectCap != seed.ActiveProjectCap || s.QuestionsPerEntry != seed.QuestionsPerEntry {
		t.Errorf("new profile's settings = %+v, want the seed %+v", s, seed)
	}

	// Names are unique ignoring case; the ID of a second "My-Portfolio!"
	// would also be taken, so an unused one is made.
	a.call(http.StatusUnprocessableEntity, "POST", "/api/profiles", `{"name":"my portfolio"}`, nil)
	a.call(http.StatusUnprocessableEntity, "PATCH", "/api/profiles/personal", `{"name":"MY PORTFOLIO"}`, nil)
	var second apitypes.Profile
	a.call(http.StatusCreated, "POST", "/api/profiles", `{"name":"My-Portfolio!"}`, &second)
	if second.ID != "my-portfolio-2" {
		t.Errorf("second ID = %q, want my-portfolio-2", second.ID)
	}

	// The profiles are separate databases.
	a.call(http.StatusCreated, "POST", "/api/profiles/my-portfolio/projects", `{"title":"Site"}`, nil)
	if got := a.projectTitles("personal"); len(got) != 0 {
		t.Errorf("personal sees %v", got)
	}
	if got := a.projectTitles("my-portfolio"); !slices.Equal(got, []string{"Site"}) {
		t.Errorf("my-portfolio projects = %v", got)
	}

	// Each profile has its own event stream.
	personal, portfolio := a.events("personal"), a.events("my-portfolio")
	a.call(http.StatusCreated, "POST", "/api/profiles/my-portfolio/projects", `{"title":"Blog"}`, nil)
	if got := portfolio.next(t); got != "changeset.applied" {
		t.Errorf("my-portfolio event = %q, want changeset.applied", got)
	}

	// Renaming keeps the ID and the file.
	var renamed apitypes.Profile
	a.call(http.StatusOK, "PATCH", "/api/profiles/my-portfolio", `{"name":"Work"}`, &renamed)
	if renamed != (apitypes.Profile{ID: "my-portfolio", Name: "Work"}) {
		t.Errorf("renamed = %+v", renamed)
	}

	// Deleting ends its stream and removes its files; the other profiles go on.
	a.call(http.StatusNoContent, "DELETE", "/api/profiles/my-portfolio", "", nil)
	if got := portfolio.next(t); got != "" {
		t.Errorf("deleted profile's stream sent %q, want it ended", got)
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if exists(t, path+suffix) {
			t.Errorf("my-portfolio.db%s still exists", suffix)
		}
	}
	a.call(http.StatusNotFound, "GET", "/api/profiles/my-portfolio/projects", "", nil)
	a.call(http.StatusNotFound, "DELETE", "/api/profiles/my-portfolio", "", nil)
	a.call(http.StatusCreated, "POST", "/api/profiles/personal/projects", `{"title":"Garden"}`, nil)
	if got := personal.next(t); got != "changeset.applied" {
		t.Errorf("personal event = %q, want changeset.applied", got)
	}

	// After a restart: same profiles, in creation order, with their names.
	a.close()
	a = e.open()
	want := []apitypes.Profile{{ID: "personal", Name: "Personal"}, {ID: "my-portfolio-2", Name: "My-Portfolio!"}}
	if got := a.profiles(); !slices.Equal(got, want) {
		t.Errorf("after restart = %+v, want %+v", got, want)
	}

	// The last profile can't be deleted.
	a.call(http.StatusNoContent, "DELETE", "/api/profiles/my-portfolio-2", "", nil)
	var body apitypes.ErrorBody
	a.call(http.StatusConflict, "DELETE", "/api/profiles/personal", "", &body)
	if body.Error.Code != "last_profile" {
		t.Errorf("code = %q, want last_profile", body.Error.Code)
	}
	if !exists(t, filepath.Join(e.dir(), "personal.db")) {
		t.Error("personal.db removed")
	}
}

func TestOpenCleansUp(t *testing.T) {
	e := newEnv(t)
	a := e.open()
	a.close()

	// A profile a crash left half-built is removed; a file whose name can't
	// be a profile ID is left alone and skipped.
	for _, name := range []string{"half.db.new", "half.db.new-wal", "Not An ID.db"} {
		if err := os.WriteFile(filepath.Join(e.dir(), name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	a = e.open()
	if got := a.profiles(); len(got) != 1 || got[0].ID != "personal" {
		t.Errorf("profiles = %+v, want only personal", got)
	}
	for name, want := range map[string]bool{"half.db.new": false, "half.db.new-wal": false, "Not An ID.db": true} {
		if got := exists(t, filepath.Join(e.dir(), name)); got != want {
			t.Errorf("%s exists = %v, want %v", name, got, want)
		}
	}
}

// TestPendingRunsResumeInEveryProfile stores entries with pending runs in two
// profiles while the server is down; on the next start both get processed.
func TestPendingRunsResumeInEveryProfile(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	a := e.open()
	a.call(http.StatusCreated, "POST", "/api/profiles", `{"name":"Work"}`, nil)
	a.close()

	entries := map[string]int64{}
	for _, id := range []string{"personal", "work"} {
		sqlDB, err := db.Open(ctx, filepath.Join(e.dir(), id+".db"))
		if err != nil {
			t.Fatal(err)
		}
		st := store.New(sqlDB, time.Now)
		w := pipeline.NewWorker(st, pipeline.NewSnapshotBuilder(st, time.Now, 5), mock.New(0), nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
		entry, _, err := w.SaveEntry(ctx, domain.Entry{Body: "todo: write the " + id + " plan"})
		if err != nil {
			t.Fatal(err)
		}
		entries[id] = entry.ID
		if err := sqlDB.Close(); err != nil {
			t.Fatal(err)
		}
	}

	a = e.open()
	for id, entryID := range entries {
		deadline := time.Now().Add(5 * time.Second)
		for {
			var d apitypes.EntryDetail
			a.call(http.StatusOK, "GET", "/api/profiles/"+id+"/entries/"+strconv.FormatInt(entryID, 10), "", &d)
			if d.Run != nil && d.Run.Status == domain.RunSucceeded {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s: run not processed: %+v", id, d.Run)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
}

// TestDuplicateNamesOnDiskWarn copies a profile file under another ID, as a
// user restoring a backup might: both start, and a warning names their IDs
// (never their names).
func TestDuplicateNamesOnDiskWarn(t *testing.T) {
	e := newEnv(t)
	a := e.open()
	a.call(http.StatusOK, "PATCH", "/api/profiles/personal", `{"name":"Secret Diary"}`, nil)
	a.close()
	b, err := os.ReadFile(filepath.Join(e.dir(), "personal.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e.dir(), "copy.db"), b, 0o600); err != nil {
		t.Fatal(err)
	}

	a = e.open()
	if got := a.profiles(); len(got) != 2 {
		t.Fatalf("profiles = %+v, want both", got)
	}
	logs := e.logs.String()
	if !strings.Contains(logs, "same name") || !strings.Contains(logs, "copy") {
		t.Errorf("no duplicate-name warning naming the IDs in:\n%s", logs)
	}
	if strings.Contains(logs, "Secret Diary") {
		t.Error("a profile name was logged")
	}
}
