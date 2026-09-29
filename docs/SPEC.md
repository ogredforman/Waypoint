# Waypoint — System Specification

> **Working title.** "Waypoint" is a placeholder name used for the binary, module path, and docs. Rename freely.
>
> **Audience:** the project owner and AI coding agents. This is the source of truth for *what* the system is and *why*. For *how and in what order* to build it, see [`PLAN.md`](./PLAN.md).
>
> **Status:** Pre-prototype. Local-first, single user, no auth.

---

## 1. Summary

Waypoint is a personal project-planning and progress-tracking app whose main control surface is an **AI-assisted diary**. Instead of manually maintaining task lists, the user writes free-form entries about what they worked on, what they're thinking, how they feel, and what's in their way. The system extracts structured information from each entry, proposes updates to the project's state (tasks, blockers, ideas, progress), links every thought back to the part of the project it concerns, and keeps a continuously updated **briefing** that tells the user what to do next.

It is built for individuals who start ambitious projects but struggle with focus, starting, and follow-through (for example, people with ADHD). The core insight behind it: *a detailed, current writeup of "where I am and what's next" keeps work moving.* Waypoint generates and maintains that writeup automatically from the user's own reflections.

## 2. Problem statement

- Ambitious goals stall because the next concrete action is unclear, starting is hard, and context is lost between sessions.
- Traditional todo apps require up-front, disciplined data entry, which is exactly the thing that breaks down.
- Writing a detailed plan helps, but keeping it current is its own chore, so it goes stale.

## 3. Goals

1. **Capture with near-zero friction.** Writing an entry must be as easy as writing in a notebook, with no required fields.
2. **Turn reflection into structure.** Extract progress, blockers, ideas, decisions, and feelings from entries and apply them to project state.
3. **Keep the user in control.** The AI *proposes* changes; the user reviews and applies them.
4. **Always answer "what should I do next?"** Keep a single, concrete next action visible, along with a "where you left off" recap.
5. **Make everything traceable.** Every task, blocker, and status change links back to the entry text that produced it.
6. **Be easy to run locally.** Ship as one binary with an embedded UI and a single SQLite file, running on Linux, Windows, and macOS.

## 4. Non-goals (for now)

- Authentication, multiple users, sharing, or collaboration.
- Cloud hosting, sync between devices, or mobile-native apps.
- Calendars, time blocking, reminders, or notifications.
- Team or project-management features (assignees, sprints, Gantt charts).
- Full event-sourced replay of state from entries. The design keeps this possible but doesn't implement it (see §7.1).

The design must not *preclude* auth or multi-user later. Tables carry no user scoping yet, but nothing should assume a global singleton that would be hard to scope.

## 5. Product principles

These are requirements, not suggestions. Agents should treat violations as bugs.

- **Tasks are actions, not components.** A task is something *the user does*, phrased so it can be started without further thinking ("Stub the `/login` route in `auth.go`", not "Auth"). Outcome-shaped units belong to milestones.
- **Propose, don't impose.** The AI never mutates state silently. All changes go through a reviewable changeset.
- **One next action.** The primary view shows one recommended next action with a reason. Longer lists are secondary.
- **No guilt mechanics.** No overdue counters, red badges, broken streaks, or shaming copy. Returning after a long gap gets a warm recap, not a backlog of failures.
- **Questions are few and skippable.** Follow-up questions are capped (default 2 per entry) and can always be dismissed.
- **Ideas get parked, not activated.** New ideas and project sparks are captured in a parking lot so they don't derail the current project.
- **Limit active projects.** Unlimited parked projects, but a small cap on active ones (default 3). Activating another one is a deliberate action.
- **Entries are immutable.** The user's words are never edited by the system. Corrections are new entries or manual edits to derived state.

## 6. Domain model

### 6.1 Entities

