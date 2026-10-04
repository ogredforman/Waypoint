# Waypoint — Implementation Plan

> Companion to [`SPEC.md`](./SPEC.md). The spec defines *what* and *why*; this plan defines *order*, *scope per stage*, and *exit criteria*. Each stage should end in a runnable, usable state. Don't pull later-stage work forward.

**Current stage: Stage 1.5 — Foundation refinement**

Task IDs are `<stage>.<task>` (e.g. `1.5`, or `1.5.3` in Stage 1.5). Tick tasks `[x]` as they're finished. `/devflow:next-task` picks the first unchecked task in the current stage; `/devflow:park` adds ideas to the Parked section at the bottom.

## Background (conversation summary)

- **Idea:** a personal planning app where an AI-assisted diary is the main input. Free-form entries about progress, thoughts, feelings, and blockers are turned into structured project state (tasks, blockers, ideas), with every thought linked to the part of the project it concerns.
- **Motivation:** the owner repeatedly sets ambitious goals and struggles to follow through (focus, starting, context loss; ADHD). Detailed "what's next" writeups have helped. Waypoint automates keeping that writeup current.
- **Key design decisions:**
  - Entries are immutable. AI output is a *proposed changeset* the user reviews.
  - Tasks are concrete actions, not project components.
  - The home screen shows one next action and a "where you left off" recap.
  - No guilt mechanics. Ideas get parked. Active projects are capped.
- **Stack:** Go single binary + embedded Svelte SPA + SQLite (pure Go driver) + SSE. Pluggable extractor: mock, Anthropic, or OpenAI-compatible (covers local models).
- **Scope now:** local-first, single user, no auth. One person can keep separate *profiles* (e.g. Personal, Portfolio), each its own database (Stage 1.5). Auth, multi-user, and sync may come later.
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

