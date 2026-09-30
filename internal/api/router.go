package api

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"runtime"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/ogredforman/waypoint/internal/api/apitypes"
	"github.com/ogredforman/waypoint/internal/briefing"
	"github.com/ogredforman/waypoint/internal/changeset"
	"github.com/ogredforman/waypoint/internal/pipeline"
	"github.com/ogredforman/waypoint/internal/store"
)

// Deps are the dependencies the HTTP handlers need.
type Deps struct {
	DB     *sql.DB
	Logger *slog.Logger
	// Web serves everything outside /api (the embedded frontend). Nil means
	// no frontend: those paths get chi's default 404.
	Web http.Handler
	// Hub serves /api/events. Nil leaves the route unmounted.
	Hub *Hub

	// Store, Applier, Worker, and Briefings back the data routes (SPEC
	// §10.5). A nil Store leaves those routes unmounted; otherwise all four
	// must be set.
	Store     *store.Store
	Applier   *changeset.Applier
	Worker    *pipeline.Worker
	Briefings *briefing.Service
	// Settings is what GET /api/settings reports.
	Settings apitypes.Settings
}

// handlers serve the data routes. Every write to projects, tasks, blockers,
// and ideas goes through the applier as a manual changeset.
type handlers struct {
	store     *store.Store
	applier   *changeset.Applier
	worker    *pipeline.Worker
	briefings *briefing.Service
	settings  apitypes.Settings
	logger    *slog.Logger
}

// NewRouter returns the HTTP handler for the API and, if set, the frontend.
func NewRouter(d Deps) http.Handler {
	return newMux(d)
}

// newMux builds the router with its middleware stack. Tests use it directly
// to mount extra routes behind the same middleware.
func newMux(d Deps) *chi.Mux {
	r := chi.NewRouter()
	root := r
	r.Use(middleware.RequestID)
	r.Use(logRequests(d.Logger))
	r.Use(recoverPanics(d.Logger))

	r.Route("/api", func(r chi.Router) {
		r.NotFound(func(w http.ResponseWriter, r *http.Request) {
			writeError(w, http.StatusNotFound, "not_found", "no such endpoint")
		})
		r.MethodNotAllowed(func(w http.ResponseWriter, req *http.Request) {
			// chi only sets Allow in its own 405 handler, so work it out
			// again from the root router.
			for _, m := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
				if root.Match(chi.NewRouteContext(), m, req.URL.Path) {
					w.Header().Add("Allow", m)
				}
			}
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		})
		r.Get("/health", handleHealth(d.DB, d.Logger))
		if d.Hub != nil {
			r.Get("/events", d.Hub.ServeHTTP)
		}
		if d.Store != nil {
			h := &handlers{
				store:     d.Store,
				applier:   d.Applier,
				worker:    d.Worker,
				briefings: d.Briefings,
				settings:  d.Settings,
				logger:    d.Logger,
			}
			h.routes(r)
		}
	})
	if d.Web != nil {
		r.Handle("/*", d.Web)
	}
	return r
}

// routes mounts the data routes from SPEC §10.5 on the /api router.
func (h *handlers) routes(r chi.Router) {
	r.Get("/projects", h.listProjects)
	r.Post("/projects", h.createProject)
	r.Get("/projects/{id}", h.getProject)
	r.Patch("/projects/{id}", h.updateProject)
	r.Get("/projects/{id}/tasks", h.listTasks)
	r.Post("/projects/{id}/tasks", h.createTask)
	r.Get("/projects/{id}/blockers", h.listBlockers)
	r.Get("/projects/{id}/briefing", h.getBriefing)
	r.Post("/projects/{id}/briefing/refresh", h.refreshBriefing)

	r.Get("/tasks/{id}", h.getTask)
	r.Patch("/tasks/{id}", h.updateTask)
	r.Delete("/tasks/{id}", h.deleteTask)

	r.Patch("/blockers/{id}", h.updateBlocker)
	r.Delete("/blockers/{id}", h.deleteBlocker)

	r.Get("/ideas", h.listIdeas)
	r.Patch("/ideas/{id}", h.updateIdea)
	r.Delete("/ideas/{id}", h.deleteIdea)
	r.Post("/ideas/{id}/promote", h.promoteIdea)

	r.Get("/entries", h.listEntries)
	r.Post("/entries", h.createEntry)
	r.Get("/entries/{id}", h.getEntry)
	r.Post("/entries/{id}/reprocess", h.reprocessEntry)
	r.Post("/questions/{id}/dismiss", h.dismissQuestion)

	r.Get("/changesets", h.listChangesets)
	r.Post("/changesets/{id}/apply", h.applyChangeset)

	r.Get("/settings", h.getSettings)
}

// logRequests logs one line per request. It records only the method, path,
// status, size, duration, and request ID: never bodies or query strings,
// which may carry diary text.
func logRequests(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			defer func() {
				status := ww.Status()
				if status == 0 {
					status = http.StatusOK
				}
				logger.LogAttrs(r.Context(), slog.LevelInfo, "request",
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
					slog.Int("status", status),
					slog.Int("bytes", ww.BytesWritten()),
					slog.Duration("duration", time.Since(start)),
					slog.String("request_id", middleware.GetReqID(r.Context())),
				)
			}()
			next.ServeHTTP(ww, r)
		})
	}
}

// recoverPanics turns a handler panic into a 500 and logs it through slog.
// http.ErrAbortHandler is re-panicked so net/http can abort the response.
// The log records the panic value's type, and the message only for runtime
// errors (nil dereference, index out of range): other panic values may be
// built from request data, which can be diary text.
func recoverPanics(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}
				if rec == http.ErrAbortHandler {
					panic(rec)
				}
				logger.LogAttrs(r.Context(), slog.LevelError, "handler panic",
					slog.String("panic", panicSummary(rec)),
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
					slog.String("request_id", middleware.GetReqID(r.Context())),
				)
				// Best effort: if the handler already wrote headers, the client
				// gets a truncated response instead of this 500.
				writeError(w, http.StatusInternalServerError, "internal", "internal server error")
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// panicSummary describes a recovered panic value without its contents,
// unless it is a runtime error, whose message the runtime writes.
func panicSummary(rec any) string {
	if err, ok := rec.(runtime.Error); ok {
		return err.Error()
	}
	return fmt.Sprintf("%T", rec)
}
