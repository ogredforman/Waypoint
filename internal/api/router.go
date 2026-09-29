package api

import (
	"database/sql"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// Deps are the dependencies the HTTP handlers need.
type Deps struct {
	DB     *sql.DB
	Logger *slog.Logger
	// Web serves everything outside /api (the embedded frontend). Nil means
	// no frontend: those paths get chi's default 404.
	Web http.Handler
}

// NewRouter returns the HTTP handler for the API and, if set, the frontend.
func NewRouter(d Deps) http.Handler {
	return newMux(d)
}

// newMux builds the router with its middleware stack. Tests use it directly
// to mount extra routes behind the same middleware.
func newMux(d Deps) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(logRequests(d.Logger))
	r.Use(recoverPanics(d.Logger))

	r.Route("/api", func(r chi.Router) {
		r.NotFound(func(w http.ResponseWriter, r *http.Request) {
			writeError(w, http.StatusNotFound, "not_found", "no such endpoint")
		})
		r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		})
		r.Get("/health", handleHealth(d.DB, d.Logger))
	})
	if d.Web != nil {
		r.Handle("/*", d.Web)
	}
	return r
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
					slog.Any("panic", rec),
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
