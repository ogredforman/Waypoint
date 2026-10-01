# Code map

A short orientation to the code, so a task can start without reading whole packages. Read this
first, then open only the files you'll change or call. Keep it current: when a task adds a
package, a public entry point, or a test fixture, update the matching line here in the same commit.
Signatures here are summaries; `go doc ./internal/<pkg>` or `grep -n '^func' <file>` gives the exact ones.

## Reading the docs cheaply

- SPEC sections by heading: `awk '/^### 6.2/,/^### 6.3/' docs/SPEC.md` (or `/^## 7\./,/^## 8\./` for a whole section).
  §5 principles, §6.1 entities, §6.2 change ops, §7.1 pipeline, §7.2 questions, §7.3 briefing,
  §7.4 manual edits, §8 functional reqs, §10.2 layout, §10.3 interfaces, §10.4 mock extractor,
  §10.5 API, §11 UI surfaces, §12 testing.
- PLAN: `grep -n '\[ \]' docs/PLAN.md | head` for the next tasks; read only the current stage's section.
- ADRs: 0001 API types (Go DTOs → generated TS), 0002 deleting tasks/blockers/ideas,
  0003 splitting changesets.

## Wiring (`cmd/waypoint/main.go`)

`config.Load` → `db.Open` + `db.Migrate` → `api.NewHub` → `store.New(sqlDB, time.Now)` →
`pipeline.NewSnapshotBuilder` → `briefing.NewService(st, briefing.Heuristic{}, …, hub, logger)` →
`changeset.NewApplier(st, cfg.ActiveProjectCap, briefings)` → `mock.New(cfg.MockLatency)` →
`pipeline.NewWorker(st, snapshots, extractor, hub, briefings, logger)` (goroutine `worker.Run`) →
`api.NewRouter(api.Deps{…})` → `api.Serve`. All dependencies are built here; no package globals.

## Go packages (`internal/`)

- **domain**: entity structs (`Project`, `Task`, `Blocker`, `Idea`, `Entry`, `Observation`,
  `ObservationLink`, `Changeset`, `ChangeOp`, `ExtractionRun`, `FollowUpQuestion`, `Briefing`,
  `Milestone`) in `types.go`; string enums with `Xxxs()` lists in `enums.go` (must match SQL CHECKs;
  `db/enums_test.go` enforces it); validation, `ValidationError`/`FieldError`, project status rules
  (`CheckNewProjectStatus`, `CheckProjectStatusChange`, `ActiveCapError`) in `validate.go`.
- **db**: `Open` (modernc sqlite, WAL, FKs on), `Migrations`, `Migrate` (goose, embedded).
  `migrations/0000N_*.sql` (never edit committed ones), `queries/<entity>.sql` → sqlc output in
  `dbgen/` (generated; `make sqlc`).
- **store**: the only code that runs SQL. `Store.InTx(ctx, func(*Tx) error)` for writes,
  `Store.InReadTx(ctx, func(*ReadTx) error)` for reads. Reads (`GetX`, `ListXByProject`,
  `ListRecentEntries…`, `ListChangesetsByStatus`, `CountActiveProjects`, …) are on the embedded
  `reader`, so both tx types have them; writes (`CreateX`, `UpdateX`, `DeleteX`, `CreateChangeset`,
  `ResolveChangeset`, `SetChangeOpResult`, `MoveChangeOp`, `ClaimNextExtractionRun`,
  `FinishExtractionRun`, `UpsertBriefing`, question answer/dismiss) are on `*Tx`. One file per entity.
  `ErrNotFound` for missing rows. Entries are insert-only.
- **changeset**: every mutation of projects/tasks/blockers/ideas. `ops.go`: one payload struct per
  op type (`CreateTask`, `UpdateTask`, `SplitTask`, `DeleteTask`, `ParkIdea`, …) implementing
  `Payload`; `DecodePayload`, `ValidateOps`; `Nullable[T]` (`To`, `Clear`) for PATCH-style fields.
  `ref.go`: `Ref` (`ID(n)` or `Temp(pos)` for `$new:N`), `CheckRefs`. `validate.go`: cross-row
  rules (same project, no parent cycles). `apply.go`: `Applier.Apply(ctx, changesetID,
  map[opID]Decision)` → `Result{Changeset, Created, SplitOff, HeldByCap, Projects}`; one
  transaction, full rollback, undecided/capped ops split into a new pending changeset (ADR 0003);
  calls `AfterApply.Applied`. `manual.go`: `Applier.ApplyManual(ctx, ops ...Payload)` for
  `source=manual`. `reject.go`: `RejectPending`. `AfterApplies` chains hooks (briefings, then the SSE hub).
