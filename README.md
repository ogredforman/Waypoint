# Waypoint

A local-first, AI-assisted diary that turns what you write about your projects into a plan you can
actually follow: tasks, blockers, parked ideas, and one clear next action.

**Status:** Stage 0 (foundations). The binary builds and runs, but doesn't do anything yet.

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

Requirements: Go 1.26+ and GNU `make`. Node.js is needed once the frontend lands (PLAN 0.7).
`staticcheck` is pinned as a tool dependency in `go.mod`, so it needs no separate install.
How `sqlc` is provided gets decided in PLAN 0.5.

| Target | What it does |
|---|---|
| `make dev` | Run the server locally |
| `make build` | Build `bin/waypoint` (with the frontend embedded, once it exists) |
| `make test` | All Go and frontend tests |
| `make lint` | `go vet`, `staticcheck`, `svelte-check` |
| `make sqlc` | Regenerate query code after editing `internal/db/queries/` |

Configuration and provider setup instructions arrive with Stage 2.