## Stage 1 — Prototype (mock AI, no real model) — done 2026-10-01

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
- [x] **1.16b** App layout and base style (ADR 0004). The **Diary** becomes a collapsible panel on the right, beside the main pane on every screen except `/diary`, which shows the same content full-page. It can be dragged to resize (within a minimum and maximum width, and with the arrow keys on the focused handle); its width and open/collapsed state are remembered in `localStorage`. On narrow windows (below Tailwind's `lg`) there is no sidebar: a floating button in the bottom-right corner opens the diary as a full-height overlay that closes with Esc or a close button. This task builds the shell and an empty panel; 1.17 and 1.18 fill it. Also a small visual foundation: a dark theme with purple as the accent colour, and subtle gradients and glows (panel edges, focused inputs, the primary button, the next-action card), with text contrast meeting WCAG AA. Palette, type scale, and spacing go in the Tailwind theme as named tokens so another theme can be added later, plus shared button, input, card, and panel components, applied to the existing shell and Projects screen. Calm and plain (SPEC §11 UX notes), not a design system.
- [x] **1.17** **Now** screen: briefing cards. **Diary input** in the diary panel (Cmd/Ctrl+Enter) with an optional project scope picker that defaults to the project on screen, if any.
- [x] **1.18a** **Diary** timeline, built to fit both the diary panel and the full-page `/diary` view: entries newest first with paging; observation highlights that open the linked task or project; follow-up questions with answer and dismiss.
- [x] **1.18b** Inline pending changeset review under each entry (accept, reject, edit per op, accept-all; rejecting an op also rejects ops that use what it creates). For an entry with no project whose changeset puts things in one, a "Project: X (guessed)" picker lets the user choose another project or none: another project rewrites `project_id` on those ops; none rejects the task and blocker ops and removes the project from the ideas. Adds `jsdom` (dev) for a component test of the review UI (SPEC §12).
- [x] **1.18c** Mock extractor: don't ask "What's the next step?" on an entry that answers that question (`reply_to_question_id` names a question with the same text), so answering without a `todo:`/`doing:` line doesn't raise the same question again under the reply. The extractor may need the replied-to question's text in `ExtractInput`; update SPEC §10.4.
- [x] **1.19a** **Project** screen header: editable title, why, and definition of done; project status changes (active, paused, parked, done) with the same calm handling at the active-project cap as 1.16. The API client handles 204 responses and gains PATCH and DELETE helpers.
- [x] **1.19b** **Project** screen tasks: task list grouped by status with add task, inline edit, status changes, and reordering (move up/down swaps `position` with the neighbouring task in the same group); deleting a task asks for confirmation.
- [x] **1.19c** **Project** screen blockers and ideas: open and resolved blockers with resolve, deleting a blocker asks for confirmation; this project's parked ideas (promote and delete come in 1.21).
- [x] **1.19d** Atomic task reorder: `POST /api/tasks/{id}/move` (`{"direction": "up"|"down"}`) swaps `position` with the neighbouring task in the same status group as one `source=manual` changeset (two `update_task` ops, one briefing refresh); the Project screen's move up/down uses it instead of two PATCHes.
- [x] **1.20** **Task detail:** origin entry and linked observations timeline.
- [x] **1.21** **Parking lot:** ideas and parked projects; promote to task or project; activate a parked project (same cap handling as 1.16); delete an idea (with confirmation).
- [x] **1.22a** Live updates, part 1: a `changeset.applied` SSE event after every applied changeset (`changeset.AfterApplies` runs the briefing service and the hub); one shared `EventSource` in `web/src/lib/events.ts` with reconnect handling; the diary timeline reloads entries as their runs finish and shows a new entry as "processing" straight away; the Now screen updates on `briefing.updated` and `changeset.applied`.
- [x] **1.22b** Live updates, part 2: the Project screen (header, tasks, blockers, ideas), Parking lot, Projects list, and Task detail reload on `changeset.applied` (for their project where it applies; ideas aren't in `project_ids`, so idea lists reload on any applied changeset) and on reconnect.
- [x] **1.23** Playwright smoke test: create project → write entry with `todo:`/`blocked:` lines → apply → task and blocker appear → briefing updates.
- [x] **1.24** Reprocess an entry from the diary timeline (SPEC §7.1: "The UI warns about this before reprocessing"). `POST /api/entries/{id}/reprocess` exists (1.14) but nothing in the UI calls it, so a failed run is a dead end. Each entry card offers "Read again" (shown on a failed run, and in a quiet menu otherwise; hidden while a run is pending or running). If the entry has pending changesets or open questions, a confirmation says calmly that they'll be replaced and shows how many ops are affected; with none, it reprocesses straight away. A `409` (run in progress) shows a calm notice. The card shows "reading" right away and updates through the existing SSE events. Stage 2's retry button (2.5) builds on this.
- [x] **1.25** Manual blocker creation: `POST /api/projects/{id}/blockers` (`{description, task_id?}`) as one `source=manual` changeset with a `create_blocker` op, mirroring `POST /api/projects/{id}/tasks`; add it to SPEC §10.5. The Project screen's blockers section gets an "Add blocker" form with an optional task picker for the project's open tasks. API integration test and a component test.
- [x] **1.26** Focus on navigation: after client-side `navigate`, move focus to the new screen's `<h1>` (`tabindex="-1"`, no visible focus ring for this programmatic focus) so screen-reader users hear the new page. Every screen has an `<h1>` (the Project screen's editable title included). Not on the first load, and not when only the query string changes.

**Exit criteria:**
- Using only the mock syntax, the owner can create a project, write entries, review and apply changes, and see tasks, blockers, ideas, and the Now briefing update live.
- Every task shows the entry it came from.
- Killing the app mid-processing and restarting resumes the pending run.
- All tests pass in CI.

---

## Stage 1.5 — Foundation refinement

**Goal:** settle the base the rest of the app builds on (appearance, settings, profiles, deletion, and the shape of tasks) now that the Stage 1 frontend is close to its finished form, so Stage 2 and later don't redo it. Comes from the owner's Stage 1 review (`docs/Stage-1-Review.md`). The core loop keeps working throughout: `make e2e` passes after every task.

### 1.5A. Fixes and visual basics
- [x] **1.5.1** Icons via `@lucide/svelte` (only the icons used are bundled). The collapsed diary sidebar shows an icon with a tooltip instead of sideways text. The diary's "Save" button becomes "Write Entry".
- [x] **1.5.2** Button variants as shared components: `primary` (filled accent), `secondary` (outlined, e.g. Edit), `quiet` (subtle background, for actions that are bare text today such as "more" and "read again"), and `danger` (red, for delete and remove). Apply them on every screen. Add to SPEC §11: red marks destructive buttons only, never state.
- [x] **1.5.3** Text areas grow with their content up to a maximum height and scroll only past it, with no resize handle. A thin scrollbar styled from theme tokens everywhere. Remove the health footer; show a calm banner only when the server can't be reached.
- [x] **1.5.4** A `plan` observation kind (new migration rebuilding the `observations.kind` CHECK) with its own highlight colour. The mock's `todo:` lines produce one, so a new task is visible in the timeline, not only in the changeset. Update SPEC §6.1 and §10.4 and MOCK-TESTING.md.
- [x] **1.5.5** Observation links to entities the same changeset creates: an extractor link may name a temp ID (`$new:N`); it is stored pending, resolved when that op is applied, and dropped if the op is rejected. The mock links `todo:`/`doing:`/`done:` highlights to the tasks they create, so Task detail shows them. Plan first (touches `internal/changeset` and the pipeline); update SPEC §6.2 and §7.1.
- [x] **1.5.5b** The diary's "About" project picker stays current: `DiaryInput` reloads its project list on `changeset.applied` and on reconnect (as `liveReload` does for the other screens), so a project created, renamed, or moved between statuses shows up without a page reload.
- [x] **1.5.5c** "From the diary" on the Project screen (the "recent entries" SPEC §11 lists but Stage 1 didn't build): the observations linked to the project (notes, decisions, feelings, progress, and the rest), newest first, each with its kind's highlight colour (the link to its entry is parked: nothing can link to one entry yet), with a "show more" past the first page. A read endpoint for a project's linked observations. Task detail's observation list is the model.
- [x] **1.5.5d** The next step lives on the project: the Project screen shows the project's next action from its briefing at the top (task, reason, and a link to the task), and says so calmly when there isn't one. In the mock, a reply to "What's the next step?" treats each plain line as a `todo:` line, so answering proposes a task instead of leaving a note. Update SPEC §10.4 and MOCK-TESTING.md.
- [x] **1.5.5e** A Now screen you can read at a glance. Today "Where you left off" is one run-on string ("Earlier: …; … Then: … Most recently: …") with raw summaries strung together. Instead, each project is its own clearly separated block; the next action stands out first; "Where you left off" is a short list of recent entries, newest first, each with its relative date and its summaries shown as separate items in their kinds' highlight colours, linking to the entry. Blockers and low-energy alternatives stay brief, below. Plan first: the briefing's recap becomes structured (entries with dates and typed summaries) instead of one string, which changes the `briefings` table, the API type, and the heuristic; settle how the Stage 2 LLM recap (2.9) fits the same shape. Update SPEC §7.3 and §11. The links to entries moved to 1.5.5f.
- [x] **1.5.5f** Link to a single diary entry: `/diary?entry=N` loads the timeline back as far as that entry, scrolls to it, and briefly highlights it. The Now screen's "Where you left off" entries, "From the diary" on the Project screen, and Task detail's origin entry and observations link to it. Update SPEC §11.

### 1.5B. Settings and profiles
- [x] **1.5.6** Settings store: a `settings` table (new migration) and `GET`/`PATCH /api/settings`. User preferences (active-project cap, questions per entry, and later theme and fonts) live here; `config.toml` keeps technical settings (port, data dir, mock latency), and its preference values only seed a new profile's defaults. Update SPEC §10.5.
- [x] **1.5.7** **Settings** screen, starting with the caps. Lowering the active-project cap below the current number of active projects deactivates nothing; it applies to the next activation.
- [x] **1.5.8** Profiles (ADR): each profile is its own SQLite file in the data dir, with a display name. The existing database becomes a profile named "Personal". The server opens every profile at startup, each with its own store, worker, SSE hub, and briefing service; pending runs resume in every profile. The API names the profile in the path, since `EventSource` can't send headers (exact shape settled in the ADR). Endpoints to list, create, rename, and delete profiles; deleting removes the database file (with its `-wal`/`-shm`), and the last profile can't be deleted. Update SPEC §4, §6, and §10.
- [x] **1.5.9** Profile switcher in the app shell; the browser remembers the active profile. Create and rename profiles; delete one from Settings, confirmed by typing its name, with copy saying plainly that everything in it goes. Deleting the active profile switches to another first.

### 1.5C. Appearance
- [x] **1.5.10** Fonts bundled with the app (`@fontsource`, no external CDN). Two separate per-profile settings, each with a live preview in Settings: an **interface font** (Inter, Geist, Manrope, IBM Plex Sans, Atkinson Hyperlegible Next) and a **diary font** (the same five plus Literata, a serif, as the default).
- [x] **1.5.11** Themes, per profile, chosen in Settings with preview swatches. Dark: **Midnight** (the current purple) and **Forest** (deep green, teal accent). Light: **Paper** (warm off-white, indigo accent), **Sage** (soft green), and **Dawn** (cream, amber accent). Every theme meets WCAG AA text contrast, and no accent is near the `danger` red. A new profile copies the current profile's appearance.
- [x] **1.5.11b** Appearance applies instantly: choosing a theme or font in Settings applies it to the whole app and saves it at once, with no Save button. A failed save puts the previous choice back and says so in one line.

### 1.5D. Organising and deleting
- [x] **1.5.12** Archive projects (ADR): an `archived` project status that hides the project from the Now screen, lists, project pickers, and the extraction snapshot. An "Archived" group on the Projects screen restores a project to `parked`. Update SPEC §6.
- [x] **1.5.13** Delete a project (ADR, replacing "projects can't be deleted" in SPEC §6.2): a `delete_project` op in a `source=manual` changeset removes the project with its tasks, blockers, ideas, milestones, briefing, and links. Its diary entries are either kept (unscoped, text untouched) or deleted too (as in 1.5.14). Backend: migration, store, op, API (`DELETE /api/projects/{id}?entries=keep|delete`, `GET /api/projects/{id}/deletion`), ADR 0008. The UI moved to 1.5.13b.
- [x] **1.5.13b** Delete a project from the Project screen: a `danger` **Delete project** button; the confirmation shows what goes with it (`GET /api/projects/{id}/deletion`) and asks about its diary entries: **keep them** (the default; they become unscoped) or **delete them too**. Afterwards the app goes to Projects; other open screens showing the project cope with it being gone.
- [x] **1.5.14** Delete a diary entry: removes the entry with its runs, observations and links, follow-up questions, and pending changesets. Changes already applied from it stay, with their origin cleared. A question it answered becomes dismissed; replies to its questions lose the reply link. Entries are still never *edited*. Offered from each entry card's menu, with confirmation. Update SPEC §5, §6.2, and §7.1.
- [x] **1.5.15** Many projects stay tidy: the Projects screen gets a filter box, sorts each status group by last activity (`last_activity_at` on the API's project: its own edits, its tasks, blockers, and ideas, and its newest entry), and collapses Done and Archived by default. The pickers moved to 1.5.15a.
- [x] **1.5.15a** Searchable project pickers: a shared project combobox (type to narrow, arrow keys and Enter to pick, Escape to close) replaces the `<select>` in the diary scope and the guessed-project picker in review. "No project" stays a choice, and a done or archived project still shows when it's the one already chosen.

### 1.5E. Richer tasks
- [x] **1.5.15b** Blockers on a task, optionally (the schema, `create_blocker`, and `update_blocker` already take a `task_id`): a blocker row can be moved onto a task or back to the whole project; Task detail lists the task's open blockers with add and resolve; in the mock, a `blocked:` line that names an open task in the project (best match, as `done:` does) proposes a blocker on that task, otherwise a project-wide one. Update SPEC §10.4 and MOCK-TESTING.md.
- [x] **1.5.16** Subtasks in the UI: the Project screen nests subtasks under their parent; Task detail shows them as a checklist with **Add step** (`create_task` with `parent_task_id`), a checkbox that marks a step done (or back to todo), reordering, and delete. The briefing may recommend a step and names its parent ("step 2 of *Set up CI*").
- [ ] **1.5.17** Rich text editor (ADR; Tiptap or similar): word-processor editing with headings, bold and italic, bulleted, numbered, and check lists, and links, stored as markdown. Used for task notes, a project's why and definition of done, and an idea's body. The diary input stays plain text. Checklists in notes are free-form detail; steps the system should track are subtasks.
- [ ] **1.5.18** Milestones in the backend: `create_milestone` {project_id, title, description?}, `update_milestone` {milestone_id, title?, description?, status?, position?}, `delete_milestone` (its tasks stay, with no milestone), and `milestone_id` accepted on `create_task` and `update_task`. A new migration rebuilds `change_ops` with the extended op-type CHECK (as in 1.4). Store, validation, applier, API, and tests. Plan first (touches the schema and `internal/changeset`); update SPEC §6.
- [ ] **1.5.19** Milestones in the UI: the Project screen groups tasks by milestone (plus "No milestone") with add, edit, reorder, and mark done or dropped; tasks can move between milestones. The heuristic briefing prefers tasks in the first open milestone. Proposing milestones from diary entries waits for the Stage 2 prompt.

**Exit criteria:**
- At least two profiles in use, each with its own theme and fonts; switching is instant and nothing leaks between them.
- Projects can be archived, restored, and deleted; diary entries and profiles can be deleted.
- A task can have hand-made steps and formatted notes, and tasks can be grouped into milestones.
- A `todo:` line shows a highlight in the diary, linked to its task once applied.
- The Now screen reads at a glance: one next action per project, then where you left off as a short dated list.
- A project's screen shows its next action and what the diary has said about it; a blocker can sit on a task.
- `make test`, `make lint`, and `make e2e` pass.

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
- [ ] **2.12** Provider settings on the Settings screen (1.5.7): provider, model, endpoint, and key (key stored in config, masked in UI and excluded from export).
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

- Authentication and multiple users (add `user_id` scoping, sessions or tokens, and non-localhost binding with TLS guidance). Profiles (1.5.8) separate one person's areas of life; they are not accounts.
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

Ideas and tangents captured mid-work (via `/devflow:park`). Not scheduled. Review them at the start of each stage.
- [ ] SPEC §10.1 lists a `release` Makefile target that no task covers yet; CI (0.9) cross-compiles inline, so a `release` target belongs with the release pipeline (parked 2026-09-29; suggested stage: 4, with 4.2)
- [ ] Configurable bind host (e.g. `0.0.0.0` for Docker); only safe once auth exists, so the server stays on `127.0.0.1` for now (parked 2026-09-29; suggested stage: later)
- [x] *(Scheduled as 1.5.13.)* Deleting a project. Blocked by entry immutability (entries reference projects); needs a decision on what happens to its entries, tasks, and ideas. Parking or finishing covers it for now (parked 2026-09-29; suggested stage: later)
- [x] *(Scheduled as 1.5.6–1.5.7.)* Manage user-facing settings (caps and similar preferences) in the app, beyond the provider settings in 2.12, instead of only via `config.toml`/env; technical settings such as port can stay in `config.toml` (parked 2026-09-29; suggested stage: 3)
- [ ] `add_dependency` doesn't check for dependency cycles (A blocked by B blocked by A); it needs a store query for a task's dependencies. Add it with the task dependencies UI (parked 2026-09-29; suggested stage: 3)
- [ ] The heuristic's entry excerpt is read fresh (newest observation linked to the next task). If the LLM briefer should choose the excerpt itself, it needs somewhere to store it, such as a nullable `excerpt_observation_id` on `briefings`. Also: `briefing.Service` refreshes synchronously after each apply and holds a mutex across the Briefer call, which is fine for the heuristic but would put a model call on the apply request path; the LLM briefer should refresh in the background (parked 2026-09-30; suggested stage: 2, with the LLM briefer)
- [x] *(Scheduled as 1.5.11.)* More colour palettes or UI themes (e.g. other accent colours, a light theme), chosen in the app. 1.16b ships one dark purple theme with its colours as named tokens so more can be added (parked 2026-09-30; suggested stage: 3)
- [ ] Explore an app-managed local model: Waypoint downloads llama.cpp `llama-server` plus a small GGUF model (e.g. Qwen3-4B, Gemma 3 4B) and runs it as a localhost sidecar behind `OpenAICompatExtractor`, keeping no-CGO; decide after 2.8 evals show a small model handles extraction and snapshot linking well enough (open questions: per-OS/GPU builds, download and checksum, process lifecycle, model licenses) (parked 2026-10-01; suggested stage: 2)
- [ ] Tell the extractor the profile's question limit (e.g. a `MaxQuestions` field on `ExtractInput`) so the LLM prompt asks for no more than `questions_per_entry`; today the worker drops the extras after validation (1.5.6) (parked 2026-10-01; suggested stage: 2, with the extraction prompt)
- [ ] A `settings.updated` SSE event, so other open tabs pick up a changed cap without reloading; screens read `GET /api/settings` when they load (parked 2026-10-01; suggested stage: 1.5, with 1.5.7 if the Settings screen needs it)
- [ ] Other tabs pick up profile changes: the profile list is loaded once per page (1.5.9), so a profile added, renamed, or deleted in another tab only shows after a reload, and a tab whose profile was deleted elsewhere gets 404s until then. A `profiles.changed` server-wide event, or re-reading the list on focus, would fix both (parked 2026-10-01; suggested stage: 1.5)
- [ ] Real italic font files for the briefing quote, if the browser's synthetic slant looks bad once seen. 1.5.10 bundles only upright faces, so italics are synthesized (parked 2026-10-02; suggested stage: 1.5, with 1.5.11)
- [ ] After deleting a project, the Projects screen could say "Deleted <title>." (with what happened to its entries); today the app just lands on Projects (parked 2026-10-03; suggested stage: 1.5, with 1.5.15)
- [ ] A step's own Task detail page could say whose step it is ("Step 2 of *Set up CI*", linking to the parent), as the briefing does (parked 2026-10-04; suggested stage: 1.5 or 3)
