package web

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

const indexHTML = "<!doctype html><title>Waypoint</title>"

func builtFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html":           {Data: []byte(indexHTML)},
		"assets/index-abc.js":  {Data: []byte("console.log(1)")},
		"assets/index-abc.css": {Data: []byte("body{}")},
		"favicon.svg":          {Data: []byte("<svg/>")},
		".gitkeep":             {Data: nil},
	}
}

func do(h http.Handler, method, target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return rec
}

func TestHandler(t *testing.T) {
	h := NewHandler(builtFS())

	tests := []struct {
		name, method, target string
		status               int
		body                 string // substring; "" skips the check
		contentType          string // prefix; "" skips the check
		cacheControl         string // exact; "-" means the header must be absent
	}{
		{"root", "GET", "/", 200, indexHTML, "text/html", "no-cache"},
		{"index by name", "GET", "/index.html", 200, indexHTML, "text/html", "no-cache"},
		{"spa route", "GET", "/projects/3", 200, indexHTML, "text/html", "no-cache"},
		{"spa route with query", "GET", "/tasks/7?tab=history", 200, indexHTML, "text/html", "no-cache"},
		{"head spa route", "HEAD", "/diary", 200, "", "text/html", "no-cache"},
		{"hashed js", "GET", "/assets/index-abc.js", 200, "console.log", "text/javascript", "public, max-age=31536000, immutable"},
		{"hashed css", "GET", "/assets/index-abc.css", 200, "body{}", "text/css", "public, max-age=31536000, immutable"},
		{"unhashed file", "GET", "/favicon.svg", 200, "<svg/>", "image/svg+xml", "-"},
		{"missing asset", "GET", "/assets/index-old.js", 404, "", "", "-"},
		{"missing file at root", "GET", "/robots.txt", 404, "", "", "-"},
		{"directory", "GET", "/assets", 200, indexHTML, "text/html", "no-cache"},
		{"post", "POST", "/", 405, "", "", "-"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(h, tt.method, tt.target)
			if rec.Code != tt.status {
				t.Fatalf("status = %d, want %d (body %q)", rec.Code, tt.status, rec.Body.String())
			}
			if tt.body != "" && !strings.Contains(rec.Body.String(), tt.body) {
				t.Errorf("body = %q, want it to contain %q", rec.Body.String(), tt.body)
			}
			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, tt.contentType) {
				t.Errorf("Content-Type = %q, want prefix %q", ct, tt.contentType)
			}
			cc, ok := rec.Header()["Cache-Control"]
			switch {
			case tt.cacheControl == "-" && ok:
				t.Errorf("Cache-Control = %q, want none", cc)
			case tt.cacheControl != "-" && rec.Header().Get("Cache-Control") != tt.cacheControl:
				t.Errorf("Cache-Control = %q, want %q", rec.Header().Get("Cache-Control"), tt.cacheControl)
			}
			if tt.method == "HEAD" && rec.Body.Len() != 0 {
				t.Errorf("HEAD response has a body: %q", rec.Body.String())
			}
		})
	}

	if allow := do(h, "POST", "/").Header().Get("Allow"); allow != "GET, HEAD" {
		t.Errorf("Allow = %q, want GET, HEAD", allow)
	}
}

func TestHandlerNotBuilt(t *testing.T) {
	// A fresh clone embeds only dist/.gitkeep.
	h := NewHandler(fstest.MapFS{".gitkeep": {Data: nil}})

	for _, target := range []string{"/", "/projects/3"} {
		rec := do(h, "GET", target)
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("GET %s: status = %d, want 503", target, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "make web") {
			t.Errorf("GET %s: body = %q, want a hint to run make web", target, rec.Body.String())
		}
	}
}

func TestDist(t *testing.T) {
	// The embedded tree is rooted at dist/, whether or not the frontend is built.
	if _, err := fs.Stat(Dist(), ".gitkeep"); err != nil {
		t.Fatalf("Dist() is not rooted at dist/: %v", err)
	}
}
