# Waypoint — Implementation Plan

> Companion to [`SPEC.md`](./SPEC.md). The spec defines *what* and *why*; this plan defines *order*, *scope per stage*, and *exit criteria*. Each stage should end in a runnable, usable state. Don't pull later-stage work forward.

**Current stage: Stage 1 — Prototype (mock AI, no real model)**

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

## Stage 0 — Foundations (scaffolding) — done 2026-09-29

**Goal:** an empty but correctly wired app that builds and runs on all three OSes.

Tasks:
- [x] **0.1** Initialize the Go module, repo layout (SPEC §10.2), and a `Makefile` with `dev`, `build`, `test`, `lint`, `sqlc` targets (the commands `AGENTS.md` refers to).
- [x] **0.2** Config loading (flags > env > `config.toml`); OS-appropriate data dir for the DB.
- [x] **0.3** SQLite via `modernc.org/sqlite` with WAL mode; goose migrations embedded and run at startup.
- [x] **0.4** Initial migration with the full core schema from SPEC §6.1 (milestones table included but unused).
- [x] **0.5** sqlc setup and first generated queries.
- [x] **0.6** chi server with `/api/health`, structured logging (`log/slog`), graceful shutdown, bound to `127.0.0.1:<port>`.
- [x] **0.7** Svelte + TS + Vite + Tailwind app in `/web`; production build embedded via `embed.FS`; SPA fallback routing; dev proxy for `/api`.
- [x] **0.8** Decide API type-sharing approach and record it in `/docs/adr/0001-api-types.md`.
- [x] **0.9** CI (GitHub Actions or similar): test and cross-compile for linux/darwin/windows.

**Exit criteria:** `make build` produces a binary on each OS that opens a page in the browser showing "Waypoint" and a healthy API status. The DB file is created and migrated.

---

## Stage 1 — Prototype (mock AI, no real model)

**Goal:** the complete core loop, *write → extract → review → apply → briefing*, working end to end with the deterministic **mock extractor**. This validates the data model, pipeline, changeset system, and UI before any AI cost or unpredictability enters the picture.

### 1A. Domain and state
- [x] **1.1** Domain types, enums, and validation in `internal/domain`.
- [x] **1.2** Store layer: CRUD for projects, tasks, blockers, ideas, entries (insert-only), observations, and links. Tasks, blockers, and ideas can be deleted, cleaning up references (ADR 0002); projects can't.
- [x] **1.3** Project status rules, including the active-project cap.

### 1B. Changeset system
- [x] **1.4** Change op types and payload validation (SPEC §6.2), including `delete_task`, `delete_blocker`, and `delete_idea`, plus `update_project`, `update_blocker`, `update_idea`, and `position` on `update_task` for manual edits: a new migration rebuilds `change_ops` with the extended `op_type` CHECK list (ADR 0002). Payload validation also covers the cross-row rules the store doesn't check: a parent task or blocker task in the same project, no parent-task cycles.
- [x] **1.5** Transactional applier: per-op accept, reject, or edit; temp-ID (`$new:N`) resolution; full rollback on any failure; changeset status updates. Enforces the project status rules from 1.3 (`domain.CheckProjectStatusChange` with `tx.CountActiveProjects` and `cfg.ActiveProjectCap`) inside the apply transaction. Undecided ops, and extraction ops held back by the active-project cap, split off into a new pending changeset (ADR 0003, migration 00003); on a manual changeset an `ActiveCapError` fails the apply.
- [x] **1.6** Manual edits routed through the applier as `source=manual` changesets.
- [x] **1.7** Unit tests covering every op type, temp IDs, partial apply, and rollback.

### 1C. Pipeline
- [x] **1.8** `Extractor` interface, `ExtractionResult` types, and the shared output validator.
- [x] **1.9** Mock extractor per SPEC §10.4, with configurable artificial latency (`mock_latency` in `config.toml`). For an entry with no project it guesses one from project titles and the tasks and blockers the entry names.
- [x] **1.10** Snapshot builder: active projects plus the entry's own, open tasks and blockers, parked ideas, and the observation summaries of the last `snapshot_recent_entries` earlier entries (from any project for an unscoped entry).
- [x] **1.11** Background worker: entry saved → run created (`pending`) → extract → validate → store observations, changeset, and questions → emit SSE. Pending runs resume on restart; failures are recorded; `Worker.Reprocess` (HTTP route in 1.14, see Parked).
- [x] **1.12** SSE hub and `/api/events`.

### 1D. Briefing
- [x] **1.13** Heuristic `Briefer` (SPEC §7.3). Recompute on changeset apply and task edit.

