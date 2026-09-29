// Command waypoint runs the Waypoint server: an AI-diary-driven personal project planner.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"runtime"

	"github.com/ogredforman/waypoint/internal/config"
	"github.com/ogredforman/waypoint/internal/db"
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
	logger.Info("nothing else is wired up yet (PLAN 0.4+)")
	return 0
}
