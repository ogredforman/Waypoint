package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"testing"

	"github.com/ogredforman/waypoint/internal/api/apitypes"
	"github.com/ogredforman/waypoint/internal/db"
)

// openDB opens a real SQLite database in a temp dir.
func openDB(t *testing.T) *sql.DB {
	t.Helper()
	sqlDB, err := db.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return sqlDB
}

// jsonLogger returns a logger that writes JSON lines to the returned buffer.
func jsonLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})), &buf
}

// logLines decodes each JSON log line in buf.
func logLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var lines []map[string]any
	dec := json.NewDecoder(buf)
	for {
		var m map[string]any
		if err := dec.Decode(&m); err == io.EOF {
			return lines
		} else if err != nil {
			t.Fatal(err)
		}
		lines = append(lines, m)
	}
}

func do(t *testing.T, h http.Handler, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return rec
}

func TestHealth(t *testing.T) {
	logger, buf := jsonLogger()
	sqlDB := openDB(t)
	h := NewRouter(Deps{Logger: logger, Profiles: oneProfile(nil, sqlDB)})

	rec := do(t, h, http.MethodGet, "/api/health")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var body apitypes.Health
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "ok" {
		t.Errorf("status field = %q, want ok", body.Status)
	}

	// A database that can't be reached makes the server unhealthy.
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	rec = do(t, h, http.MethodGet, "/api/health")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status after close = %d, want 503", rec.Code)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "unavailable" {
		t.Errorf("status field after close = %q, want unavailable", body.Status)
	}
	var sawWarn bool
	for _, line := range logLines(t, buf) {
		errText, _ := line["err"].(string)
		if line["msg"] == "health: db ping failed" && line["level"] == "WARN" && errText != "" {
			sawWarn = true
		}
	}
	if !sawWarn {
		t.Error("no WARN log line for the failed ping")
	}
}

func TestRequestLogging(t *testing.T) {
	logger, buf := jsonLogger()
	h := NewRouter(Deps{Logger: logger})

	// The query string must not be logged: later endpoints may carry diary text in it.
	do(t, h, http.MethodGet, "/api/health?secret=diary")

	lines := logLines(t, buf)
	if len(lines) != 1 {
		t.Fatalf("got %d log lines, want 1: %v", len(lines), lines)
	}
	line := lines[0]
	checks := map[string]any{
		"level":  "INFO",
		"msg":    "request",
		"method": "GET",
		"path":   "/api/health",
		"status": float64(200),
	}
	for k, want := range checks {
		if line[k] != want {
			t.Errorf("log %s = %v, want %v", k, line[k], want)
		}
	}
	for _, k := range []string{"bytes", "duration", "request_id"} {
		if _, ok := line[k]; !ok {
			t.Errorf("log line missing %s: %v", k, line)
		}
	}
	if id, _ := line["request_id"].(string); id == "" {
		t.Errorf("request_id is empty")
	}
	if bytes.Contains(buf.Bytes(), []byte("diary")) {
		t.Errorf("query string leaked into logs")
	}
}

func TestPanicRecovery(t *testing.T) {
	logger, buf := jsonLogger()
	r := newMux(Deps{Logger: logger})
	// A panic value may be built from request data, so only its type is
	// logged; a runtime error's message is the runtime's own.
	r.Get("/api/boom", func(http.ResponseWriter, *http.Request) { panic("diary words") })
	r.Get("/api/index", func(_ http.ResponseWriter, r *http.Request) {
		var none []byte
		_ = none[len(r.URL.Path)]
	})

	for target, want := range map[string]string{
		"/api/boom":  "string",
		"/api/index": "runtime error: index out of range [10] with length 0",
	} {
		buf.Reset()
		rec := do(t, r, http.MethodGet, target)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("GET %s: status = %d, want 500", target, rec.Code)
		}
		var sawPanic, sawRequest bool
		for _, line := range logLines(t, buf) {
			switch line["msg"] {
			case "handler panic":
				sawPanic = line["level"] == "ERROR" && line["panic"] == want
			case "request":
				sawRequest = line["status"] == float64(500)
			}
		}
		if !sawPanic {
			t.Errorf("GET %s: no ERROR log line with panic %q", target, want)
		}
		if !sawRequest {
			t.Errorf("GET %s: request log line does not record status 500", target)
		}
	}
	do(t, r, http.MethodGet, "/api/boom")
	if bytes.Contains(buf.Bytes(), []byte("diary")) {
		t.Error("panic value leaked into logs")
	}
}

func TestMethodNotAllowedSetsAllow(t *testing.T) {
	logger, _ := jsonLogger()
	r := newMux(Deps{Logger: logger})
	r.Post("/api/things/{id}", func(http.ResponseWriter, *http.Request) {})
	r.Patch("/api/things/{id}", func(http.ResponseWriter, *http.Request) {})

	for target, want := range map[string][]string{
		"/api/health":   {http.MethodGet},
		"/api/things/3": {http.MethodPost, http.MethodPatch},
	} {
		rec := do(t, r, http.MethodDelete, target)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("DELETE %s: status = %d, want 405", target, rec.Code)
		}
		if got := rec.Header().Values("Allow"); !slices.Equal(got, want) {
			t.Errorf("DELETE %s: Allow = %v, want %v", target, got, want)
		}
	}
}

func TestErrorShape(t *testing.T) {
	logger, _ := jsonLogger()
	r := newMux(Deps{Logger: logger})
	r.Get("/api/boom", func(http.ResponseWriter, *http.Request) { panic("boom") })

	tests := []struct {
		name, method, target string
		status               int
		code                 string
	}{
		{"unknown endpoint", http.MethodGet, "/api/nope", http.StatusNotFound, "not_found"},
		{"wrong method", http.MethodPost, "/api/health", http.StatusMethodNotAllowed, "method_not_allowed"},
		{"panic", http.MethodGet, "/api/boom", http.StatusInternalServerError, "internal"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(t, r, tt.method, tt.target)
			if rec.Code != tt.status {
				t.Errorf("status = %d, want %d", rec.Code, tt.status)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
			var body apitypes.ErrorBody
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("body %q: %v", rec.Body.String(), err)
			}
			if body.Error.Code != tt.code || body.Error.Message == "" {
				t.Errorf("error = %+v, want code %q and a message", body.Error, tt.code)
			}
		})
	}
}

func TestWebFallthrough(t *testing.T) {
	logger, _ := jsonLogger()
	web := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "web:"+r.URL.Path)
	})
	h := NewRouter(Deps{Logger: logger, Web: web})

	for _, target := range []string{"/", "/projects/3", "/assets/index-abc.js"} {
		rec := do(t, h, http.MethodGet, target)
		if got, want := rec.Body.String(), "web:"+target; got != want {
			t.Errorf("GET %s: body = %q, want %q", target, got, want)
		}
	}

	// /api stays the API's: unknown endpoints get the JSON error, not the SPA.
	rec := do(t, h, http.MethodGet, "/api/nope")
	if rec.Code != http.StatusNotFound || rec.Header().Get("Content-Type") != "application/json" {
		t.Errorf("GET /api/nope: status %d, Content-Type %q; want a JSON 404", rec.Code, rec.Header().Get("Content-Type"))
	}
	if rec := do(t, h, http.MethodGet, "/api/health"); rec.Code != http.StatusOK {
		t.Errorf("GET /api/health: status = %d, want 200", rec.Code)
	}
}
