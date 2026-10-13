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

- Authentication, multiple users, sharing, or collaboration. Profiles (§6.4) separate one person's areas of life (e.g. Personal, Portfolio); they are not accounts.
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
- **Questions are few and skippable.** Follow-up questions are capped (at most 2 per entry, fewer if the user sets it lower) and can always be dismissed.
- **Ideas get parked, not activated.** New ideas and project sparks are captured in a parking lot so they don't derail the current project.
- **Limit active projects.** Unlimited parked projects, but a small cap on active ones (default 3). Activating another one is a deliberate action.
- **Entries are immutable.** The user's words are never edited by the system. Corrections are new entries or manual edits to derived state. The user can delete an entry whole, with what was derived from it ([ADR 0007](./adr/0007-deleting-entries.md)).

## 6. Domain model

### 6.1 Entities

| Entity | Purpose | Key fields |
|---|---|---|
| **Project** | A goal the user is pursuing | `title`, `why` (motivation), `definition_of_done` (both markdown), `status` (active / paused / parked / done / archived), timestamps. An archived project is kept for the record but hidden from the Now screen, lists, project pickers, and the extraction snapshot ([ADR 0006](./adr/0006-archived-projects.md)) |
| **Milestone** | An outcome-shaped chunk of a project | `project_id`, `title`, `description`, `status` (open / done / dropped), `position` |
| **Task** | A concrete action the user does | `project_id`, `milestone_id?`, `parent_task_id?`, `title`, `notes` (markdown), `status` (todo / in_progress / done / dropped), `size?` (xs / s / m / l), `energy?` (low / medium / deep), `position`, `origin_entry_id?`, timestamps |
| **Task dependency** | One task blocks another | `task_id`, `blocked_by_task_id` |
| **Entry** | A diary entry. Immutable. | `body` (markdown text), `project_id?` (user-chosen scope, or null for auto), `reply_to_question_id?`, `created_at`. (Self-reported mood and energy are deferred to Stage 3; feelings are captured as `feeling` observations.) |
| **Observation** | A typed fragment extracted from an entry | `entry_id`, `kind` (progress / plan / blocker / idea / feeling / decision / question / note; plan is something the user means to do), `summary`, `span_start`, `span_end`, `extraction_run_id` |
| **Observation link** | Links an observation to a project element (many-to-many) | `observation_id`, `target_type` (project / milestone / task / blocker / idea), `target_id`, `confidence` |
| **Pending observation link** | A link to an entity a pending op will create (`$new:N` in extractor output) | `observation_id`, `change_op_id`, `target_type`, `confidence`. Applying the op turns it into an observation link to the created row; rejecting the op deletes it |
| **Blocker** | Something preventing progress, with a lifecycle | `project_id`, `task_id?`, `description`, `status` (open / resolved), `origin_entry_id?`, `resolved_entry_id?`, timestamps |
| **Idea** | A parking-lot item: a new idea or project spark | `project_id?`, `title`, `body` (markdown), `status` (parked / promoted / discarded), `origin_entry_id?` |
| **Follow-up question** | A question the system asks after an entry | `entry_id`, `extraction_run_id`, `text`, `reason`, `status` (open / answered / dismissed), `answer_entry_id?` |
| **Extraction run** | One AI (or mock) processing pass over an entry | `entry_id`, `provider`, `model`, `prompt_version`, `input_snapshot?` (JSON), `raw_output?` (JSON: `{"attempts": [...]}`, one per extractor call, each with the model's `response` as the provider sent it (a JSON string if it isn't valid JSON; null for the mock), the decoded `result` (null if the extractor failed), the validator's `problems`, and `latency_ms`), `status` (pending / running / succeeded / failed; `running` resets to `pending` on restart), `error?`, `latency_ms?`, `created_at` |
| **Changeset** | Changes proposed from one extraction run, or one manual edit | `source` (extraction / manual), `extraction_run_id?` (set exactly when `source` is extraction), `parent_changeset_id?` (set when split off another changeset at apply; same source and run as the parent), `status` (pending / applied / partially_applied / rejected), `created_at`, `resolved_at?` |
| **Change op** | A single proposed mutation inside a changeset | `changeset_id`, `position` (order within the changeset), `op_type`, `payload` (JSON), `rationale?`, `confidence?`, `status` (pending / applied / rejected / edited), `applied_payload?` |
| **Briefing** | Cached "what's next" output per project | `project_id`, `next_task_id?`, `next_reason`, `recap`, `generated_at`, `source` (heuristic / llm) |

Validation in `internal/domain` enforces two rules the schema doesn't: a blocker has `resolved_entry_id` only when it is resolved (it may be resolved without one, by a manual edit), and a follow-up question has `answer_entry_id` exactly when it is answered.