### 1E. API
- [x] **1.14** Endpoints from SPEC §10.5, except settings UI editing and export (config file only for now). DTOs live in `internal/api/apitypes` with generated TS types (ADR 0001).
- [x] **1.15** API integration tests using the mock extractor end to end.

### 1F. UI
- [x] **1.16** App shell and navigation: Now, Diary, Projects, Parking Lot. The **Projects** screen lists projects grouped by status and has a "New project" form (title, why, definition of done, start active or parked). At the active-project cap the form says so calmly and offers to create the project parked instead.
- [ ] **1.16b** App layout and base style (ADR 0004). The **Diary** becomes a collapsible panel on the right, beside the main pane on every screen except `/diary`, which shows the same content full-page. It can be dragged to resize (within a minimum and maximum width, and with the arrow keys on the focused handle); its width and open/collapsed state are remembered in `localStorage`. On narrow windows (below Tailwind's `lg`) there is no sidebar: a floating button in the bottom-right corner opens the diary as a full-height overlay that closes with Esc or a close button. This task builds the shell and an empty panel; 1.17 and 1.18 fill it. Also a small visual foundation: palette, type scale, and spacing in the Tailwind theme, plus shared button, input, card, and panel components, applied to the existing shell and Projects screen. Calm and plain (SPEC §11 UX notes), not a design system.
- [ ] **1.17** **Now** screen: briefing cards. **Diary input** in the diary panel (Cmd/Ctrl+Enter) with an optional project scope picker that defaults to the project on screen, if any.
- [ ] **1.18** **Diary** timeline, built to fit both the diary panel and the full-page `/diary` view: entries with observation highlights; inline pending changeset review (accept, reject, edit per op, accept-all); follow-up questions with answer and dismiss. For an entry with no project whose changeset puts things in one, a "Project: X (guessed)" picker lets the user choose another project or none: another project rewrites `project_id` on those ops; none rejects the task and blocker ops and removes the project from the ideas.
- [ ] **1.19** **Project** screen: header with editable title, why, and definition of done; project status changes (active, paused, parked, done) with the same calm handling at the active-project cap; task list with add task, inline edit, status changes and reordering; blockers with resolve; ideas. Deleting a task or blocker asks for confirmation.
- [ ] **1.20** **Task detail:** origin entry and linked observations timeline.
- [ ] **1.21** **Parking lot:** ideas and parked projects; promote to task or project; activate a parked project (same cap handling as 1.16); delete an idea (with confirmation).
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
- [ ] SPEC §10.1 lists a `release` Makefile target that no task covers yet; CI (0.9) cross-compiles inline, so a `release` target belongs with the release pipeline (parked 2026-09-29; suggested stage: 4, with 4.2)
- [ ] Configurable bind host (e.g. `0.0.0.0` for Docker); only safe once auth exists, so the server stays on `127.0.0.1` for now (parked 2026-09-29; suggested stage: later)
- [ ] Deleting a project. Blocked by entry immutability (entries reference projects); needs a decision on what happens to its entries, tasks, and ideas. Parking or finishing covers it for now (parked 2026-09-29; suggested stage: later)
- [ ] Manage user-facing settings (caps and similar preferences) in the app, beyond the provider settings in 2.12, instead of only via `config.toml`/env; technical settings such as port can stay in `config.toml` (parked 2026-09-29; suggested stage: 3)
- [ ] `add_dependency` doesn't check for dependency cycles (A blocked by B blocked by A); it needs a store query for a task's dependencies. Add it with the task dependencies UI (parked 2026-09-29; suggested stage: 3)
- [ ] Reprocess UI: before reprocessing, warn that the entry's pending changesets will be rejected and its open questions dismissed, showing how many ops are affected; confirm to continue (parked 2026-09-30; suggested stage: 1, with 1.18)
- [ ] The heuristic's entry excerpt is read fresh (newest observation linked to the next task). If the LLM briefer should choose the excerpt itself, it needs somewhere to store it, such as a nullable `excerpt_observation_id` on `briefings`. Also: `briefing.Service` refreshes synchronously after each apply and holds a mutex across the Briefer call, which is fine for the heuristic but would put a model call on the apply request path; the LLM briefer should refresh in the background (parked 2026-09-30; suggested stage: 2, with the LLM briefer)
- [ ] Manual blocker creation: `POST /api/projects/{id}/blockers` as a `create_blocker` manual changeset. SPEC §10.5 has no route for it; blockers come from entries for now, and PATCH/DELETE cover editing (parked 2026-09-30; suggested stage: 1, with 1.19)
- [ ] Client-side navigation scrolls to the top but doesn't move focus or announce the new page, so screen-reader users get no cue. Focus the new screen's `<h1>` (`tabindex="-1"`) after `navigate` (parked 2026-09-30; suggested stage: 1, with 1.22)
