# Waypoint — Implementation Plan

> Companion to [`SPEC.md`](./SPEC.md). The spec defines *what* and *why*; this plan defines *order*, *scope per stage*, and *exit criteria*. Each stage should end in a runnable, usable state. Don't pull later-stage work forward.

**Current stage: Stage 0 — Foundations**

Task IDs are `<stage>.<task>` (e.g. `1.5`). Tick tasks `[x]` as they're finished. `/next-task` picks the first unchecked task in the current stage; `/park` adds ideas to the Parked section at the bottom.

## Background (conversation summary)

- **Idea:** a personal planning app where an AI-assisted diary is the main input. Free-form entries about progress, thoughts, feelings, and blockers are turned into structured project state (tasks, blockers, ideas), with every thought linked to the part of the project it concerns.
- **Motivation:** the owner repeatedly sets ambitious goals and struggles to follow through (focus, starting, context loss; ADHD). Detailed "what's next" writeups have helped. Waypoint automates keeping that writeup current.
- **Key design decisions:**
  - Entries are immutable. AI output is a *proposed changeset* the user reviews.
  - Tasks are concrete actions, not project components.
  - The home screen shows one next action and a "where you left off" recap.
  - No guilt mechanics. Ideas get parked. Active projects are capped.
- **Stack:** Go single binary + embedded Svelte SPA + SQLite (pure Go driver) + SSE. Pluggable extractor: mock, Anthropic, or OpenAI-compatible (covers local models).
- **Scope now:** local-first, single user, no auth. Auth, multi-user, and sync may come later.
- **Guardrail:** building the productivity app must not become the new lofty goal. Stages are small, and the MVP's test is simply *"I use it to track building it."*

---

## Stage 0 — Foundations (scaffolding)

**Goal:** an empty but correctly wired app that builds and runs on all three OSes.

Tasks:
- [x] **0.1** Initialize the Go module, repo layout (SPEC §10.2), and a `Makefile` with `dev`, `build`, `test`, `lint`, `sqlc` targets (the commands `AGENTS.md` refers to).
- [x] **0.2** Config loading (flags > env > `config.toml`); OS-appropriate data dir for the DB.
- [x] **0.3** SQLite via `modernc.org/sqlite` with WAL mode; goose migrations embedded and run at startup.
- [x] **0.4** Initial migration with the full core schema from SPEC §6.1 (milestones table included but unused).
- [x] **0.5** sqlc setup and first generated queries.
- [ ] **0.6** chi server with `/api/health`, structured logging (`log/slog`), graceful shutdown, bound to `127.0.0.1:<port>`.
- [ ] **0.7** Svelte + TS + Vite + Tailwind app in `/web`; production build embedded via `embed.FS`; SPA fallback routing; dev proxy for `/api`.
- [ ] **0.8** Decide API type-sharing approach and record it in `/docs/adr/0001-api-types.md`.
- [ ] **0.9** CI (GitHub Actions or similar): test and cross-compile for linux/darwin/windows.

**Exit criteria:** `make build` produces a binary on each OS that opens a page in the browser showing "Waypoint" and a healthy API status. The DB file is created and migrated.

---

## Stage 1 — Prototype (mock AI, no real model)

**Goal:** the complete core loop, *write → extract → review → apply → briefing*, working end to end with the deterministic **mock extractor**. This validates the data model, pipeline, changeset system, and UI before any AI cost or unpredictability enters the picture.

### 1A. Domain and state
- [ ] **1.1** Domain types, enums, and validation in `internal/domain`.
- [ ] **1.2** Store layer: CRUD for projects, tasks, blockers, ideas, entries (insert-only), observations, and links.
- [ ] **1.3** Project status rules, including the active-project cap.

### 1B. Changeset system
- [ ] **1.4** Change op types and payload validation (SPEC §6.2).
- [ ] **1.5** Transactional applier: per-op accept, reject, or edit; temp-ID (`$new:N`) resolution; full rollback on any failure; changeset status updates.
- [ ] **1.6** Manual edits routed through the applier as `source=manual` changesets.
- [ ] **1.7** Unit tests covering every op type, temp IDs, partial apply, and rollback.

