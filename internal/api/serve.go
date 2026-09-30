package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// shutdownTimeout bounds how long in-flight requests get to finish.
const shutdownTimeout = 10 * time.Second

// Serve serves h on ln until ctx is cancelled, then shuts down gracefully:
// it stops accepting connections and waits for in-flight requests to finish.
// It returns nil after a clean shutdown. Each onShutdown func runs when
// shutdown starts, to end long-lived requests such as SSE streams.
func Serve(ctx context.Context, ln net.Listener, h http.Handler, logger *slog.Logger, onShutdown ...func()) error {
	srv := &http.Server{
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelWarn),
	}
	for _, f := range onShutdown {
		srv.RegisterOnShutdown(f)
	}

	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	logger.Info("listening", "addr", ln.Addr().String())

	select {
	case err := <-errc:
		return fmt.Errorf("serve: %w", err)
	case <-ctx.Done():
	}

	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	if err := <-errc; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}
