# Waypoint

A local-first, AI-assisted diary that turns what you write about your projects into a plan you can
actually follow: tasks, blockers, parked ideas, and one clear next action.

**Status:** Stage 1 (prototype, mock AI). Stage 0 is done: the binary builds, runs, and passes CI on Linux, macOS, and Windows, but doesn't do anything yet.

- What it is and why: [`docs/SPEC.md`](docs/SPEC.md)
- How it's being built, and what's next: [`docs/PLAN.md`](docs/PLAN.md)
- Instructions for AI coding agents: [`AGENTS.md`](AGENTS.md)

## Development workflow (Claude Code)

| Command | What it does |
|---|---|
| `/next-task` | Picks the next unchecked task in `docs/PLAN.md`, plans it, waits for approval, builds, tests, reviews |
| `/next-task 1.5` | Same, for a specific task |
| `/park <idea>` | Saves an idea to PLAN.md's Parked section without acting on it |
| `/progress` | Where things stand and what's next |

Requirements for the hooks in `.claude/settings.json`: Go on your PATH, and `git`.

## Building

Requirements: Go 1.26+, GNU `make`, and Node.js 24+ (for the Svelte frontend in `web/`; run
`npm --prefix web install` once before `make dev`).
`staticcheck` is pinned as a tool dependency in `go.mod`, so it needs no separate install.
How `sqlc` is provided gets decided in PLAN 0.5.

| Target | What it does |
|---|---|
| `make dev` | Run the Go server and the Vite dev server together; open http://localhost:5173 (Vite proxies `/api` to Go) |
| `make build` | Build the frontend, then `bin/waypoint` with it embedded; the binary serves it at http://127.0.0.1:7420 |
| `make test` | All Go and frontend tests |
| `make lint` | `go vet`, `staticcheck`, `svelte-check` |
| `make sqlc` | Regenerate query code after editing `internal/db/queries/` |

## Configuration

Each setting comes from the first place that sets it: command-line flag, then `WAYPOINT_*` environment
variable, then `config.toml`, then the default. Run `waypoint -h` to list the flags.

| Flag | Env | `config.toml` | Default |
|---|---|---|---|
| `-config` | `WAYPOINT_CONFIG` | n/a | `config.toml` in the OS config folder (see below) |
| `-port` | `WAYPOINT_PORT` | `port` | `7420` |
| `-data-dir` | `WAYPOINT_DATA_DIR` | `data_dir` | OS data folder (see below) |
| `-log-level` | `WAYPOINT_LOG_LEVEL` | `log_level` | `info` |
| n/a | n/a | `active_project_cap` | `3` (how many projects can be active at once) |
| n/a | n/a | `mock_latency` | `0` (artificial delay per mock extraction, e.g. `"1500ms"`) |

| OS | Config folder | Data folder (holds `waypoint.db`) |
|---|---|---|
| Windows | `%APPDATA%\Waypoint` | `%LOCALAPPDATA%\Waypoint` |
| macOS | `~/Library/Application Support/Waypoint` | `~/Library/Application Support/Waypoint` |
| Linux | `$XDG_CONFIG_HOME/waypoint` or `~/.config/waypoint` | `$XDG_DATA_HOME/waypoint` or `~/.local/share/waypoint` |

The server only listens on `127.0.0.1`. A relative `data_dir` in `config.toml` is relative to that file.
Provider setup instructions arrive with Stage 2.