- **extract**: provider-neutral boundary. `Extractor{Extract, Name}`, `ExtractInput` (entry, snapshot, `ReplyTo` question),
  `ExtractionResult` (observations, links, `ChangeOpDraft`s, `QuestionDraft`s), `StateSnapshot`
  (`Has`), `Validate` (shared validator, `MaxQuestions = 2`).
  **extract/mock**: deterministic line-prefix extractor (SPEC §10.4), `mock.New(latency)`.
- **pipeline**: `SnapshotBuilder.Build(ctx, entry)`; `Worker`: `SaveEntry` (entry + pending run in
  one tx), `Reprocess`, `Wake`, `Run(ctx)` (claims runs, extracts, validates, stores result +
  changeset, emits `Event`s via `Notifier`, calls `AfterRun.EntryProcessed`). Failures recorded on
  the run, never partial state.
- **briefing**: `Briefer` interface, `Heuristic` implementation (`heuristic.go`), `Service`
  (`Refresh`, `Get` → `View`; implements `changeset.AfterApply` and `pipeline.AfterRun`; notifies
  `BriefingUpdated`). Briefings are cached per project in the DB.
- **api**: chi router (`router.go`, route list in `handlers.routes`), one file per resource
  (`projects.go`, `tasks.go`, `blockers.go`, `ideas.go`, `entries.go`, `changesets.go`),
  `convert.go` (domain → apitypes), `respond.go` (JSON encode/decode, `h.fail` maps errors to the
  single error shape, `badRequest`), `events.go` (`Hub`: SSE `/api/events`, `Publish`, `Notify`,
  `BriefingUpdated`, `Applied` → `changeset.applied`), `serve.go`. Handlers are thin: decode, call a service, encode.
  **api/apitypes**: request/response/event DTOs; `gen/` generates `web/src/lib/api/*.gen.ts`
  (`make gen`, checked by `make lint`).
- **config**: `Load(args, env, goos, stderr)` (flags > env > `config.toml`), `EnsureDataDir`.
- **web**: embeds `web/dist`, `NewHandler` serves the SPA with fallback.
- `tools/hooks`: Claude Code hooks (edit guard, formatter, stop check). Not app code.

## Frontend (`web/src`)

Svelte 5 runes + TS + Tailwind, Vitest. `main.ts` → `App.svelte`. `lib/api/client.ts` is the
typed API client (the only place that calls `fetch`), with generated `types.gen.ts`/`enums.gen.ts`.
`lib/router.ts` + `lib/navigation.svelte.ts`: client-side routing; every screen's `<h1>` takes `{@attach pageHeading}` (focus after navigation). `screens/`: page components
(`Now`, `Diary`, `Projects`, `Project`, `Task`, `ParkingLot`, `NotFound`). `components/`: reusable
pieces. Tests sit next to the code as `*.test.ts`.

- Layout (ADR 0004): `App.svelte` mounts `DiaryPanel` (lg and up) or `DiaryOverlay` (below lg), never
  both, and neither on `/diary`. Both wrap `DiaryContent`, which `screens/Diary` also shows full-page.
  `lib/diaryPanel.ts`: panel width limits, drag/key resizing, `localStorage` prefs.
- Live updates: `lib/events.ts` is the app's one `EventSource` (`startEvents` in `App.svelte`; `on(name, handler)`,
  `onReconnect`). Screens subscribe in `onMount` and refetch; `lib/events.fake.ts` `FakeEventSource` for tests.
  `lib/live.ts`: `liveReload` (reload on `changeset.applied`, optionally for one project, and on reconnect) and
  `latest` (drop stale answers); used by the Project, Projects, Parking lot, and Task screens. `screens/Project.test.ts`.
- Diary input: `components/DiaryInput.svelte` (in `DiaryContent`); `lib/diary.ts` (submit shortcut,
  default project scope from the route, `entryRequest`); `lib/diaryDraft.svelte.ts` (shared draft that
  outlives the panel/overlay).
- Diary timeline: `components/EntryTimeline.svelte` → `EntryCard.svelte` (highlights, run state) →
  `FollowUpQuestion.svelte` (answer/dismiss). `lib/timeline.svelte.ts`: shared timeline state (`loadTimeline`,
  `loadOlder`, `loadEntry`, `addEntry`, `reprocessEntry`, `followEvents`). `lib/entries.ts`: `segments`, `observationHref`, detail helpers,
  reading again (`canReprocess`, `reprocessImpact`, `reprocessWarning`). `components/EntryCard.test.ts`.
