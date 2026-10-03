package api

import (
	"context"
	"database/sql"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/ogredforman/waypoint/internal/api/apitypes"
	"github.com/ogredforman/waypoint/internal/domain"
	"github.com/ogredforman/waypoint/internal/store"
)

// testProfiles is an in-memory api.Profiles. profile.Manager is the real one;
// its tests cover the files and the lifecycle.
type testProfiles struct {
	mu       sync.Mutex
	list     []domain.Profile
	handlers map[string]http.Handler
	dbs      []*sql.DB // pinged by Ping
}

// oneProfile returns profiles with the single profile "t" served by h. A
// non-nil sqlDB is what Ping checks.
func oneProfile(h http.Handler, sqlDB *sql.DB) *testProfiles {
	p := &testProfiles{
		list:     []domain.Profile{{ID: "t", Name: "Test"}},
		handlers: map[string]http.Handler{"t": h},
	}
	if sqlDB != nil {
		p.dbs = append(p.dbs, sqlDB)
	}
	return p
}

// profileRouter is the full router with one profile, "t", served by a
// profile handler built from d: data routes are at /api/profiles/t/....
func profileRouter(d ProfileDeps) http.Handler {
	return NewRouter(Deps{Logger: d.Logger, Profiles: oneProfile(NewProfileHandler(d), nil)})
}

func (p *testProfiles) List() []domain.Profile {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.list)
}