| Entity | Purpose | Key fields |
|---|---|---|
| **Project** | A goal the user is pursuing | `title`, `why` (motivation), `definition_of_done`, `status` (active / paused / parked / done), timestamps |
| **Milestone** | *Post-MVP.* An outcome-shaped chunk of a project | `project_id`, `title`, `description`, `status` (open / done / dropped), `position` |
| **Task** | A concrete action the user does | `project_id`, `milestone_id?`, `parent_task_id?`, `title`, `notes`, `status` (todo / in_progress / done / dropped), `size?` (xs / s / m / l), `energy?` (low / medium / deep), `position`, `origin_entry_id?`, timestamps |
| **Task dependency** | One task blocks another | `task_id`, `blocked_by_task_id` |
| **Entry** | A diary entry. Immutable. | `body` (markdown text), `project_id?` (user-chosen scope, or null for auto), `reply_to_question_id?`, `created_at`. (Self-reported mood and energy are deferred to Stage 3; feelings are captured as `feeling` observations.) |
| **Observation** | A typed fragment extracted from an entry | `entry_id`, `kind` (progress / blocker / idea / feeling / decision / question / note), `summary`, `span_start`, `span_end`, `extraction_run_id` |
| **Observation link** | Links an observation to a project element (many-to-many) | `observation_id`, `target_type` (project / milestone / task / blocker / idea), `target_id`, `confidence` |
| **Blocker** | Something preventing progress, with a lifecycle | `project_id`, `task_id?`, `description`, `status` (open / resolved), `origin_entry_id?`, `resolved_entry_id?`, timestamps |
| **Idea** | A parking-lot item: a new idea or project spark | `project_id?`, `title`, `body`, `status` (parked / promoted / discarded), `origin_entry_id?` |
| **Follow-up question** | A question the system asks after an entry | `entry_id`, `extraction_run_id`, `text`, `reason`, `status` (open / answered / dismissed), `answer_entry_id?` |
| **Extraction run** | One AI (or mock) processing pass over an entry | `entry_id`, `provider`, `model`, `prompt_version`, `input_snapshot?` (JSON), `raw_output?` (JSON), `status` (pending / running / succeeded / failed; `running` resets to `pending` on restart), `error?`, `latency_ms?`, `created_at` |
| **Changeset** | Changes proposed from one extraction run, or one manual edit | `source` (extraction / manual), `extraction_run_id?` (set exactly when `source` is extraction), `status` (pending / applied / partially_applied / rejected), `created_at`, `resolved_at?` |
| **Change op** | A single proposed mutation inside a changeset | `changeset_id`, `position` (order within the changeset), `op_type`, `payload` (JSON), `rationale?`, `confidence?`, `status` (pending / applied / rejected / edited), `applied_payload?` |
| **Briefing** | Cached "what's next" output per project | `project_id`, `next_task_id?`, `next_reason`, `recap`, `generated_at`, `source` (heuristic / llm) |

Validation in `internal/domain` enforces two rules the schema doesn't: a blocker has `resolved_entry_id` only when it is resolved (it may be resolved without one, by a manual edit), and a follow-up question has `answer_entry_id` exactly when it is answered.

### 6.2 Change op types

Ops are the only way extraction output mutates state. Each op payload is typed and validated server-side.

- `create_task`: {project_id, title, notes?, size?, energy?, parent_task_id?, milestone_id?}
- `update_task`: {task_id, fields to change}
- `set_task_status`: {task_id, status}
- `split_task`: {task_id, subtasks[]} (breaks a vague task into startable steps)
- `create_blocker`: {project_id, task_id?, description}
- `resolve_blocker`: {blocker_id}
- `park_idea`: {project_id?, title, body}
- `create_project`: {title, why?} (always created as `parked`)
- `add_dependency`: {task_id, blocked_by_task_id}

References to entities created earlier in the same changeset use temporary IDs (e.g. `"$new:1"`), which are resolved at apply time.

### 6.3 Relationships (overview)

```
Project 1─* Milestone 1─* Task
Project 1─* Task (milestone optional)
Task    *─* Task            (dependencies, parent/subtask)
Project 1─* Blocker *─1 Task (optional)
Entry   1─* Observation *─* {Project, Milestone, Task, Blocker, Idea}
Entry   1─* ExtractionRun 1─0..1 Changeset 1─* ChangeOp
Changeset (source=manual, no run) 1─* ChangeOp
Entry   1─* FollowUpQuestion ─(answer)→ Entry
```

## 7. Core flows

### 7.1 Entry processing pipeline

1. **Write.** The user submits an entry, optionally scoped to a project (default: "auto").
2. **Persist.** The entry is stored immediately and the UI returns. Processing is asynchronous.
3. **Snapshot.** Build a compact state snapshot: active projects; their open tasks (id, title, status); open blockers; parked ideas (titles only); and the last N entry summaries for the relevant project.
4. **Extract.** Call the configured `Extractor` with (entry, snapshot). It returns a validated `ExtractionResult`:
   - `observations[]`: kind, summary, text span, links[] with confidence
   - `ops[]`: proposed change ops with rationale and confidence
   - `questions[]`: at most 2 follow-up questions, each with a reason