- Changeset review: `EntryCard` → `components/ChangesetReview.svelte` (one per pending changeset; project
  picker, Apply / Accept all) → `ChangeOpRow.svelte` (accept/reject/edit). `lib/review.ts`: pure logic
  (`reject`/`accept` cascades over `$new:N`, `guessedProject`, `noProject`, `applyRequest`, `describe` using
  the op's `refs`, `editFields`).
- Now: `screens/Now.svelte` loads active projects' briefings into `components/BriefingCard.svelte`.
  `lib/projects.ts`: grouping, `activeProjects`, cap helpers.
- Project: `screens/Project.svelte` → `components/ProjectHeader.svelte` (view/edit title, why, definition of done)
  and `ProjectStatus.svelte` (status buttons, calm cap handling). `lib/projects.ts`: `projectPatch` (changed fields
  only), `statusOptions`, `activateCapMessage`. `lib/api/client.ts` also has `patchJSON`/`deleteJSON` (204 → undefined).
  Tasks: `components/ProjectTasks.svelte` (list grouped by status, add form, move up/down) → `TaskRow.svelte` (view,
  inline edit, status, delete with confirmation); move up/down is one `POST /api/tasks/{id}/move` (`api/tasks.go`
  `moveTask`, `movePositions`). `lib/tasks.ts`: `groupTasks`, `taskPatch`, `newTaskRequest`.
  Blockers: `components/ProjectBlockers.svelte` (open, then resolved) → `BlockerRow.svelte` (resolve, delete with confirmation);
  `ProjectBlockers` also has the add form with an optional open-task picker; `lib/blockers.ts`: `splitBlockers`, `openTasks`, `newBlockerRequest`. Ideas: `components/ProjectIdeas.svelte` (this project's parked ideas) → `IdeaRow.svelte`.
- Task detail: `screens/Task.svelte` (`GET /api/tasks/{id}`: fields, origin entry, linked observations grouped by entry
  via `lib/tasks.ts` `observationTimeline`; read-only, edits live on the Project screen). `lib/entries.ts` `kindLabel`.
- Parking lot: `screens/ParkingLot.svelte` (all parked ideas and parked projects) → `components/IdeaRow.svelte` (promote to a
  task in a chosen project or to a parked project, delete with confirmation; also used by `ProjectIdeas`) and
  `ParkedProjectRow.svelte` (activate, calm cap handling). `lib/ideas.ts`: `promoteTargets`, `defaultTarget`, `projectName`;
  `lib/projects.ts` `parkedProjects`.
- Theme: named tokens in `app.css` `@theme` (`canvas`/`surface`/`raised`/`line`, `ink`/`ink-muted`/
  `ink-faint`, `accent`/`accent-strong`/`accent-deep`/`on-accent`, `shadow-glow`/`focus`/`edge`,
  `text-title`, `px-gutter`, `rounded-card`). Use these, not raw Tailwind colours.
  `lib/theme.test.ts` checks AA contrast for the text pairs.
- `components/ui/`: `Button` (`primary`/`secondary`/`quiet`), `Input`, `Textarea` (`bind:value`),
  `Card` (`glow` for the one thing to notice), `Panel` (row container). `npm --prefix web test`, `npm --prefix web run check`.
- Component tests: `// @vitest-environment jsdom` at the top, then Svelte's `mount`/`flushSync` with a stubbed
  `fetch`; `components/ChangesetReview.test.ts` is the pattern. Svelte delegates events, so dispatch them
  with `bubbles: true`.
- E2E: `web/e2e/smoke.spec.ts` (Playwright, `make e2e`) drives the real binary with the mock extractor;
  `web/playwright.config.ts` starts it on port 7429 with a temp data dir and an empty config. Select by role and label.

## Test fixtures (reuse these; don't write new setup)

All use a real temp SQLite DB migrated with `db.Migrations()`.

- `store/store_test.go`: `newTestStore(t) (*Store, *sql.DB, *clock)`, `inTx`, `exec`, `count`,
  `seedProject`, `seedTask`, `seedEntry`, `seedRun`.
- `changeset/apply_test.go`: `newFixture(t) fixture`, `propose(t, f, runID, ops...)`, `newRun`,
  `opStatuses`, `taskCount`, `dbSnapshot` (rollback checks). `validate_test.go`: `fields(t, err)`.
- `pipeline/snapshot_test.go`: `newFixture(t)` with `f.project/task/blocker/idea/entry/observe`.
  `worker_test.go`: `f.worker(extractor, notifier)`, `f.save`, `f.run`, `f.stored`, `processNext`,
  `fakeExtractor`, `events` notifier.
- `briefing/service_test.go`: `newFixture(t)` with `f.manual(ops...)`, `f.project`, `f.task`,
  `f.entry`, `f.briefing`, `fixedBriefer`.
- `api/routes_test.go`: `newServer(t)`; `api/integration_test.go`: `startApp(t, dbPath, extractor)`
  (full wiring over HTTP + SSE); `api/events_test.go`: `newEventsServer`, `openStream`.
- `db/schema_test.go`: `migrated(t)`, `seed`, `noFKViolations`.
- `extract/mock/mock_test.go`: `run(t, snap, project, body)`.
