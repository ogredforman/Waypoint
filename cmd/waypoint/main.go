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
	"github.com/ogredforman/waypoint/internal/db"
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
		"db", cfg.DBPath(),
		"addr", cfg.Addr(),
		"log_level", cfg.LogLevel.String(),
	)

	ctx := context.Background()
	sqlDB, err := db.Open(ctx, cfg.DBPath())
	if err != nil {
		logger.Error("startup failed", "err", err)
		return 1
	}
	defer func() { _ = sqlDB.Close() }()

	migrations, err := db.Migrations()
	if err != nil {
		logger.Error("startup failed", "err", err)
		return 1
	}
	version, applied, err := db.Migrate(ctx, sqlDB, migrations)
	if err != nil {
		logger.Error("startup failed", "err", err, "applied", applied)
		return 1
	}
	logger.Info("database ready", "schema_version", version, "applied", applied)

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
	router := api.NewRouter(api.Deps{DB: sqlDB, Logger: logger, Web: web.NewHandler(web.Dist())})
	if err := api.Serve(ctx, ln, router, logger); err != nil {
		logger.Error("server stopped", "err", err)
		return 1
	}
	logger.Info("stopped")
	return 0
}