5. **Record.** Store the extraction run (including raw output), observations, the changeset, and questions.
6. **Notify.** Push a server-sent event (`changeset.ready`) to the UI.
7. **Review.** The user accepts, rejects, or edits each op, or accepts all. Applied ops mutate state inside a single DB transaction.
8. **Refresh briefing.** Recompute the briefing for affected projects.

**Auditability without full event sourcing:** entries are immutable and every state mutation happens through a recorded change op (including manual edits, recorded as a changeset with `source=manual`). That keeps a complete history of *why* state is the way it is. Replaying state from entries is a possible future capability, not a current requirement.

### 7.2 Follow-up questions

The extractor generates questions when useful information is missing. Examples:
- A task has been in progress for a long time and no blocker has been mentioned.
- Progress was reported but no next step is clear.
- The entry has a negative tone and the scope might be the problem.

Answering a question creates a new entry with `reply_to_question_id` set, which goes through the normal pipeline. Dismissed questions are recorded so the same question isn't asked again right away.

### 7.3 Briefing ("Now" view)

For each active project, the briefing shows:
- **Next action:** exactly one task, plus a one-sentence reason and the most relevant recent entry excerpt.
- **Where you left off:** a 2–4 sentence recap of recent entries.
- **Open blockers.**
- **Low-energy alternatives:** up to 2 small or low-energy tasks.

Two implementations exist behind one interface:
- *Heuristic*, used in the prototype and as a fallback: prefer in-progress tasks, then unblocked todo tasks by `position`, preferring smaller sizes.
- *LLM*: selects and explains using the snapshot. It must return a task ID that exists; otherwise the heuristic is used.

### 7.4 Manual editing

The user can always edit tasks, blockers, ideas, and projects directly. Manual edits are recorded as a `manual` changeset so the audit trail stays complete.

## 8. Functional requirements

Requirements are tagged by the stage in which they're first needed: **[P]** Prototype, **[M]** MVP, **[L]** Later.

**Projects**
- [P] Create, view, edit, and change status of projects.
- [P] Enforce the active-project cap (configurable, default 3).
- [L] Milestones.

**Tasks**
- [P] CRUD, status changes, and ordering within a project.
- [P] Show where each task came from (origin entry) and all linked observations.
- [M] Split a task into subtasks via a change op.
- [L] Dependencies UI, energy/size filters, and staleness prompts ("still want this?").

**Diary**
- [P] Write an entry, with an optional project scope.
- [P] Diary timeline showing extracted observations highlighted inline on each entry.
- [P] Pending changeset review UI: per-op accept, reject, and edit, plus accept-all.
- [P] Follow-up questions shown after an entry; answer or dismiss.
- [M] Real LLM extraction with validated structured output.

**Blockers and ideas**
- [P] Blocker list per project; resolve manually or via op.
- [P] Parking lot of ideas; promote an idea to a task or project.

**Briefing**
- [P] Heuristic "Now" view.
- [M] LLM-generated next action and recap.
- [L] Scope-creep warnings and "welcome back" recaps after long gaps.

**Settings**
- [P] Choose the extractor provider (mock / anthropic / openai-compatible), model, and API key or endpoint, via config file or env vars.
- [M] The same settings editable in the UI.

## 9. Non-functional requirements

- **Local-first:** runs entirely on the user's machine. Binds to `127.0.0.1` by default. All data lives in one SQLite file. The only network traffic is to the configured LLM endpoint, and none at all when using the mock or a local model.
- **Portable:** a single binary per OS (linux/amd64, linux/arm64, darwin/arm64, darwin/amd64, windows/amd64) with the frontend embedded. No CGO. A Docker image is optional.
- **Responsive:** saving an entry returns in under 100 ms locally. Extraction runs in the background.
- **Resilient to AI failure:** invalid or failed extraction must never corrupt state. The entry is saved regardless; the run is marked failed and can be retried.
- **Privacy:** diary content is sensitive. Never log entry bodies at info level. Keep API keys out of the DB export and out of logs.
- **Portable data:** the user can export everything as JSON and back up the DB by copying the file.

## 10. Architecture

