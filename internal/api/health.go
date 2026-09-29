package api

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"time"
)

type healthResponse struct {
	Status string `json:"status"` // "ok" or "unavailable"
}

// handleHealth reports whether the server can reach its database. A 503 uses
// the status body, not the error shape: it's a status report, not a failed
// request, and the UI reads the same field either way.
func handleHealth(sqlDB *sql.DB, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := sqlDB.PingContext(ctx); err != nil {
			logger.LogAttrs(r.Context(), slog.LevelWarn, "health: db ping failed", slog.Any("err", err))
			writeJSON(w, http.StatusServiceUnavailable, healthResponse{Status: "unavailable"})
			return
		}
		writeJSON(w, http.StatusOK, healthResponse{Status: "ok"})
	}
}