### 1C. Pipeline
- [ ] **1.8** `Extractor` interface, `ExtractionResult` types, and the shared output validator.
- [ ] **1.9** Mock extractor per SPEC §10.4, with configurable artificial latency.
- [ ] **1.10** Snapshot builder.
- [ ] **1.11** Background worker: entry saved → run created (`pending`) → extract → validate → store observations, changeset, and questions → emit SSE. Pending runs resume on restart; failures are recorded; `reprocess` endpoint.
- [ ] **1.12** SSE hub and `/api/events`.

### 1D. Briefing
- [ ] **1.13** Heuristic `Briefer` (SPEC §7.3). Recompute on changeset apply and task edit.

### 1E. API
- [ ] **1.14** Endpoints from SPEC §10.5, except settings UI editing and export (config file only for now).
- [ ] **1.15** API integration tests using the mock extractor end to end.

### 1F. UI
- [ ] **1.16** App shell and navigation: Now, Diary, Projects, Parking Lot.
- [ ] **1.17** **Now** screen: briefing cards and diary input (Cmd/Ctrl+Enter).
- [ ] **1.18** **Diary** timeline: entries with observation highlights; inline pending changeset review (accept, reject, edit per op, accept-all); follow-up questions with answer and dismiss.
- [ ] **1.19** **Project** screen: header, task list with inline edit, status changes and reordering, blockers, ideas.
- [ ] **1.20** **Task detail:** origin entry and linked observations timeline.
- [ ] **1.21** **Parking lot:** ideas and parked projects; promote to task or project.
- [ ] **1.22** Live updates via SSE; a "processing…" state on new entries.
- [ ] **1.23** Playwright smoke test: create project → write entry with `todo:`/`blocked:` lines → apply → task and blocker appear → briefing updates.

**Exit criteria:**
- Using only the mock syntax, the owner can create a project, write entries, review and apply changes, and see tasks, blockers, ideas, and the Now briefing update live.
- Every task shows the entry it came from.
- Killing the app mid-processing and restarting resumes the pending run.
- All tests pass in CI.

---

## Stage 2 — MVP (real AI, dogfoodable)

**Goal:** swap in a real model so natural-language entries work, with just enough polish that the owner **uses Waypoint to track the rest of Waypoint's development.**

