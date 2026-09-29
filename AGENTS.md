# Waypoint

AI-diary-driven personal project planner. The user writes free-form diary entries; the system
extracts tasks, blockers, and ideas as a *proposed changeset* the user reviews, and keeps a
"what's next" briefing current.

Go single binary + embedded Svelte SPA + SQLite (pure Go). Local-first, single user, no auth.

- Product and architecture: `docs/SPEC.md` (source of truth for *what* and *why*)
- Build order and task list: `docs/PLAN.md` (source of truth for *what to do now*)
- Decisions: `docs/adr/`

## Before any task

- Read the task in `docs/PLAN.md` and only the SPEC sections it touches. Don't read the whole SPEC by default.
- The current stage is the one named at the top of `docs/PLAN.md`. Do not implement anything
  tagged for a later stage, even if it looks easy. Add it to the "Parked" section of PLAN.md instead.
- If a task is ambiguous or conflicts with the SPEC, stop and ask. Don't guess on schema or API shape.
- For anything touching more than 2 files, propose a plan and wait for approval before writing code.

## Commands

These are created in Stage 0 task 0.1. Until then, they don't exist.

- `make dev`: Go server + Vite dev server with `/api` proxy
- `make test`: all Go and frontend tests. Must pass before a task is finished.
- `make lint`: `go vet`, `staticcheck`, `svelte-check`
- `make sqlc`: regenerate query code after editing `internal/db/queries/`
- `make build`: production binary with the frontend embedded

## Hard rules

- Every mutation of projects, tasks, blockers, or ideas goes through `internal/changeset`.
  Handlers and services never write those tables directly. Manual edits are `source=manual` changesets.
- Entries are immutable. There is no UPDATE on `entries.body`, ever.
- Nothing outside `internal/extract/*` knows which LLM provider is in use.
- Every extractor's output passes through the shared validator before it touches the DB.
- Schema changes are new goose migrations. Never edit a migration that is already committed.
- No CGO. SQLite driver is `modernc.org/sqlite`.
- Never log entry bodies, observation text, or API keys at info level or above.
- The server binds to `127.0.0.1` by default.
- Product principles in SPEC §5 are requirements, not style: one next action, no guilt mechanics
  (no overdue/streak/red-failure UI), at most 2 follow-up questions per entry, new ideas get parked.
- New dependencies need a one-line reason in the commit message. Significant ones get an ADR.

## Style

- Standard-library-first, boring Go. Small packages with clear boundaries (SPEC §10.2).
- Prefer explicit code over clever abstractions. No generic repository frameworks.
- Tests use a real temp SQLite database, not mocks of the store.

## Finishing a task

1. Tests added or updated; `make test` and `make lint` pass.
2. Run the `reviewer` agent on the diff and fix any Blocking findings.
3. Tick the task `[x]` in `docs/PLAN.md`.
4. If a design decision changed, update `docs/SPEC.md` or add `docs/adr/NNNN-title.md` in the same commit.
5. One task per commit. Message format: `<area>: <what changed> (PLAN <stage>.<task>)`.
