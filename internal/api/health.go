package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/ogredforman/waypoint/internal/api/apitypes"
)

// handleHealth reports whether the server can reach every profile's
// database. A 503 uses the status body, not the error shape: it's a status
// report, not a failed request, and the UI reads the same field either way.
// With no profiles to check, the server is healthy.
func handleHealth(ps Profiles, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if ps != nil {
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()
			if err := ps.Ping(ctx); err != nil {
				if r.Context().Err() != nil {
					return // the client went away; nothing to report
				}
				logger.LogAttrs(r.Context(), slog.LevelWarn, "health: db ping failed", slog.Any("err", err))
				writeJSON(w, http.StatusServiceUnavailable, apitypes.Health{Status: "unavailable"})
				return
			}
		}
		writeJSON(w, http.StatusOK, apitypes.Health{Status: "ok"})
	}
}
