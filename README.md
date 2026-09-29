# Waypoint

A local-first, AI-assisted diary that turns what you write about your projects into a plan you can
actually follow: tasks, blockers, parked ideas, and one clear next action.

**Status:** pre-prototype. Nothing runs yet.

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

Requirements for the hooks in `.claude/settings.json`: Go 1.22+ on your PATH, and `git`.

Build, run, and configuration instructions arrive with Stage 0 and Stage 2.
