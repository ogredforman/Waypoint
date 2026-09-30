package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ogredforman/waypoint/internal/api/apitypes"
	"github.com/ogredforman/waypoint/internal/pipeline"
)

// count returns the number of connected subscribers.
func (h *Hub) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs)
}

// frame is one SSE message: a comment line, or an event with its data.
type frame struct {
	comment string
	event   string
	data    string
}

// stream is an open /api/events response read frame by frame.
type stream struct {
	resp   *http.Response
	frames chan frame
	errc   chan error
	cancel context.CancelFunc
}

// openStream connects to /api/events on srv and reads frames in the
// background until the stream ends or the test finishes.
func openStream(t *testing.T, srv *httptest.Server) *stream {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/events", nil)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	s := &stream{resp: resp, frames: make(chan frame, 64), errc: make(chan error, 1), cancel: cancel}
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.errc <- readFrames(resp.Body, s.frames)
	}()
	t.Cleanup(func() {
		cancel()
		_ = resp.Body.Close()
		<-done
	})
	return s
}

// readFrames parses SSE frames from r into out. It returns io.EOF when the
// stream ends cleanly.
func readFrames(r io.Reader, out chan<- frame) error {
	sc := bufio.NewScanner(r)
	var f frame
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
			out <- f
			f = frame{}
		case strings.HasPrefix(line, ": "):
			f.comment = strings.TrimPrefix(line, ": ")
		case strings.HasPrefix(line, "event: "):
			f.event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			f.data = strings.TrimPrefix(line, "data: ")
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return io.EOF
}