### 6.2 Change op types

Ops are the only way extraction output mutates state. Each op payload is typed and validated server-side.

Tasks:
- `create_task`: {project_id, title, notes?, size?, energy?, parent_task_id?, milestone_id?} (starts as `todo`)
- `update_task`: {task_id, title?, notes?, size?, energy?, parent_task_id?, position?} (`position` orders the task within its project's task list; status changes go through `set_task_status`)
- `set_task_status`: {task_id, status}
- `split_task`: {task_id, subtasks[]} (breaks a vague task into startable steps, created as its subtasks)
- `add_dependency`: {task_id, blocked_by_task_id}
- `delete_task`: {task_id}

Blockers:
- `create_blocker`: {project_id, task_id?, description}
- `update_blocker`: {blocker_id, description?, task_id?}
- `resolve_blocker`: {blocker_id}
- `delete_blocker`: {blocker_id}

Ideas:
- `park_idea`: {project_id?, title, body?}
- `update_idea`: {idea_id, title?, body?, project_id?, status?} (promoting an idea is a status change plus the op that creates the task or project)
- `delete_idea`: {idea_id}

Milestones:
- `create_milestone`: {project_id, title, description?} (starts `open`, at the end of the project's milestones)
- `update_milestone`: {milestone_id, title?, description?, status?, position?} (`position` orders the milestone within its project)
- `delete_milestone`: {milestone_id}

Extraction may propose milestone ops and a task's `milestone_id`, naming open milestones from the snapshot or `$new:N`; the prompt asks for a new milestone only when the entry names an outcome or phase, and leaves `position` (reordering) to the user.

Extraction suggests `split_task` only for a task the entry is about (it mentions it, or says they are stuck on it, it feels too big, or they don't know where to start) that is vague (an area or outcome rather than an action, or size `l`) or marked stalled in the snapshot: 2 to 5 startable steps in order, the first small. It never splits a task the entry doesn't touch or one that already has open steps, and its rationale never mentions how long the task has waited. These are the only steps the model may propose without the entry naming them; the user reviews them like any op.

Projects:
- `create_project`: {title, why?} (always created as `parked`)
- `update_project`: {project_id, title?, why?, definition_of_done?, status?} (a status change to `active` is subject to the active-project cap)
- `delete_project`: {project_id, delete_entries?} (manual only: extraction never proposes it, and the shared validator rejects it)

The update ops change only the fields present in the payload. For the optional references and enums (`size`, `energy`, `parent_task_id`, a task's `milestone_id`, a blocker's `task_id`, an idea's `project_id`), `null` clears the field. Unknown fields are rejected. A manual "create an active project" is `create_project` followed by `update_project` in the same changeset.

References to entities created earlier in the same changeset use temporary IDs: `"$new:N"` names the entity created by the op at position `N` (positions start at 1). The referenced op must come earlier and create the right kind of entity; `split_task` subtasks can't be referenced. Temporary IDs are resolved at apply time. An observation's link may also name a temporary ID: it is stored as a pending link on the creating op, becomes a real link when that op is applied (as is or edited), follows the op if it is split off, and is dropped if the op is rejected.

Beyond the shape, ops are checked against the current state just before they are applied: referenced rows exist, a parent task, a blocker's task, or a task's milestone is in the same project, a dependency's tasks are in the same project, and a new parent doesn't create a parent-task cycle.

Deleting removes the row for good ([ADR 0002](./adr/0002-deleting-tasks-blockers-ideas.md)); the softer option is a status (dropped, resolved, discarded). Deleting a task makes its subtasks top-level, removes its dependencies, keeps its blockers on the project without a task, clears it from briefings, and removes observation links to it. Deleting a blocker or idea removes the links to it. Deleting a milestone keeps its tasks in the project with no milestone and removes the links to it. Observations and entries are never deleted by an op; deleting an entry is its own action (§7.1, [ADR 0007](./adr/0007-deleting-entries.md)). Deleting a project ([ADR 0008](./adr/0008-deleting-projects.md)) removes it with its tasks, blockers, ideas, milestones, and briefing, and the observation links to any of them. The entries scoped to it are kept, unscoped with their text untouched, unless `delete_entries` is set, in which case each is deleted as §7.1 describes, in the same transaction; that is refused (`run_in_progress`) while one of their runs is running. Parking, finishing, or archiving stays the softer option. The applier records the deleted row in the op's `applied_payload`.

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

### 6.4 Profiles

A **profile** is one of the user's separate areas (e.g. Personal, Portfolio), and each is its own
SQLite database: everything in §6.1, plus the settings (§10.5), belongs to exactly one profile, and
nothing links across profiles. A profile has an `id` (made from its name at creation, never changed;
also its file name, `<data_dir>/profiles/<id>.db`) and a display `name` (trimmed, 1 to 60 characters,
unique ignoring case), stored in the database's one-row `profile` table. There is always at least one
profile; the database from before profiles became the one named "Personal". See
[ADR 0005](./adr/0005-profiles.md).

## 7. Core flows

### 7.1 Entry processing pipeline

1. **Write.** The user submits an entry, optionally scoped to a project (default: "auto").
2. **Persist.** The entry is stored immediately and the UI returns. Processing is asynchronous.
3. **Snapshot.** Build a compact state snapshot: active projects, plus the entry's project whatever its status (even archived, so reading an old entry again still works); their open milestones (id, title, in order); their open tasks (id, title, status, size, milestone if it is open, and whether it has stalled: in progress with neither it nor any of its steps changed for `stalled_after_days` in `config.toml`, default 14, 0 for never); open blockers; parked ideas (titles only); and the observation summaries of the last N entries written before this one, from the entry's project or, for an unscoped entry, from any project. N is `snapshot_recent_entries` in `config.toml` (default 5; 0 for none).
4. **Extract.** Call the configured `Extractor` with (entry, snapshot). It returns a validated `ExtractionResult`:
   - `observations[]`: kind, summary, text span, links[] with confidence. A link names a row in the snapshot, or `"$new:N"` for the entity the op at position N creates (that op must create that kind of entity)
   - `ops[]`: proposed change ops with rationale and confidence
   - `questions[]`: at most 2 follow-up questions, each with a reason
5. **Record.** Store the extraction run (including raw output: every attempt's reply, result, problems, and latency, so a failed or retried run keeps what the model said each time; the run's `latency_ms` is the total), observations, the changeset, and questions, in one transaction. Links to `$new:N` are stored pending on the op that creates their target. A result with no ops gets no changeset, since an empty one could never be resolved. If the result fails validation, the extractor is asked once more, given the problems and its rejected result; an extractor error (a timeout, an unreachable model) is not retried. If extraction or validation still fails, the run is marked failed with its error and nothing else is stored. The error starts with its stage, from which the API gives a failed run a `failure` reason (`timeout`, `unreachable`, `invalid`, `error`) that the diary turns into one calm line next to **Read again**.
6. **Notify.** Push a server-sent event (`entry.processed`, plus `changeset.ready` when there is a changeset, or `run.failed`) to the UI.
7. **Review.** The user accepts, rejects, or edits each op, or accepts all, then applies. What was decided is applied inside a single DB transaction and the changeset is resolved. Ops left undecided, and ops of an extraction changeset held back by the active-project cap, move to a new pending changeset split off the original ([ADR 0003](./adr/0003-splitting-changesets.md)). An op can't be applied while an op it depends on (by temporary ID) is undecided or rejected. Applying a creating op turns the observation links pending on it into links to the new row; rejecting it, or rejecting a changeset on reprocessing, drops them.
8. **Refresh briefing.** Recompute the briefing for affected projects.

Extraction runs are the worker's queue: saving an entry creates it and a `pending` run in one transaction, and a background worker processes runs one at a time. A run left `running` by a stopped process goes back to `pending` at startup. **Reprocessing** an entry queues a new run; it is refused while the entry has a pending or running run. In the same transaction it rejects the entry's pending changesets (split-offs included) and dismisses its open questions, so the old and new proposals can't both be applied. The UI warns about this before reprocessing. Observations from earlier runs are kept.

**Deleting** an entry ([ADR 0007](./adr/0007-deleting-entries.md)) removes it whole, in one transaction, with its runs, observations (and their links and pending links), follow-up questions, and the changesets of its runs (pending, split off, or resolved) with their ops. It is refused while the entry's run is running; a pending run goes with it. Changes already applied from it stay, with their origin cleared (a task's, blocker's, or idea's `origin_entry_id`, a blocker's `resolved_entry_id`). A question it answered becomes dismissed; entries replying to its questions lose the reply link. The text of other entries is untouched. Briefings of active projects are then refreshed.

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
- **Where you left off:** up to 3 recent entries about the project, newest first, each with its date and a few short points, each point an observation kind (so it shows in that kind's highlight colour).
- **Open blockers.**
- **Low-energy alternatives:** up to 2 small or low-energy tasks.

Two implementations exist behind one interface:
- *Heuristic*, used in the prototype and as a fallback: prefer in-progress tasks, then unblocked todo tasks by `position`, preferring smaller sizes. Only tasks that can be started now count: no open blocker on the task, no dependency on an open task, and no open subtasks (the subtasks are the steps to start instead). The most recently updated in-progress task wins, in any milestone; otherwise the smallest of the first 3 such todo tasks by position (an unsized task counts as `m`; ties go to position), drawn from the first open milestone (in milestone order) that has a startable todo task, or from all of them if none does. A step counts in its top-level task's milestone. The recap is the 3 most recent entries about the project that have observations, each with up to 3 of its latest run's observation summaries, kinds kept.
- *LLM*: selects and explains using the snapshot. It must return the ID of a task that can be started now, with a short reason; otherwise the heuristic's next action is used. Its recap has the same shape: it is shown the recent entries (IDs, dates, observations) and returns short points per entry, each with an observation kind; entries it wasn't shown are dropped, and an invalid recap falls back to the heuristic's. The two parts fall back independently; the briefing's source is `llm` if either came from the model. With no task to start, the model isn't asked. It uses the configured extraction provider and model; with the mock provider, briefings are heuristic only.

A model can take seconds or more, so a refresh is two-phase: the heuristic briefing is written at once (an apply or the "Now" screen never waits on the model), then the project is queued and the model's briefing replaces it in the background, emitting `briefing.updated` again. A model briefing is discarded if the project was refreshed while the model was working; that refresh queued it again with the newer state.

Only the next action, its reason, and the recap are cached (the `briefings` table, and `briefing_recap_points` for the recap's points, each naming its entry; an entry's date is read from the entry). Open blockers, low-energy alternatives, and the entry excerpt (the text of the newest observation linked to the next task) are facts about current state, read fresh each time the briefing is shown. Active projects' briefings are regenerated after every applied changeset that touches the project (manual edits included) and after an entry about it is processed (every active project's, for an unscoped entry); each regeneration emits `briefing.updated`. Every applied changeset, manual or from an entry, also emits `changeset.applied` with the IDs of the projects whose own row, tasks, or blockers it changed, so screens of non-active projects and the parking lot can refresh. "Entries about a project" are those scoped to it plus unscoped ones with an observation linked to the project or one of its tasks or blockers.

### 7.4 Manual editing

The user can always edit tasks, blockers, ideas, and projects directly. Manual edits are recorded as a `manual` changeset so the audit trail stays complete.

## 8. Functional requirements

Requirements are tagged by the stage in which they're first needed: **[P]** Prototype, **[M]** MVP, **[L]** Later.

**Projects**
- [P] Create, view, edit, and change status of projects.
- [P] Enforce the active-project cap (a setting, default 3). Any project status can change to any other; the cap only stops a project from *becoming* active when the cap is reached. Projects already active stay active if the cap is lowered. A manually created project may start active (if there is room) or parked; a project proposed by extraction (`create_project`) is always parked (§6.2).
- [P] Archive a project (status `archived`) to put it out of sight: it leaves the Now screen, the Parking lot, the project pickers (diary scope, review, promote targets; still shown when it is the one already chosen), and the extraction snapshot. The Projects screen lists archived projects last, under "Archived", each with **Restore**, which makes it parked again.
- [P] The Projects screen stays tidy with many projects: a filter box narrows it by title, each status group lists the most recently active project first, and Done and Archived start collapsed (a filter opens any group with a match). A project's last activity (`last_activity_at` on every project in the API) is the latest of its own edits, changes to its tasks, blockers, and ideas, and its newest diary entry.
- [L] Milestones.

**Tasks**
- [P] CRUD, status changes, and ordering within a project. Deleting asks for confirmation.
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
- [P] Choose the extractor provider (mock / anthropic / openai-compatible), model, and API key or endpoint, via config file or env vars: `provider`, `model`, `endpoint` (the API base URL, e.g. `http://localhost:11434/v1`), and `api_key` in `config.toml`, or `WAYPOINT_PROVIDER`, `WAYPOINT_MODEL`, `WAYPOINT_ENDPOINT`, `WAYPOINT_API_KEY`; no flags, so a key stays out of shell history. The default is the mock. For `anthropic`, `endpoint` defaults to `https://api.anthropic.com/v1` and `model` to `claude-haiku-4-5`; `api_key` is required. A local server must give the model a context window that holds the prompt and tool schema (about 5k tokens before the snapshot); Ollama's default of 4096 cuts the prompt off, so set its context length to 16k or more. `timeout` in `config.toml` (or `WAYPOINT_TIMEOUT`), a duration such as `10m`, bounds each model request; the default is 5 minutes for `openai-compatible` and 2 for `anthropic`.
- [M] The same settings editable in the UI.

## 9. Non-functional requirements

- **Local-first:** runs entirely on the user's machine. Binds to `127.0.0.1` by default. Each profile's data lives in one SQLite file (§6.4). The only network traffic is to the configured LLM endpoint, and none at all when using the mock or a local model.
- **Portable:** a single binary per OS (linux/amd64, linux/arm64, darwin/arm64, darwin/amd64, windows/amd64) with the frontend embedded. No CGO. A Docker image is optional.
- **Responsive:** saving an entry returns in under 100 ms locally. Extraction runs in the background.
- **Resilient to AI failure:** invalid or failed extraction must never corrupt state. The entry is saved regardless; the run is marked failed and can be retried.
- **Privacy:** diary content is sensitive. Never log entry bodies at info level. Keep API keys out of the DB export and out of logs.
- **Portable data:** the user can export everything as JSON and back up a profile by copying its file.

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

Everything below the HTTP layer exists once per profile (§6.4): each profile has its own database,
store, worker, changeset applier, briefing service, and SSE hub, all started at launch by
`internal/profile`. The HTTP layer routes `/api/profiles/{profile}/...` to that profile's instance;
the extractor is shared.

### 10.1 Technology choices

| Concern | Choice | Notes |
|---|---|---|
| Language | Go (latest stable) | |
| HTTP router | `go-chi/chi` | Standard-library-compatible |
| Database | SQLite via `modernc.org/sqlite` | Pure Go, no CGO, so cross-compiling is trivial. WAL mode. |
| Queries | `sqlc` | Type-safe generated code from SQL |
| Migrations | `pressly/goose` with embedded SQL files | Run automatically at startup |
| Background work | In-process worker goroutine + channel, one per profile; jobs persisted as extraction runs with `status=pending` | Pending runs resume on restart, in every profile |
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
/internal/profile/        profile manager: one database and set of services per profile (ADR 0005)
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
    ReplyTo  *FollowUpQuestion // the question this entry answers, if any
    MaxQuestions int           // the questions-per-entry setting; the worker drops extras
    PromptVersion string
}

type ExtractionResult struct {
    Observations []ObservationDraft
    Ops          []ChangeOpDraft
    Questions    []QuestionDraft // len <= MaxQuestions
}

type Briefer interface {
    // State: the project, its tasks, dependencies, open blockers, and the
    // observation summaries of recent entries about it. Richer than the
    // extractor's StateSnapshot: it needs size, energy, and dependencies.
    Brief(ctx context.Context, st briefing.State) (Briefing, error)
}

// Caller is how code outside internal/extract/* uses the model without
// knowing the provider (the LLM Briefer). The model extractors implement it.
type Caller interface {
    Call(ctx context.Context, c ToolCall) (ToolReply, error) // one forced tool call
}
```

Every extractor's output passes through the **same validator** before storage. The validator checks that referenced IDs exist, enforces enum values and caps, and verifies text spans fall within the entry. "Exist" means *in the snapshot the extractor was given*: an extractor can only know the IDs it was shown, so a real ID outside the snapshot is most likely made up. The applier still re-checks refs against the DB at apply time. An observation link may point at a `$new:N` entity only if the op at position N creates that kind of entity; such a link is stored pending until that op is applied or rejected. Invalid output fails the whole result; it never becomes partial state.

Observation spans are **byte offsets** into the UTF-8 entry body and must fall on character boundaries. The browser indexes strings in UTF-16 units, so the API converts spans before sending them to the UI. LLM extractors don't ask the model for offsets: the tool schema (`extract.ResultSchema`) has each observation carry a verbatim `quote`, and a shared helper turns it into a span (the first match after the previous quote, else anywhere; exact, then ignoring whitespace and quote-mark differences; else an empty span).

### 10.4 Mock extractor

The mock is deterministic and rule-based, so the whole system can be built and tested without an AI model. Line prefixes map to observations and ops:

| Line prefix | Result |
|---|---|
| `done: <text>` | progress observation; `set_task_status` done on the best fuzzy title match, or `create_task` + done |
| `doing: <text>` | progress observation; `set_task_status` in_progress |
| `todo: <text>` | plan observation; `create_task` |
| `blocked: <text>` | blocker observation; `create_blocker` on the best-matching open task of the entry's project, else on the whole project |
| `unblocked: <text>` | `resolve_blocker` on the best match |
| `idea: <text>` | idea observation; `park_idea` |
| `feel: <text>` | feeling observation |
| `decided: <text>` | decision observation |
| anything else | note observation linked to the scoped project |

Questions: if the entry has no `todo:`/`doing:` lines, ask "What's the next step?" If a task has been in progress for more than 3 days with no blocker, ask about blockers. An entry that replies to a question (`reply_to_question_id`) never gets that same question again; the extractor sees the replied-to question in `ExtractInput.ReplyTo`. In a reply to "What's the next step?", every plain line is read as a `todo:` line, so answering proposes a task (or stays a note when there's no project to put it in). The mock may add artificial latency (configurable) to exercise the async UI.

New tasks, blockers, and ideas go into the entry's project. For an entry with no project, the mock guesses one: the only project in the snapshot, else the one with the most clues (words of its title in the entry, and lines naming its tasks or blockers), else the first project with low confidence. Ops on a guessed project carry a lower confidence and say so in their rationale; the review UI lets the user pick another project or none. `done:`, `doing:`, and `unblocked:` match across all projects; `blocked:` matches only the open tasks of the project the blocker goes into (including tasks the entry creates), and a matched `blocked:` line counts as a clue like `done:`. An observation naming a task links to that task and its project. Picking another project in review takes a blocker off an existing task of the guessed one. Observations aren't linked to a project guessed without any clue, since the picker moves ops, not observations. A task created from a `todo:`, `done:`, or `doing:` line is linked from that line's observation by temporary ID, even on a guessed project, since the task moves with its op. A prefixed line that yields nothing (no match, or no project to use) becomes a note.

### 10.5 API outline

Server-wide:

```
GET    /api/health
GET    /api/profiles                  POST   /api/profiles             body: { name, appearance_from? }
PATCH  /api/profiles/{profile}        body: { name }
DELETE /api/profiles/{profile}
```

Everything else belongs to one profile (§6.4) and sits under `/api/profiles/{profile}`, shortened to
`/api` below (so `/api/projects` means `/api/profiles/{profile}/projects`). The profile is in the path,
not a header, because `EventSource` can't send headers ([ADR 0005](./adr/0005-profiles.md)):

```
GET    /api/projects                  POST /api/projects
GET    /api/projects/{id}             PATCH /api/projects/{id}
DELETE /api/projects/{id}?entries=keep|delete   GET /api/projects/{id}/deletion (what deleting takes with it)
GET    /api/projects/{id}/tasks       POST /api/projects/{id}/tasks
PATCH  /api/tasks/{id}                GET  /api/tasks/{id} (incl. linked observations, open blockers + origin)
DELETE /api/tasks/{id}               POST /api/tasks/{id}/move   body: { direction: up|down }
GET    /api/projects/{id}/blockers    POST /api/projects/{id}/blockers
PATCH  /api/blockers/{id}             DELETE /api/blockers/{id}
GET    /api/projects/{id}/milestones  POST /api/projects/{id}/milestones
PATCH  /api/milestones/{id}           DELETE /api/milestones/{id}
POST   /api/milestones/{id}/move      body: { direction: up|down }
GET    /api/ideas?status=&project_id= PATCH /api/ideas/{id}      DELETE /api/ideas/{id}
POST   /api/ideas/{id}/promote        body: { to: task|project, project_id? }
GET    /api/entries?project_id=&before=&limit=   POST /api/entries
GET    /api/entries/{id}              (incl. observations, questions, changesets)
POST   /api/entries/{id}/reprocess   DELETE /api/entries/{id}
GET    /api/changesets?status=pending
POST   /api/changesets/{id}/apply     body: { ops: [{id, decision: accept|reject|edit, payload?}] }
POST   /api/questions/{id}/dismiss
GET    /api/projects/{id}/observations?before=&limit=   (what the diary says about it)
GET    /api/projects/{id}/briefing    POST /api/projects/{id}/briefing/refresh
GET    /api/settings                  PATCH /api/settings
GET    /api/export
GET    /api/events                    (SSE: entry.processed, changeset.ready, briefing.updated, run.failed, changeset.applied, entry.deleted)
```

Request and response bodies are the DTOs in `internal/api/apitypes`, with TypeScript types generated from them (ADR 0001). Notes:

- **Edits are manual changesets.** Every POST, PATCH, and DELETE on projects, milestones, tasks, blockers, and ideas becomes one `source=manual` changeset (§7.4). PATCH bodies follow the op payloads of §6.2 without the ID, which comes from the path: absent fields stay as they are, and `null` clears a nullable field. A task's `status` in a PATCH or create becomes a `set_task_status` op. Moving a task (`POST /api/tasks/{id}/move`) swaps its `position` with the neighbouring task in the same status group and milestone (ordered by position, then id) as `update_task` ops in one changeset, and answers with the tasks it moved; at the end of the group nothing moves and the list is empty. A milestone's PATCH is one `update_milestone` (any status: `open`, `done`, or `dropped`; `position` orders it in its project), and `DELETE /api/milestones/{id}` (204) leaves its tasks in the project with no milestone. Milestones list in order; `POST /api/milestones/{id}/move` swaps a milestone's `position` with its neighbour in the project as `update_milestone` ops in one changeset and answers with the milestones it moved, as the task move does. A task's `milestone_id` (on create and PATCH; `null` clears it) must name a milestone in the task's project. A blocker's `status` can only be set to `resolved` (`resolve_blocker`). An idea's status can't be set to `promoted` by PATCH: promoting creates the task or parked project and marks the idea promoted in one changeset. A project created with `status: active` or a definition of done gets an `update_project` after its `create_project`. `DELETE /api/projects/{id}` (204) needs `entries=keep` or `entries=delete` (otherwise `bad_request`); it is one `delete_project` op, and each entry deleted with the project is published as `entry.deleted`. `GET /api/projects/{id}/deletion` counts the project's tasks, blockers, ideas, milestones, and scoped entries for the confirmation.
- **Entries.** `POST /api/entries` answers 202 once the entry and its pending run are stored; SSE reports the outcome. An entry with `reply_to_question_id` marks that question answered in the same transaction; replying to a question that is no longer open is rejected. `GET /api/entries?project_id=` lists the entries *about* the project: those scoped to it and unscoped ones with an observation linked to it or to its tasks or blockers. Pages are newest first; `next_before` in the response is the `before` for the next page. `DELETE /api/entries/{id}` (204) deletes the entry as §7.1 describes and publishes `entry.deleted`; while its run is running it is `run_in_progress`. `GET /api/projects/{id}/observations` lists the latest-run observations linked to the project or to its tasks or blockers, each once, newest entry first and in text order within an entry; its pages hold whole entries (`limit` counts entries) and continue the same way.
- **Changesets.** Each op carries `refs`, the names of the existing rows its payload refers to, keyed `kind:id` (e.g. `task:7`), so the review UI can say what the op touches.
- **Spans.** Observation spans in API responses are UTF-16 code unit offsets, converted from the stored UTF-8 byte offsets (§10.3), so the UI can slice the entry body directly.
- **Settings.** User preferences live in each profile's `settings` table (one row): `active_project_cap` (at least 1, default 3), `questions_per_entry` (0 to 2, default 2), and the appearance: `theme` (`midnight`, the default, or `forest`, both dark; `paper`, `sage`, or `dawn`, light), `interface_font` (`inter`, `geist`, `manrope`, `ibm-plex-sans`, or `atkinson-hyperlegible-next`; default `inter`) and `diary_font` (`literata`, a serif and the default, or any interface font). `GET /api/settings` returns them; `PATCH /api/settings` changes the fields present (none may be null; an empty body, unknown field, or out-of-range value is `invalid`) and answers with all of them. Settings aren't projects, tasks, blockers, or ideas, so they are written directly, not as changesets. The applier reads the cap inside each apply transaction, so a new cap applies to the next activation; lowering it deactivates nothing. The worker stores only the first `questions_per_entry` questions of a run (the validator still fails a result with more than 2; the raw output keeps them all). `config.toml` keeps technical settings (port, data dir, log level, mock latency, snapshot size, stalled threshold); its `active_project_cap` and `questions_per_entry` only seed the settings of a profile that has none (at startup, and when a profile is created) and never overwrite stored ones. `POST /api/profiles` takes an optional `appearance_from` (a profile ID; the app sends the one in use): the new profile starts with that profile's theme and fonts instead of the defaults, and an unknown ID is `invalid`. `GET /api/export` comes later.
- **Profiles.** `GET /api/profiles` lists `{id, name}` in creation order. `POST` creates a profile (201) with its settings seeded from `config.toml`; `PATCH` renames it (the `id` never changes). A blank, over-long, or already-used name (ignoring case) is `invalid`. `DELETE` (204) stops the profile and removes its database file with its `-wal` and `-shm`; its event streams end. The last profile can't be deleted (`last_profile`). An unknown `{profile}` in any path is `not_found`. `/api/health` checks every profile's database. On first start the database from before profiles (`<data_dir>/waypoint.db`) becomes the profile "Personal"; with no profiles at all, an empty "Personal" is created.
- **Errors** use one shape: `{"error": {"code", "message", "problems?"}}`. Codes: `bad_request` (400: malformed JSON or query), `not_found` (404), `method_not_allowed` (405, with `Allow`), `invalid` (422: validation failures and unknown fields, with per-field `problems`), `active_cap` (409), `last_profile` (409: deleting the only profile), `not_pending` (409: the changeset was already resolved), `run_in_progress` (409: the entry is being processed), `internal` (500).

## 11. UI surfaces

1. **Now** (home): a briefing per active project, each a clearly separated block. The next action comes first and stands out (a step names its parent: "Step 2 of *Set up CI*", numbered as in the parent's checklist); then "Where you left off", a short list of recent entries, newest first, each with a relative date ("today", "yesterday", "3 days ago", then the date, linking to the entry in the diary) and its points as separate items in their kinds' highlight colours; then open blockers and low-energy alternatives, kept brief. This is the screen the user lives on.
2. **Diary**: the diary input above a timeline of entries with observation highlights. Clicking a highlight opens the linked task or blocker in the main pane. Pending changesets appear inline beneath the entry that produced them. Each entry's **More** menu offers reading it again and deleting it; deleting asks first, saying that its highlights, questions, and unreviewed changes go with it and that applied changes stay. The diary is not a separate screen but a collapsible, resizable panel on the right, available beside every other screen; its width and open state persist. `/diary` shows the same content full-page, for long reviews and links; `/diary?entry=N` loads the timeline back as far as that entry, scrolls to it, and briefly highlights it (a calm line says so when the entry isn't there). Other screens link to an entry this way. On narrow windows the panel is replaced by a floating button in the bottom-right corner that opens the diary as an overlay (ADR 0004).
3. **Project**: header (why, definition of done), the project's next action from its briefing (task, reason, link to the task; a calm line when there isn't one), task list grouped by milestone (in order, then "No milestone"; a done or dropped milestone starts folded) and within each by status, with each task's steps (subtasks) nested under it; milestones are added, edited, reordered, marked done or dropped, and deleted there, and a task's milestone is picked in its edit form, blockers, ideas for this project, and "From the diary": the observations about the project (linked to it or to its tasks or blockers), newest entry first, grouped by entry with each kind's highlight colour, each entry's date linking to it in the diary, a page of entries at a time.
4. **Task detail**: fields, its steps as a checklist (add a step, tick it done or back to todo, reorder, delete), origin entry, and a chronological list of every linked observation ("everything I've thought about this"), grouped by entry; the origin entry and each group's date link to the entry in the diary.
5. **Parking lot**: ideas and parked projects.
6. **Settings**: the caps (active projects at once, follow-up questions per entry) of the profile in use; its appearance, a theme (shown as swatches in each theme's own colours), an interface font, and a diary font, each applied to the whole app and saved as soon as it is chosen (no Save button; a failed save puts the previous choice back and says so in one line); then the profiles: add one (it doesn't become the one in use, and starts with the appearance of the one that is), rename, switch to, and delete. Deleting asks the user to type the profile's name and says plainly that everything in it goes and can't be undone; deleting the profile in use switches to another first, and the only profile can't be deleted. Later: the provider, model, and keys. Lowering the active-project cap below the number of active projects pauses nothing, and the screen says so.
7. **Profile switcher**: a select in the header beside Settings. Switching starts every screen, the diary, and the live-update stream over in the chosen profile, with nothing from the old one; a project or task page goes to Now, other pages stay. The browser remembers the profile in use (`localStorage`, checked against the list at startup, falling back to the first). An unsaved diary draft belongs to its profile and comes back on switching back, for as long as the page is open.

UX notes: keyboard-first diary input (Ctrl/Cmd+Enter to submit); calm visual design (restrained gradients and glows; glow draws attention, never alarm) in one of five themes: Midnight (dark, purple accent; the default), Forest (dark, deep green with a teal accent), Paper (light, warm off-white with an indigo accent), Sage (light, soft green), and Dawn (light, cream with an amber accent). Every theme meets WCAG AA for text, and no accent is near its theme's danger red. The browser remembers the theme it last showed, so a page load doesn't flash another theme; fonts are bundled with the app (`@fontsource`), never loaded from a CDN, and the diary font applies wherever diary text is written or read (the diary input, entry cards, follow-up answers, and entry text on Task detail); no red "overdue" states; empty states that encourage starting small; text areas grow with their content to a maximum height, then scroll; task notes, a project's why and definition of done, and an idea's body are rich text, edited word-processor style (headings, bold and italic, bulleted, numbered, and check lists, links) and stored as markdown ([ADR 0009](./adr/0009-rich-text-editor.md)), while the diary input stays plain text; the server's status appears only as a calm banner when it can't be reached (or can't reach its database), never as a permanent indicator. Buttons come in four variants: primary (the one main action), secondary (outlined alternatives such as Edit), quiet (subtle, for cancel and small in-line actions), and danger; red marks destructive buttons only (delete, remove), never state.

## 12. Quality and testing

- **Unit tests:** validator, changeset applier (including temp IDs and rollback on failure), heuristic briefing, mock extractor.
- **Integration tests:** an HTTP API test harness against a temp SQLite DB, using the mock extractor end to end.
- **Extraction evals ([M]):** `/testdata/entries/*.md` paired with expected observations and ops. Run a scoring command (`waypoint eval`) against any provider to catch prompt regressions. `waypoint eval -provider X [-model M] [-v]` scores ops (type, rows, status) and links (target rows); other payload fields and free text aren't compared. It counts forbidden ops and runs that fail validation.
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
