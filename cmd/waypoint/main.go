// Command waypoint runs the Waypoint server: an AI-diary-driven personal project planner.
package main

import (
	"log/slog"
	"os"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	logger.Info("waypoint starting; nothing is wired up yet (PLAN 0.2+)")
}