// next returns the next frame, failing the test if none arrives in time.
func (s *stream) next(t *testing.T) frame {
	t.Helper()
	select {
	case f := <-s.frames:
		return f
	case err := <-s.errc:
		t.Fatalf("stream ended: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("no frame within 5s")
	}
	return frame{}
}

// waitEnd waits for the stream to end, failing the test if it stays open.
func (s *stream) waitEnd(t *testing.T) {
	t.Helper()
	for {
		select {
		case <-s.frames:
		case err := <-s.errc:
			if !errors.Is(err, io.EOF) {
				t.Errorf("stream ended with %v, want EOF", err)
			}
			return
		case <-time.After(5 * time.Second):
			t.Fatal("stream still open after 5s")
		}
	}
}

// newEventsServer serves a router with a hub on a test HTTP server.
func newEventsServer(t *testing.T) (*Hub, *httptest.Server) {
	t.Helper()
	logger, _ := jsonLogger()
	hub := NewHub(logger)
	srv := httptest.NewServer(NewRouter(Deps{DB: openDB(t), Logger: logger, Hub: hub}))
	t.Cleanup(srv.Close)
	t.Cleanup(hub.Close) // runs before srv.Close, so open streams end first
	return hub, srv
}

// waitCount waits until hub has want subscribers.
func waitCount(t *testing.T, hub *Hub, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for hub.count() != want {
		if time.Now().After(deadline) {
			t.Fatalf("subscribers = %d, want %d", hub.count(), want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestEventsHeaders(t *testing.T) {
	_, srv := newEventsServer(t)
	s := openStream(t, srv)
	if s.resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", s.resp.StatusCode)
	}
	if got := s.resp.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", got)
	}
	if got := s.resp.Header.Get("Cache-Control"); got != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", got)
	}
	if f := s.next(t); f.comment != "connected" {
		t.Errorf("first frame = %+v, want the connected comment", f)
	}
}

func TestEventsNotifyFansOutInOrder(t *testing.T) {
	hub, srv := newEventsServer(t)
	streams := []*stream{openStream(t, srv), openStream(t, srv)}
	for _, s := range streams {
		s.next(t) // connected: the subscriber is registered
	}

	hub.Notify(pipeline.Event{Type: pipeline.EventEntryProcessed, EntryID: 1, RunID: 2})
	hub.Notify(pipeline.Event{Type: pipeline.EventChangesetReady, EntryID: 1, RunID: 2, ChangesetID: 3})
	hub.Notify(pipeline.Event{Type: pipeline.EventRunFailed, EntryID: 4, RunID: 5})
	hub.BriefingUpdated(6)

	want := []frame{
		{event: "entry.processed", data: `{"entry_id":1,"run_id":2}`},
		{event: "changeset.ready", data: `{"entry_id":1,"run_id":2,"changeset_id":3}`},
		{event: "run.failed", data: `{"entry_id":4,"run_id":5}`},
		{event: "briefing.updated", data: `{"project_id":6}`},
	}
	for i, s := range streams {
		for _, w := range want {
			if got := s.next(t); got != w {
				t.Errorf("stream %d: frame = %+v, want %+v", i, got, w)
			}
		}
	}
}

func TestEventsNotifyUnknownTypeDropped(t *testing.T) {
	hub, srv := newEventsServer(t)
	s := openStream(t, srv)
	s.next(t)

	hub.Notify(pipeline.Event{Type: "made.up", EntryID: 1})
	hub.Notify(pipeline.Event{Type: pipeline.EventRunFailed, EntryID: 1, RunID: 1})
	if f := s.next(t); f.event != "run.failed" {
		t.Errorf("frame = %+v, want run.failed (unknown type dropped)", f)
	}
}

func TestEventsPublishDropsBadEvents(t *testing.T) {
	hub, srv := newEventsServer(t)
	s := openStream(t, srv)
	s.next(t)

	hub.Publish("entry.processed", map[string]any{"x": make(chan int)}) // unencodable
	hub.Publish("bad\nname", map[string]int{"entry_id": 1})
	hub.Publish("run.failed", map[string]int{"entry_id": 1})
	if f := s.next(t); f.event != "run.failed" {
		t.Errorf("frame = %+v, want run.failed (bad events dropped)", f)
	}
	if n := hub.count(); n != 1 {
		t.Errorf("subscribers = %d, want 1", n)
	}
}

func TestEventsDisconnectUnsubscribes(t *testing.T) {
	hub, srv := newEventsServer(t)
	s := openStream(t, srv)
	s.next(t)
	waitCount(t, hub, 1)

	s.cancel()
	waitCount(t, hub, 0)
}

func TestEventsSlowSubscriberDropped(t *testing.T) {
	logger, _ := jsonLogger()
	hub := NewHub(logger)
	slow, ok := hub.subscribe()
	if !ok {
		t.Fatal("subscribe refused")
	}

	published := make(chan struct{})
	go func() {
		defer close(published)
		for range subscriberBuffer + 1 {
			hub.Publish("entry.processed", map[string]int{"entry_id": 1})
		}
	}()
	select {
	case <-published:
	case <-time.After(5 * time.Second):
		t.Fatal("Publish blocked on a slow subscriber")
	}

	select {
	case <-slow.done:
	default:
		t.Error("slow subscriber not dropped")
	}
	if n := hub.count(); n != 0 {
		t.Errorf("subscribers = %d, want 0", n)
	}
}

func TestEventsKeepalive(t *testing.T) {
	hub, srv := newEventsServer(t)
	hub.keepalive = 20 * time.Millisecond
	s := openStream(t, srv)
	s.next(t)
	if f := s.next(t); f.comment != "ping" {
		t.Errorf("frame = %+v, want the ping comment", f)
	}
}

func TestEventsCloseEndsStreams(t *testing.T) {
	hub, srv := newEventsServer(t)
	s := openStream(t, srv)
	s.next(t)

	hub.Close()
	s.waitEnd(t)

	resp, err := srv.Client().Get(srv.URL + "/api/events")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status after Close = %d, want 503", resp.StatusCode)
	}
	var body apitypes.ErrorBody
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != "unavailable" {
		t.Errorf("error code = %q, want unavailable", body.Error.Code)
	}
}

func TestEventsRouteNeedsHub(t *testing.T) {
	logger, _ := jsonLogger()
	h := NewRouter(Deps{DB: openDB(t), Logger: logger})
	if rec := do(t, h, http.MethodGet, "/api/events"); rec.Code != http.StatusNotFound {
		t.Errorf("status without a hub = %d, want 404", rec.Code)
	}
}
