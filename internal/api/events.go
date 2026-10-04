package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ogredforman/waypoint/internal/api/apitypes"
	"github.com/ogredforman/waypoint/internal/changeset"
	"github.com/ogredforman/waypoint/internal/pipeline"
)

const (
	// subscriberBuffer is how many events a client may fall behind before
	// the hub drops it. EventSource reconnects on its own, and the UI
	// refetches on reconnect, so dropping loses nothing for good.
	subscriberBuffer = 32
	// defaultKeepalive is how often an idle stream gets a comment line, so
	// proxies keep it open and a dead client is noticed.
	defaultKeepalive = 25 * time.Second
)

// SSE event names for events that don't come from the worker.
const (
	// eventBriefingUpdated: a project's briefing was regenerated.
	eventBriefingUpdated = "briefing.updated"
	// eventChangesetApplied: a changeset was applied.
	eventChangesetApplied = "changeset.applied"
	// eventEntryDeleted: an entry was deleted.
	eventEntryDeleted = "entry.deleted"
)

// Hub fans server events out to every connected /api/events stream (SPEC
// §10.5). Publishing never blocks: a client whose buffer is full is
// disconnected instead. Events carry IDs only, never entry text.
type Hub struct {
	logger    *slog.Logger
	keepalive time.Duration

	mu     sync.Mutex
	subs   map[*subscriber]struct{}
	closed bool
}

type subscriber struct {
	msgs chan []byte
	// done is closed when the hub drops the subscriber or shuts down.
	done chan struct{}
}

// NewHub returns a hub with no subscribers.
func NewHub(logger *slog.Logger) *Hub {
	return &Hub{
		logger:    logger,
		keepalive: defaultKeepalive,
		subs:      make(map[*subscriber]struct{}),
	}
}

// Publish sends an event named name, with payload encoded as JSON, to every
// connected client. name must be one of the fixed event names; a name with a
// line break would split the frame, so it is refused.
func (h *Hub) Publish(name string, payload any) {
	if strings.ContainsAny(name, "\r\n") {
		h.logger.Error("sse: invalid event name")
		return
	}
	data, err := json.Marshal(payload)
	if err != nil {
		h.logger.Error("sse: encode event", "event", name, "err", err)
		return
	}
	msg := []byte(fmt.Sprintf("event: %s\ndata: %s\n\n", name, data))

	h.mu.Lock()
	defer h.mu.Unlock()
	for s := range h.subs {
		select {
		case s.msgs <- msg:
		default:
			h.logger.Warn("sse: client too slow, disconnecting", "event", name)
			h.removeLocked(s)
		}
	}
}

// Notify publishes a worker event. It makes the hub a pipeline.Notifier.
func (h *Hub) Notify(e pipeline.Event) {
	switch e.Type {
	case pipeline.EventEntryProcessed:
		h.Publish(string(e.Type), apitypes.EntryProcessedEvent{EntryID: e.EntryID, RunID: e.RunID})
	case pipeline.EventChangesetReady:
		h.Publish(string(e.Type), apitypes.ChangesetReadyEvent{EntryID: e.EntryID, RunID: e.RunID, ChangesetID: e.ChangesetID})
	case pipeline.EventRunFailed:
		h.Publish(string(e.Type), apitypes.RunFailedEvent{EntryID: e.EntryID, RunID: e.RunID})
	default:
		h.logger.Warn("sse: unknown worker event", "event", string(e.Type))
	}
}

// BriefingUpdated publishes a "briefing.updated" event. It makes the hub a
// briefing.Notifier.
func (h *Hub) BriefingUpdated(projectID int64) {
	h.Publish(eventBriefingUpdated, apitypes.BriefingUpdatedEvent{ProjectID: projectID})
}

// Applied publishes a "changeset.applied" event. It makes the hub a
// changeset.AfterApply.
func (h *Hub) Applied(_ context.Context, res changeset.Result) {
	ids := res.Projects
	if ids == nil {
		ids = []int64{}
	}
	h.Publish(eventChangesetApplied, apitypes.ChangesetAppliedEvent{ChangesetID: res.Changeset.ID, ProjectIDs: ids})
}

// EntryDeleted publishes an "entry.deleted" event.
func (h *Hub) EntryDeleted(entryID int64) {
	h.Publish(eventEntryDeleted, apitypes.EntryDeletedEvent{EntryID: entryID})
}

// Close ends every open stream and refuses new ones. The server calls it
// when shutdown starts; otherwise Shutdown would wait on streams that never
// finish.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	for s := range h.subs {
		h.removeLocked(s)
	}
}

// ServeHTTP streams events to one client until it disconnects or the hub
// closes.
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s, ok := h.subscribe()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "server is shutting down")
		return
	}
	defer h.unsubscribe(s)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	rc := http.NewResponseController(w)
	// The first comment flushes the headers, so the client's open event
	// fires before any real event arrives.
	if !h.write(w, rc, []byte(": connected\n\n")) {
		return
	}

	ticker := time.NewTicker(h.keepalive)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.done:
			return
		case msg := <-s.msgs:
			if !h.write(w, rc, msg) {
				return
			}
		case <-ticker.C:
			if !h.write(w, rc, []byte(": ping\n\n")) {
				return
			}
		}
	}
}

// write sends msg and flushes it. It reports false once the stream is
// unusable.
func (h *Hub) write(w http.ResponseWriter, rc *http.ResponseController, msg []byte) bool {
	if _, err := w.Write(msg); err != nil {
		return false
	}
	if err := rc.Flush(); err != nil {
		h.logger.Debug("sse: flush failed", "err", err)
		return false
	}
	return true
}

func (h *Hub) subscribe() (*subscriber, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return nil, false
	}
	s := &subscriber{msgs: make(chan []byte, subscriberBuffer), done: make(chan struct{})}
	h.subs[s] = struct{}{}
	return s, true
}

func (h *Hub) unsubscribe(s *subscriber) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.removeLocked(s)
}

// removeLocked drops s if it is still subscribed. h.mu must be held.
func (h *Hub) removeLocked(s *subscriber) {
	if _, ok := h.subs[s]; !ok {
		return
	}
	delete(h.subs, s)
	close(s.done)
}