```
┌──────────────────────── waypoint (single Go binary) ────────────────────────┐
│                                                                             │
│  Embedded SPA (Svelte, built to static files, served via embed.FS)          │
│        │  REST /api/*      ▲ SSE /api/events                               │
│        ▼                   │                                                │
│  HTTP layer (chi) ── handlers ── services ──────────────┐                   │
│                                   │                     │                   │
│              ┌────────────────────┼──────────────┐      │                   │
│              ▼                    ▼              ▼      ▼                   │
│        Pipeline (worker)   Changeset applier  Briefing  Store (sqlc)        │
│              │                                          │                   │
│              ▼                                          ▼                   │
│        Extractor interface                         SQLite (modernc)         │
│        ├─ MockExtractor (rule-based, deterministic)                         │
│        ├─ AnthropicExtractor                                                │
│        └─ OpenAICompatExtractor (OpenAI, Ollama, LM Studio, llama.cpp)      │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 10.1 Technology choices

| Concern | Choice | Notes |
|---|---|---|
| Language | Go (latest stable) | |
| HTTP router | `go-chi/chi` | Standard-library-compatible |
| Database | SQLite via `modernc.org/sqlite` | Pure Go, no CGO, so cross-compiling is trivial. WAL mode. |
| Queries | `sqlc` | Type-safe generated code from SQL |
| Migrations | `pressly/goose` with embedded SQL files | Run automatically at startup |
| Background work | In-process worker goroutine + channel; jobs persisted as extraction runs with `status=pending` | Pending runs resume on restart |
| Realtime | Server-Sent Events | Simpler than WebSockets; one direction is enough |
| Frontend | Svelte 5 + TypeScript + Vite, built as a static SPA | Embedded via `embed.FS`. Dev mode uses the Vite dev server proxying `/api`. |
| Styling | Tailwind CSS | |
| API types | Go DTO structs → TS types generated with `tygo` | Go is the source of truth; generated TS is committed and checked by `make lint`. Some Go types need mapping rules (`time.Time`, nullable fields). See [ADR 0001](./adr/0001-api-types.md). |
| LLM | Anthropic Go SDK or plain HTTP; OpenAI-compatible HTTP client | Structured output via tool/function calling with a JSON schema, validated server-side |
| Config | Flags > env vars (`WAYPOINT_*`) > `config.toml` in the OS config dir | |
| Desktop | PWA first; Wails or Tauri shell later | |
| Build | `Makefile` | `dev`, `build`, `test`, `lint`, `sqlc`, `release` targets |

### 10.2 Repository layout

```
/cmd/waypoint/            main.go — flags, config, wiring, server start
/internal/config/         config loading
/internal/db/             migrations/ (goose .sql), queries/ (sqlc .sql), dbgen/ (sqlc-generated Go)
/internal/domain/         core types, enums, validation (no I/O)
/internal/store/          repository layer over sqlc
/internal/extract/        Extractor interface, types, JSON schema, prompt templates
/internal/extract/mock/   rule-based mock extractor
/internal/extract/anthropic/
/internal/extract/openaicompat/
/internal/pipeline/       snapshot builder, worker, retries
/internal/changeset/      op validation, temp-ID resolution, transactional apply
/internal/briefing/       heuristic + LLM briefing
/internal/api/            handlers, SSE hub
/internal/api/apitypes/   wire DTOs + SSE payloads; source for generated TS types (ADR 0001)
/internal/web/            embed.FS for built frontend
/web/                     Svelte app (Vite project)
/docs/                    SPEC.md, PLAN.md, adr/ (architecture decision records)
/testdata/                fixture entries + expected extraction output
/tools/hooks/             Claude Code hook program (separate Go module, not part of the app)
/.claude/                 Claude Code rules, skills, agents, settings
```

### 10.3 Key interfaces (Go, indicative)

```go
type Extractor interface {
    Extract(ctx context.Context, in ExtractInput) (ExtractionResult, error)
    Name() string // "mock", "anthropic", ...
}

type ExtractInput struct {
    Entry    Entry
    Snapshot StateSnapshot
    PromptVersion string
}

type ExtractionResult struct {
    Observations []ObservationDraft
    Ops          []ChangeOpDraft
    Questions    []QuestionDraft // len <= MaxQuestions
}

