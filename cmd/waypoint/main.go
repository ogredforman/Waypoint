// Command waypoint runs the Waypoint server: an AI-diary-driven personal project planner.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/ogredforman/waypoint/internal/api"
	"github.com/ogredforman/waypoint/internal/config"
	"github.com/ogredforman/waypoint/internal/extract/mock"
	"github.com/ogredforman/waypoint/internal/profile"
	"github.com/ogredforman/waypoint/internal/web"
)

func main() {
	os.Exit(run())
}

func run() int {
	cfg, err := config.Load(os.Args[1:], os.LookupEnv, runtime.GOOS, os.Stderr)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "waypoint:", err)
		return 2
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.LogLevel}))

	if err := config.EnsureDataDir(cfg); err != nil {
		logger.Error("startup failed", "err", err)
		return 1
	}
	logger.Info("config loaded",
		"config_file", cfg.File,
		"data_dir", cfg.DataDir,
		"profiles_dir", cfg.ProfilesDir(),
		"addr", cfg.Addr(),
		"log_level", cfg.LogLevel.String(),
		"mock_latency", cfg.MockLatency.String(),
		"snapshot_recent_entries", cfg.SnapshotRecentEntries,
	)

	ctx := context.Background()
	// Every profile is its own database with its own store, worker, hub, and
	// briefings (ADR 0005). Config values only seed the settings of a
	// profile that has none.
	profiles, err := profile.Open(ctx, profile.Options{
		Dir:                   cfg.ProfilesDir(),
		LegacyDB:              cfg.LegacyDBPath(),
		Seed:                  cfg.SettingsDefaults(),
		Extractor:             mock.New(cfg.MockLatency),
		SnapshotRecentEntries: cfg.SnapshotRecentEntries,
		Logger:                logger,
	})
	if err != nil {
		logger.Error("startup failed", "err", err)
		return 1
	}
	defer profiles.Close()
	ids := make([]string, 0)
	for _, p := range profiles.List() {
		ids = append(ids, p.ID)
	}
	logger.Info("profiles ready", "profiles", ids)

	ln, err := net.Listen("tcp", cfg.Addr())
	if err != nil {
		logger.Error("startup failed", "err", err)
		return 1
	}
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		// Once shutdown starts, restore default signal handling so a second
		// Ctrl-C kills the process instead of waiting out a slow shutdown.
		<-ctx.Done()
		stop()
	}()

	router := api.NewRouter(api.Deps{
		Logger:   logger,
		Web:      web.NewHandler(web.Dist()),
		Profiles: profiles,
	})
	err = api.Serve(ctx, ln, router, logger, profiles.EndStreams)
	if err != nil {
		logger.Error("server stopped", "err", err)
		return 1
	}
	logger.Info("stopped")
	return 0
}