func (p *testProfiles) Create(_ context.Context, name, _ string) (domain.Profile, error) {
	name, err := domain.CleanProfileName(name)
	if err != nil {
		return domain.Profile{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	np := domain.Profile{ID: domain.ProfileSlug(name), Name: name}
	p.list = append(p.list, np)
	return np, nil
}

func (p *testProfiles) Rename(_ context.Context, id, name string) (domain.Profile, error) {
	name, err := domain.CleanProfileName(name)
	if err != nil {
		return domain.Profile{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := range p.list {
		if p.list[i].ID == id {
			p.list[i].Name = name
			return p.list[i], nil
		}
	}
	return domain.Profile{}, store.ErrNotFound
}

func (p *testProfiles) Delete(_ context.Context, id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	i := slices.IndexFunc(p.list, func(q domain.Profile) bool { return q.ID == id })
	switch {
	case i < 0:
		return store.ErrNotFound
	case len(p.list) == 1:
		return domain.ErrLastProfile
	}
	p.list = slices.Delete(p.list, i, i+1)
	return nil
}

func (p *testProfiles) Serve(id string, w http.ResponseWriter, r *http.Request) bool {
	p.mu.Lock()
	h, ok := p.handlers[id]
	p.mu.Unlock()
	if !ok {
		return false
	}
	if h == nil {
		w.WriteHeader(http.StatusTeapot)
		return true
	}
	h.ServeHTTP(w, r)
	return true
}

func (p *testProfiles) Ping(ctx context.Context) error {
	for _, d := range p.dbs {
		if err := d.PingContext(ctx); err != nil {
			return err
		}
	}
	return nil
}

func newProfilesServer(t *testing.T) (*server, *testProfiles) {
	t.Helper()
	logger, logs := jsonLogger()
	ps := oneProfile(nil, nil)
	return &server{t: t, h: NewRouter(Deps{Logger: logger, Profiles: ps}), logs: logs}, ps
}

func TestProfileRoutes(t *testing.T) {
	s, _ := newProfilesServer(t)

	list := ok[[]apitypes.Profile](s, http.StatusOK, "GET", "/api/profiles", nil)
	if want := []apitypes.Profile{{ID: "t", Name: "Test"}}; !slices.Equal(list, want) {
		t.Errorf("list = %+v, want %+v", list, want)
	}

	created := ok[apitypes.Profile](s, http.StatusCreated, "POST", "/api/profiles", `{"name":" Portfolio "}`)
	if created != (apitypes.Profile{ID: "portfolio", Name: "Portfolio"}) {
		t.Errorf("created = %+v", created)
	}
	renamed := ok[apitypes.Profile](s, http.StatusOK, "PATCH", "/api/profiles/portfolio", `{"name":"Work"}`)
	if renamed != (apitypes.Profile{ID: "portfolio", Name: "Work"}) {
		t.Errorf("renamed = %+v", renamed)
	}
	if rec := s.call("DELETE", "/api/profiles/portfolio", nil); rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
		t.Errorf("delete: %d %q, want 204 and no body", rec.Code, rec.Body)
	}

	e := s.fails(http.StatusConflict, "last_profile", "DELETE", "/api/profiles/t", nil)
	if e.Message == "" {
		t.Error("last_profile has no message")
	}
	s.fails(http.StatusNotFound, "not_found", "DELETE", "/api/profiles/nope", nil)
	s.fails(http.StatusNotFound, "not_found", "PATCH", "/api/profiles/nope", `{"name":"x"}`)
	if p := s.fails(http.StatusUnprocessableEntity, "invalid", "POST", "/api/profiles", `{"name":"  "}`).Problems; len(p) != 1 || p[0].Field != "name" {
		t.Errorf("blank name problems = %+v", p)
	}
	s.fails(http.StatusUnprocessableEntity, "invalid", "POST", "/api/profiles", `{"name":"x","colour":"red"}`)
	s.fails(http.StatusBadRequest, "bad_request", "PATCH", "/api/profiles/t", `{"name":`)
	s.fails(http.StatusMethodNotAllowed, "method_not_allowed", "PUT", "/api/profiles", nil)
}

func TestProfilePaths(t *testing.T) {
	logger, _ := jsonLogger()
	ps := oneProfile(nil, nil)
	var seen []string
	ps.handlers["t"] = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, routePath(r))
		w.WriteHeader(http.StatusNoContent)
	})
	h := NewRouter(Deps{Logger: logger, Profiles: ps})

	for _, target := range []string{"/api/profiles/t/projects/3/tasks", "/api/profiles/t/events", "/api/profiles/t/entries?limit=5"} {
		if rec := do(t, h, http.MethodGet, target); rec.Code != http.StatusNoContent {
			t.Errorf("GET %s: status %d, want the profile handler's 204", target, rec.Code)
		}
	}
	if want := []string{"/projects/3/tasks", "/events", "/entries"}; !slices.Equal(seen, want) {
		t.Errorf("profile handler saw %v, want %v", seen, want)
	}

	// An unknown profile, and the paths from before profiles, are 404s.
	for _, target := range []string{"/api/profiles/nope/projects", "/api/projects", "/api/events", "/api/settings"} {
		rec := do(t, h, http.MethodGet, target)
		if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), `"not_found"`) {
			t.Errorf("GET %s: %d %s, want a not_found 404", target, rec.Code, rec.Body)
		}
	}
}

// TestProfileHandlerErrors checks the profile router's own 404 and 405,
// with Allow worked out from the path inside the profile.
func TestProfileHandlerErrors(t *testing.T) {
	s := newServer(t)
	s.fails(http.StatusNotFound, "not_found", "GET", "/api/profiles/t/nope", nil)
	rec := s.call("PUT", "/api/profiles/t/tasks/1", nil)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("PUT a task: status %d, want 405", rec.Code)
	}
	if got, want := rec.Header().Values("Allow"), []string{"GET", "PATCH", "DELETE"}; !slices.Equal(got, want) {
		t.Errorf("Allow = %v, want %v", got, want)
	}
}

func TestHealthChecksEveryProfile(t *testing.T) {
	logger, _ := jsonLogger()
	a, b := openDB(t), openDB(t)
	ps := oneProfile(nil, a)
	ps.dbs = append(ps.dbs, b)
	h := NewRouter(Deps{Logger: logger, Profiles: ps})
	if rec := do(t, h, http.MethodGet, "/api/health"); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	if rec := do(t, h, http.MethodGet, "/api/health"); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status with one profile's database closed = %d, want 503", rec.Code)
	}
}

// routePath is the path chi routes r by inside the profile handler.
func routePath(r *http.Request) string {
	return chi.RouteContext(r.Context()).RoutePath
}