type Briefer interface {
    Brief(ctx context.Context, projectID int64, snap StateSnapshot) (Briefing, error)
}
```

Every extractor's output passes through the **same validator** before storage. The validator checks that referenced IDs exist, enforces enum values and caps, and verifies text spans fall within the entry.

### 10.4 Mock extractor

The mock is deterministic and rule-based, so the whole system can be built and tested without an AI model. Line prefixes map to observations and ops:

| Line prefix | Result |
|---|---|
| `done: <text>` | progress observation; `set_task_status` done on the best fuzzy title match, or `create_task` + done |
| `doing: <text>` | progress observation; `set_task_status` in_progress |
| `todo: <text>` | `create_task` |
| `blocked: <text>` | blocker observation; `create_blocker` |
| `unblocked: <text>` | `resolve_blocker` on the best match |
| `idea: <text>` | idea observation; `park_idea` |
| `feel: <text>` | feeling observation |
| `decided: <text>` | decision observation |
| anything else | note observation linked to the scoped project |

Questions: if the entry has no `todo:`/`doing:` lines, ask "What's the next step?" If a task has been in progress for more than 3 days with no blocker, ask about blockers. The mock may add artificial latency (configurable) to exercise the async UI.

### 10.5 API outline

```
GET    /api/projects                  POST /api/projects
GET    /api/projects/{id}             PATCH /api/projects/{id}
GET    /api/projects/{id}/tasks       POST /api/projects/{id}/tasks
PATCH  /api/tasks/{id}                GET  /api/tasks/{id} (incl. linked observations + origin)
GET    /api/projects/{id}/blockers    PATCH /api/blockers/{id}
GET    /api/ideas                     PATCH /api/ideas/{id}   POST /api/ideas/{id}/promote
GET    /api/entries?project_id=&before=   POST /api/entries
GET    /api/entries/{id}              (incl. observations, questions, changeset)
POST   /api/entries/{id}/reprocess
GET    /api/changesets?status=pending
POST   /api/changesets/{id}/apply     body: { ops: [{id, decision: accept|reject|edit, payload?}] }
POST   /api/questions/{id}/dismiss
GET    /api/projects/{id}/briefing    POST /api/projects/{id}/briefing/refresh
GET    /api/settings                  PATCH /api/settings
GET    /api/export
GET    /api/events                    (SSE: entry.processed, changeset.ready, briefing.updated, run.failed)
```

## 11. UI surfaces

1. **Now** (home): a briefing card per active project, with the diary input directly below it. This is the screen the user lives on.
2. **Diary**: a timeline of entries with observation highlights. Clicking a highlight jumps to the linked task or blocker. Pending changesets appear inline beneath the entry that produced them.
3. **Project**: header (why, definition of done), task list grouped by status, blockers, ideas for this project, and recent entries.
4. **Task detail**: fields, origin entry, and a chronological list of every linked observation ("everything I've thought about this").
5. **Parking lot**: ideas and parked projects.
6. **Settings**: provider, model, keys, caps.

UX notes: keyboard-first diary input (Ctrl/Cmd+Enter to submit); calm visual design; no red "overdue" states; empty states that encourage starting small.

## 12. Quality and testing

- **Unit tests:** validator, changeset applier (including temp IDs and rollback on failure), heuristic briefing, mock extractor.
- **Integration tests:** an HTTP API test harness against a temp SQLite DB, using the mock extractor end to end.
- **Extraction evals ([M]):** `/testdata/entries/*.md` paired with expected observations and ops. Run a scoring command (`waypoint eval`) against any provider to catch prompt regressions.
- **Frontend:** component tests for the changeset review UI; one Playwright smoke test of the full write → review → apply loop.

## 13. Open questions

- Should entries scoped to "auto" be allowed to link across multiple projects in one extraction? (Default: yes, but ops for non-active projects become `park_idea` instead.)
- Auto-apply threshold for high-confidence ops: off by default; revisit after dogfooding.
- Desktop shell: Wails or Tauri with a sidecar. Defer until after MVP.

## 14. Guidance for AI agents working on this repo

Agent instructions live in [`AGENTS.md`](../AGENTS.md) at the repo root (imported by `CLAUDE.md`), with
area-specific rules in `.claude/rules/`. Keep them there rather than here, so there is one source of truth.
If a rule in those files contradicts this spec, the spec wins and the rule should be fixed.

## 15. Glossary

- **Entry:** one diary submission.
- **Observation:** a typed piece of meaning extracted from an entry.
- **Changeset / op:** a reviewable bundle of proposed state changes, and one change within it.
- **Briefing:** the generated "what's next and where you left off" summary.
- **Parking lot:** where ideas and inactive projects wait without demanding attention.
- **Dogfooding:** using Waypoint to track building Waypoint (the MVP's success test).