### 2A. Extraction with a real model
- [ ] **2.1** Prompt design: a system prompt stating the principles (tasks are startable actions; propose, don't impose; at most 2 questions; park new ideas); snapshot formatting; version the prompt (`prompt_version`).
- [ ] **2.2** JSON schema for `ExtractionResult`, used via tool/function calling.
- [ ] **2.3** `AnthropicExtractor`.
- [ ] **2.4** `OpenAICompatExtractor` (OpenAI, Ollama, LM Studio, llama.cpp). Test with at least one local model.
- [ ] **2.5** Robustness: timeouts, one retry with the validation errors fed back to the model, and a clean failed-run state with a retry button in the UI.
- [ ] **2.6** Store the raw output and latency for every run.

### 2B. Evals
- [ ] **2.7** `/testdata/entries`: 15–30 realistic sample entries with expected observations and ops, several written from real dogfooding.
- [ ] **2.8** `waypoint eval --provider X`: runs the fixtures, scores op and link accuracy, and prints a report. Run it whenever the prompt changes.

### 2C. LLM features
- [ ] **2.9** LLM `Briefer`: next action with a reason, plus a recap. Validated against existing task IDs, with heuristic fallback.
- [ ] **2.10** `split_task` suggestions when a task is vague or has stalled.
- [ ] **2.11** Better follow-up questions (stalled tasks, missing next step, negative tone).

### 2D. Usability minimums
- [ ] **2.12** Settings screen: provider, model, endpoint, and key (key stored in config, masked in UI and excluded from export).
- [ ] **2.13** JSON export (`/api/export`) and a documented "back up by copying the DB file" note.
- [ ] **2.14** Error and empty states; a first-run flow that creates the first project.
- [ ] **2.15** README: install, configure a provider, run.

### 2E. Dogfooding bootstrap
- [ ] **2.16** Create the project "Build Waypoint" inside Waypoint. Import the remaining stages of this plan as tasks via diary entries (not manual entry, which is the point).
- [ ] **2.17** Use it daily for 1–2 weeks. Log friction as diary entries tagged to the project. Those entries become the Stage 3 backlog.

**Exit criteria:**
- Natural-language entries (no mock syntax) reliably produce sensible, reviewable changesets with a local model and/or a cloud model.
- The eval suite runs and establishes a baseline score.
- The owner has used Waypoint for at least one week to track its own development, and the Now screen was actually used to pick what to work on.

---

## Stage 3 — Post-MVP refinement (driven by dogfooding)

**Goal:** fix what dogfooding revealed, then add the ADHD-support features from the spec. Priorities should come from the friction log, not from this list. Candidates:

- Milestones (outcome layer above tasks).
- Energy and size filters; the "low energy? try one of these" option in the briefing. Needs a self-reported energy field on entries (removed from the Stage 0 schema; add it with a new migration) to match against `tasks.energy`.
- Staleness prompts ("still want this?") for long-untouched tasks.
- "Welcome back" recap after multi-day gaps.
- Scope-creep warnings when a project's task count grows much faster than completions.
- Task dependencies UI.
- Auto-apply for high-confidence ops (opt-in, per op type).
- Search across entries and observations.
- Mood and energy trends (gentle, not metrics-heavy). Decide first whether `feeling` observations already cover mood before adding an `entries.mood` column.

**Exit criteria:** defined at the start of the stage from the dogfooding log.

---

## Stage 4 — Packaging and desktop

**Goal:** easy to install and run for someone who isn't the author.

- [ ] **4.1** PWA manifest and service worker (installable app window).
- [ ] **4.2** Release pipeline producing signed or notarized binaries where practical, plus a Docker image.
- [ ] **4.3** Run-at-login / tray option, or a desktop shell (evaluate Wails vs. Tauri + sidecar; record an ADR). The shell supports both "local embedded server" and "connect to remote server" modes.
- [ ] **4.4** Import from JSON export; DB backup and restore commands.

**Exit criteria:** a fresh machine on each OS can go from download to first entry in under 5 minutes following the README.

---

## Later / future (not planned yet)

- Authentication and multiple users (add `user_id` scoping, sessions or tokens, and non-localhost binding with TLS guidance).
- Sync between devices, or a hosted version.
- Replaying state from entries (true event sourcing) for re-extraction with newer models.
- Mobile-friendly UI and voice entry.
- Integrations (git commits as progress signals, calendar).

---

## Working agreements

- **One stage at a time.** Finish the exit criteria before starting the next stage. If something from a later stage feels urgent, write it as a diary entry (once that works) or an issue, and park it.
- **Small, reviewable changes.** When using AI agents, give each one a single numbered task from this plan, with the relevant SPEC sections referenced.
- **Keep docs true.** If implementation changes a decision, update `SPEC.md`/`PLAN.md` or add an ADR in the same change.
- **Definition of done for any task:** code, tests, docs updated if needed, and it runs via `make dev`.

---

## Parked

Ideas and tangents captured mid-work (via `/park`). Not scheduled. Review them at the start of each stage.
- [ ] `//go:embed` skips dotfiles, so `internal/web/dist/.gitkeep` alone won't let `go build` pass before the first frontend build; embed a placeholder `index.html` or use `all:dist` (parked 2026-09-29; suggested stage: 0, with 0.7)
- [ ] SPEC §10.1 lists a `release` Makefile target that no task covers yet; add it alongside cross-compiling in CI (parked 2026-09-29; suggested stage: 0, with 0.9)
- [ ] Configurable bind host (e.g. `0.0.0.0` for Docker); only safe once auth exists, so the server stays on `127.0.0.1` for now (parked 2026-09-29; suggested stage: later)
- [ ] Test that the Go enums in `internal/domain` match the SQL `CHECK (... IN ...)` lists in the migrations (parked 2026-09-29; suggested stage: 1, with 1.1)
