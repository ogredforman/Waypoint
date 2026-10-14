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
  0003 splitting changesets, 0004 diary panel layout, 0005 profiles, 0006 archived projects, 0007 deleting entries, 0008 deleting projects, 0009 rich text editor.

## Wiring (`cmd/waypoint/main.go`)

`config.Load` → `profile.Open(ctx, profile.Options{Dir: cfg.ProfilesDir(), LegacyDB: cfg.LegacyDBPath(),
Seed: cfg.SettingsDefaults(), Extractor: provider.New(…), …})` → `api.NewRouter(api.Deps{Logger, Web, Profiles})` →
`api.Serve(…, profiles.EndStreams)` → `profiles.Close()`. Per profile, `profile.Manager.start` builds (ADR 0005):
`db.Open` + `db.Migrate` → `store.New` → `st.SeedSettings` (only if missing) → `api.NewHub` →
`pipeline.NewSnapshotBuilder` → `briefing.NewService(st, …, hub, logger)` →
`changeset.NewApplier(st, AfterApplies{briefings, hub})` → `pipeline.NewWorker(…)` (goroutine `worker.Run`) →
`api.NewProfileHandler(api.ProfileDeps{…})`. All dependencies are built there; no package globals.

## Go packages (`internal/`)

- **domain**: entity structs (`Project`, `Task`, `Blocker`, `Idea`, `Entry`, `Observation`,
  `ObservationLink`, `Changeset`, `ChangeOp`, `ExtractionRun`, `FollowUpQuestion`, `Briefing`,
  `Milestone`) in `types.go`; string enums with `Xxxs()` lists in `enums.go` (must match SQL CHECKs;
  `db/enums_test.go` enforces it); validation, `ValidationError`/`FieldError`, project status rules
  (`CheckNewProjectStatus`, `CheckProjectStatusChange`, `ActiveCapError`) in `validate.go`.
  `settings.go`: `Settings` (user preferences per database, incl. `Theme`/`InterfaceFont`/`DiaryFont`), `DefaultSettings`, `CopyAppearance`, `MaxQuestionsPerEntry`.
  `profile.go`: `Profile` (ID, Name), `CleanProfileName`, `ProfileSlug`, `ValidProfileID`, `ErrLastProfile`.
- **db**: `Open` (modernc sqlite, WAL, FKs on), `Migrations`, `Migrate` (goose, embedded).
  `migrations/0000N_*.sql` (never edit committed ones), `queries/<entity>.sql` → sqlc output in
  `dbgen/` (generated; `make sqlc`).
- **store**: the only code that runs SQL. `Store.InTx(ctx, func(*Tx) error)` for writes,
  `Store.InReadTx(ctx, func(*ReadTx) error)` for reads. Reads (`GetX`, `ListXByProject`,
  `ListRecentEntries…`, `ListProjectObservationsBefore` (→ `ProjectObservation`), `ListChangesetsByStatus`, `CountActiveProjects`, …) are on the embedded
  `reader`, so both tx types have them; writes (`CreateX`, `UpdateX`, `DeleteX`, `CreateChangeset`,
  `ResolveChangeset`, `SetChangeOpResult`, `MoveChangeOp`, `ClaimNextExtractionRun`,
  `FinishExtractionRun`, `UpsertBriefing` (replaces the recap points too), question answer/dismiss) are on `*Tx`. One file per entity.
  `ErrNotFound` for missing rows. Entries are never updated; `Tx.DeleteEntry` removes one whole with what came from it (ADR 0007).
  `Tx.DeleteProject` removes a project with everything in it and unscopes its entries (ADR 0008); `CountProjectContents` (→ `ProjectContents`),
  `ListProjectEntryIDs`, `CountRunningProjectRuns`, `ProjectActivity` (→ map of last activity by project) / `GetProjectActivity`. `ErrRunInProgress` lives here (`pipeline.ErrRunInProgress` is the same value). Settings (`settings.go`): `GetSettings`
  (defaults if no row), `Store.SeedSettings` (insert if missing), `Tx.UpdateSettings`. Pending observation links (to `$new:N`,
  keyed by change op): `CreatePendingLink`, `ListPendingLinksByOp`, `ResolvePendingLinks`, `DropPendingLinks`.
  Profile row (`profile.go`): `GetProfile` (name, created), `Tx.SetProfileName`.
  Export (`export.go`): `ExportTables()` (every table, parents first; a test checks it against the schema), `ExportTable` (rows as stored), `SchemaVersion`.
  Milestones (`milestones.go`): `GetMilestone`, `ListMilestonesByProject`, `Tx.CreateMilestone` (appends), `UpdateMilestone`, `DeleteMilestone` (tasks keep no milestone, links removed).
- **changeset**: every mutation of projects/tasks/blockers/ideas. `ops.go`: one payload struct per
  op type (`CreateTask`, `UpdateTask`, `SplitTask`, `DeleteTask`, `ParkIdea`, `CreateMilestone`, …) implementing
  `Payload`; `DecodePayload`, `ValidateOps`; `Nullable[T]` (`To`, `Clear`) for PATCH-style fields.
  `ref.go`: `Ref` (`ID(n)` or `Temp(pos)` for `$new:N`), `CheckRefs`. `validate.go`: cross-row
  rules (same project, no parent cycles). `apply.go`: `Applier.Apply(ctx, changesetID,
  map[opID]Decision)` → `Result{Changeset, Created, SplitOff, HeldByCap, Projects, DeletedEntries}`; one
  transaction, full rollback, active-project cap read from the settings in the apply tx, undecided/capped ops split into a new pending changeset (ADR 0003);
  resolves pending observation links on applied creating ops, drops them on rejected ones;
  calls `AfterApply.Applied`. `manual.go`: `Applier.ApplyManual(ctx, ops ...Payload)` for
  `source=manual`. `reject.go`: `RejectPending`. `AfterApplies` chains hooks (briefings, then the SSE hub).
- **extract**: provider-neutral boundary. `Extractor{Extract, Name}`, `ExtractInput` (entry, snapshot, `ReplyTo` question, `Retry` (rejected result + problems, added to `UserMessage`)),
  `ExtractionResult` (observations, links, `ChangeOpDraft`s, `QuestionDraft`s, `Response`: the model's reply, not encoded), `ResponseError`/`ResponseOf` (an extractor error carrying the reply), `StateSnapshot`
  (projects with open `SnapshotMilestone`s, tasks (with `Size` and `Stalled`), blockers; recent entries with their `SnapshotQuestion`s; `Has`), `Validate` (shared validator, `MaxQuestions = 2`). `LinkDraft.TargetID` is a `changeset.Ref`,
  so a link can name `$new:N`; the worker stores those pending.
  `prompt.go` + embedded `prompt_system.md`: `PromptVersion` (bump on any prompt or format change), `SystemPrompt(maxQuestions)`,
  `UserMessage(ExtractInput)` (snapshot as text, rows as "task 12"); `ExtractInput.MaxQuestions` is the questions-per-entry setting.
  An extractor with a `PromptVersion()` method gets it recorded on its runs (`pipeline` `prompted`); one with `Model()`, the model (`modeled`).
  `schema.go` + embedded `schema.json`: `ToolName`, `ToolDescription`, `ResultSchema()` (the model's tool input; ops extraction
  may propose, observations with a `quote`; `schema_test.go` keeps it in step with the domain enums and payload structs).
  `quote.go`: `ResolveQuotes(body, observations)` sets spans from `ObservationDraft.Quote` (exact, then whitespace- and quote-normalised).
  `toolcall.go`: `Caller{Call(ctx, ToolCall{System, User, Tool, Description, Schema}) (ToolReply{Input, Response}, error)}`, the provider-neutral
  model call (both model extractors implement it; the mock doesn't); `ExtractWith(ctx, caller, in)` is their `Extract`.
  **extract/mock**: deterministic line-prefix extractor (SPEC §10.4), `mock.New(latency)`.
  **extract/openaicompat**: `New(Options{Endpoint, Model, APIKey, HTTPClient})` for any OpenAI-compatible `/chat/completions` (Ollama, LM Studio,
  llama.cpp, OpenAI): forced `propose_changes` tool call (or a JSON object in the content), then `ResolveQuotes`; `DefaultTimeout`.
  **extract/anthropic**: `New(Options{Endpoint, Model, APIKey, HTTPClient})` for the Messages API (defaults `https://api.anthropic.com/v1`, `claude-haiku-4-5`; key required):
  `propose_changes` tool call, forced with temperature 0 for Haiku/3.x/Sonnet 4 (`pinsOutput`), `auto` otherwise; system prompt cached; then `ResolveQuotes`.
  **extract/llmhttp**: what the model extractors share: `Post` (JSON POST, read cap, server error message cut short), `NumericIDs` (quoted `_id`s to numbers).
  **extract/provider**: `New(Options{Provider, Model, Endpoint, APIKey, Timeout, MockLatency})` picks the extractor (empty provider = mock; `Timeout` > 0 replaces the provider's `DefaultTimeout`). `switch.go`: `NewSwitch(opts, fromEnv, save SaveFunc, logger)` is the extractor `main` gives every profile and serves `/api/provider` (ADR 0010): `Settings`, `Update` (checks, saves, swaps), `Names`; its `Call` returns `extract.ErrNoModel` while the mock is in use.
- **pipeline**: `SnapshotBuilder.Build(ctx, entry)` (`WithStalledAfter(d)` marks in-progress tasks unchanged, steps included, for `d`; config `stalled_after_days`; recent entries carry their questions and status); `Worker`: `SaveEntry` (entry + pending run in
  one tx), `Reprocess`, `DeleteEntry` (refused while running), `Wake`, `Run(ctx)` (claims runs, extracts, validates, stores result +
  changeset (keeping only `questions_per_entry` questions), emits `Event`s via `Notifier`, calls `AfterRun.EntryProcessed`). An invalid result is retried once with `extract.Retry`; the run's raw output is `rawOutput(attempts)`, `{"attempts":[{response, result, problems, latency_ms}]}`, one per extractor call; failures recorded on
  the run, never partial state, with the stage first in the error (`timeout`, `unreachable`, `validate (retried)`, …); `FailureOf(msg)` → `FailureReason` for the API's `ExtractionRun.failure`.
- **briefing**: `Briefer` interface (`State` includes the project's milestones), `Heuristic` implementation (`heuristic.go`; todo window prefers the first open milestone), `Service`
  (`Refresh`, `Get` → `View` (with `Step`: the next task's parent and step number, → `Briefing.next_step`), `EntryDeleted`; drops recap entries the Briefer wasn't shown; implements `changeset.AfterApply` and `pipeline.AfterRun`; notifies
  `BriefingUpdated`). Briefings are cached per project in the DB; the recap (`[]domain.RecapEntry`, newest first, points with observation kinds) in `briefing_recap_points`.
  `llm.go` + embedded `llm_prompt.md`/`llm_schema.json`: `NewLLM(caller, now, logger)`, the model Briefer (`write_briefing` tool; next action and recap
  each checked, falling back to the heuristic's). `Service.WithModel(m)` + `Run(ctx)`: refresh writes the heuristic at once and queues the project;
  `Run` asks the model in the background and saves unless the project was refreshed meanwhile (`gen`). `profile` wires it when the extractor is a `Caller`.
- **eval**: extraction evals (SPEC §12). `LoadFixtures(dir)` → `[]Fixture{Name, Notes, Input extract.ExtractInput, Expected extract.ExtractionResult, Forbid []domain.OpType}`;
  refuses a fixture whose quotes aren't in the body, whose expected result `extract.Validate` rejects, or whose expected ops use a forbidden type.
  `Run(ctx, extractor, fixtures, done)` → `[]Outcome` (extract, validate, one retry as the pipeline does; `Status` ok/retried/invalid/error); `ScoreResult(f, got)` → `Score`
  (ops matched by type + payload refs + status, temp refs as `new`; links as a set of targets; forbidden ops; `Counts` P/R/F1); `Total`, `WriteReport` (the text table).
  CLI: `cmd/waypoint/eval.go` `runEval` is `waypoint eval [-provider X -model M -endpoint E -dir D -run S -v]` (key and timeout from config/env only).
- **profile**: `Manager` runs every profile (ADR 0005): `Open` (removes half-built `*.db.new`, moves the legacy
  `waypoint.db` in as "Personal", starts each `profiles/<id>.db`), `List`, `Create` (optionally copying another profile's appearance), `Rename`, `Delete`, `Serve`, `Ping`,
  `EndStreams`, `Close`; implements `api.Profiles`. `files.go`: `prepare` (migrate, name, seed, checkpoint, close), `removeDB`.
- **api**: chi router (`router.go`, route list in `handlers.routes`), one file per resource
  (`projects.go`, `milestones.go` (incl. `moveMilestone`, `milestoneMovePositions`), `tasks.go`, `blockers.go`, `ideas.go`, `entries.go`, `changesets.go`, `settings.go`, `export.go` (`GET /export`, the profile as a JSON attachment)),
  `profiles.go` (`Profiles` interface, `/api/profiles` routes, hands `/api/profiles/{profile}/*` to `Profiles.Serve`);
  `provider.go` (`Provider` interface, `GET`/`PUT /api/provider`, mounted when `Deps.Provider` is set);
  `NewRouter(Deps{Logger, Web, Profiles})` is the root, `NewProfileHandler(ProfileDeps{…})` one profile's data routes and `/events`,
  `projects.go` also has `GET /projects/{id}/observations`, `DELETE /projects/{id}?entries=keep|delete` and `GET /projects/{id}/deletion` (paged by entry; `pageQuery` in `entries.go` reads `before`/`limit`),
  `convert.go` (domain → apitypes, incl. `milestoneDTO`; `opRefs` names rows in op payloads, milestones too; `projectDTO` takes the last activity, `handlers.projectView` reads a project with it), `respond.go` (JSON encode/decode, `h.fail` maps errors to the
  single error shape, `badRequest`), `events.go` (`Hub`: SSE `/api/events`, `Publish`, `Notify`,
  `BriefingUpdated`, `Applied` → `changeset.applied`, `EntryDeleted` → `entry.deleted`), `serve.go`. Handlers are thin: decode, call a service, encode.
  **api/apitypes**: request/response/event DTOs; `gen/` generates `web/src/lib/api/*.gen.ts`
  (`make gen`, checked by `make lint`).
- **config**: `Load(args, env, goos, stderr)` (flags > env > `config.toml`; `provider`/`model`/`endpoint`/`api_key` from the file or
  `WAYPOINT_PROVIDER`/`_MODEL`/`_ENDPOINT`/`_API_KEY` only, no flags; `timeout` / `WAYPOINT_TIMEOUT`; `stalled_after_days` → `StalledAfter()`), `EnsureDataDir`, `ProfilesDir`, `LegacyDBPath`,
  `SettingsDefaults` (the preference values that seed a new database); `WritePath`, `ProviderFromEnv`. `write.go`: `SaveProvider(path, values)` edits only the provider keys' lines in `config.toml`, atomically, 0600.
- **web**: embeds `web/dist`, `NewHandler` serves the SPA with fallback.
- `tools/hooks`: Claude Code hooks (edit guard, formatter, stop check). Not app code.

## Frontend (`web/src`)

Svelte 5 runes + TS + Tailwind, Vitest. `main.ts` → `App.svelte`. `lib/api/client.ts` is the
typed API client (the only place that calls `fetch`); `apiPath` puts `/api/…` under the profile set by `useProfile`,
with generated `types.gen.ts`/`enums.gen.ts`; named calls (`listProjects`, `getSettings`, `updateSettings`,
`listProfiles`/`createProfile`/`renameProfile`/`deleteProfile`, `listMilestones`/`createMilestone`/`updateMilestone`/`deleteMilestone`/`moveMilestone`, `listProjectObservations`, `getBriefing`, `getProjectDeletion`, `deleteProject`) are thin wrappers over `getJSON`/`patchJSON`, and new code uses them.
`lib/router.ts` + `lib/navigation.svelte.ts`: client-side routing (`paths`, incl. `paths.diaryEntry(id)` → `/diary?entry=N`, read back by `entryParam(current.search)`); every screen's `<h1>` takes `{@attach pageHeading}` (focus after navigation). `screens/`: page components
(`Now`, `Diary`, `Projects`, `Project`, `Task`, `ParkingLot`, `Settings`, `NotFound`). `components/`: reusable
pieces. Tests sit next to the code as `*.test.ts`.

- Profiles (ADR 0005, SPEC §11): `lib/profile.svelte.ts` holds `profiles` (`list`, `active`, `notice`); `chooseProfile`
  (awaited in `main.ts`; remembered ID in `localStorage` if still listed, else the first), `switchProfile` (resets the timeline,
  swaps the per-profile diary draft, leaves project/task pages), `createProfile`/`renameProfile`/`removeProfile` (switches away
  first if active), `deleteConfirmed`, `nameProblem`. `App.svelte` keeps the header (`components/ProfileSwitcher.svelte`, a select)
  and wraps `components/ProfileWorkspace.svelte` (screens, diary, event stream) in `{#key profiles.active?.id}`, so a switch
  remounts all of it. Settings' Profiles section: `components/ProfileSettings.svelte` → `ProfileRow.svelte` (switch, rename,
  delete after typing the name). Tests: `lib/profile.test.ts`, `lib/diaryDraft.test.ts`, `components/ProfileSettings.test.ts`.
- Layout (ADR 0004): `ProfileWorkspace` mounts `DiaryPanel` (lg and up) or `DiaryOverlay` (below lg), never
  both, and neither on `/diary`. Both wrap `DiaryContent`, which `screens/Diary` also shows full-page.
  `lib/diaryPanel.ts`: panel width limits, drag/key resizing, `localStorage` prefs.
- Live updates: `lib/events.ts` is the app's one `EventSource` (`startEvents` in `ProfileWorkspace.svelte`, once per profile; `on(name, handler)`,
  `onReconnect`, `onDisconnect`). Screens subscribe in `onMount` and refetch; `lib/events.fake.ts` `FakeEventSource` for tests.
  `lib/live.ts`: `liveReload` (reload on `changeset.applied`, optionally for one project, and on reconnect) and
  `latest` (drop stale answers); used by the Project, Projects, Parking lot, and Task screens. `screens/Project.test.ts`.
- Server status: `lib/health.ts` (`fetchHealth`, `watchHealth`: asks at start, on stream drop/reconnect, and
  re-polls only while unhealthy) → `components/ServerBanner.svelte` under the header in `App.svelte`; nothing shows while healthy.
- Diary input: `components/DiaryInput.svelte` (in `DiaryContent`); `lib/diary.ts` (submit shortcut,
  default project scope from the route, `entryRequest`); `lib/diaryDraft.svelte.ts` (shared draft that
  outlives the panel/overlay; one per profile: `switchDraft`, `restoreDraft`, `draftSession`).
- Diary timeline: `components/EntryTimeline.svelte` → `EntryCard.svelte` (highlights, run state) →
  `FollowUpQuestion.svelte` (answer/dismiss). `lib/timeline.svelte.ts`: shared timeline state (`loadTimeline`,
  `loadOlder`, `loadThrough` (pages back to one entry), `loadEntry`, `addEntry`, `reprocessEntry`, `deleteEntry`, `followEvents`). `lib/entries.ts`: `segments`, `observationHref`, detail helpers,
  reading again (`canReprocess`, `reprocessImpact`, `reprocessWarning`). `components/EntryCard.test.ts`.
  Entry links: `screens/Diary` passes `?entry=N` as `focusEntry` through `DiaryContent` to `EntryTimeline` (`loadThrough`, scroll, brief
  `highlighted` ring on `EntryCard`); `components/EntryTimeline.test.ts`.
- Changeset review: `EntryCard` → `components/ChangesetReview.svelte` (one per pending changeset; project
  picker, Apply / Accept all) → `ChangeOpRow.svelte` (accept/reject/edit). `lib/review.ts`: pure logic
  (`reject`/`accept` cascades over `$new:N`, `guessedProject`, `noProject`, `applyRequest`, `describe` using
  the op's `refs`, `editFields`).
- Now: `screens/Now.svelte` loads active projects' briefings into `components/BriefingCard.svelte` (one block per project: next action, recap as a dated list, blockers and alternatives); `components/BriefingCard.test.ts`, `screens/Now.test.ts`.
  `lib/dates.ts` `relativeDay` ("today", "yesterday", "3 days ago", then the date).
  `lib/projects.ts`: grouping (`groupByStatus`, newest `last_activity_at` first), `filterProjects`, `collapsedByDefault`, `activeProjects`, `pickable` (the rule every project picker uses: no done or archived projects unless already chosen), cap helpers.
- Project pickers: `components/ProjectPicker.svelte` (a combobox: type to narrow, arrows + Enter, Escape; "No project" always a choice; callers pass `pickable` projects) used by the diary scope and review; `lib/picker.ts` (`pickerOptions`, `selectedLabel`, `step`); `components/ProjectPicker.test.ts`, `lib/picker.test.ts`.
- Projects: `screens/Projects.svelte` (filter box, status groups with Done and Archived collapsible, new project form, **Restore** on archived projects → parked); `screens/Projects.test.ts`.
- Project: `screens/Project.svelte` → `components/ProjectHeader.svelte` (view/edit title, why, definition of done)
  and `ProjectStatus.svelte` (status buttons, calm cap handling). `lib/projects.ts`: `projectPatch` (changed fields
  only), `statusOptions`, `activateCapMessage`. `lib/api/client.ts` also has `patchJSON`/`deleteJSON` (204 → undefined).
  Tasks: `components/ProjectTasks.svelte` (loads tasks and milestones; sections by milestone, then by status; add forms for tasks and milestones; move up/down) →
  `MilestoneSection.svelte` (a milestone's header: edit title and description, status, move, delete with confirmation; done/dropped fold) and `TaskRow.svelte` (view,
  inline edit incl. the milestone picker for top-level tasks, status, delete with confirmation); move up/down is one `POST /api/tasks/{id}/move` (`api/tasks.go`
  `moveTask`, `movePositions`: among siblings, i.e. same parent, and same status and milestone for top-level tasks).
  `lib/milestones.ts`: `milestoneSections` (splits `nestTasks`' groups by milestone), `sortMilestones`, `milestonePatch`, `milestoneLabel`, `milestoneStatusLabels`. Steps nest under their parent through `TaskRow`'s `children` snippet.
  `lib/tasks.ts`: `groupTasks`, `nestTasks` (top-level groups + steps by parent), `taskPatch`, `newTaskRequest`, `newStepRequest`, `stepStatus`, `stepLabel`.
  Blockers: `components/ProjectBlockers.svelte` (open, then resolved) → `BlockerRow.svelte` (resolve, delete with confirmation, and with `tasks` a "Blocks" picker that moves an open blocker between tasks and the whole project);
  `ProjectBlockers` also has the add form with an optional open-task picker; `lib/blockers.ts`: `splitBlockers`, `openTasks`, `newBlockerRequest`, `blockerTargets`, `moveBlockerRequest`. Ideas: `components/ProjectIdeas.svelte` (this project's parked ideas) → `IdeaRow.svelte`.
  Delete (ADR 0008): `components/ProjectDelete.svelte` (at the foot of the screen; `getProjectDeletion` preview, `lib/projects.ts` `deletionSummary`,
  keep or delete its entries, then `deleteProject` and go to Projects). Elsewhere a deleted project drops out: `lib/diary.ts` `knownScope` (diary scope),
  and the timeline re-reads projects on `changeset.applied` and unscopes entries whose project is gone. `components/ProjectDelete.test.ts`.
  Next action: `components/ProjectNext.svelte` (`getBriefing`, reloads on this project's `briefing.updated` and on reconnect); `components/ProjectNext.test.ts`.
  From the diary: `components/ProjectDiary.svelte` (`listProjectObservations`, grouped by `observationTimeline`, "Show more", reloads on `entry.processed` too); `components/ProjectDiary.test.ts`.
- Task detail: `screens/Task.svelte` (`GET /api/tasks/{id}`: fields, origin entry, linked observations grouped by entry
  via `lib/tasks.ts` `observationTimeline`; otherwise read-only, edits live on the Project screen) → `components/TaskSteps.svelte` (`TaskDetail.steps` as a checklist: add, tick done/todo, move, delete; `TaskSteps.test.ts`) and `components/TaskBlockers.svelte` (the task's open blockers from `TaskDetail.blockers`: resolve, delete, add on this task). `lib/entries.ts` `kindLabel`.
- Parking lot: `screens/ParkingLot.svelte` (all parked ideas and parked projects) → `components/IdeaRow.svelte` (edit title and body inline, `onchanged`; promote to a
  task in a chosen project or to a parked project, delete with confirmation; also used by `ProjectIdeas`) and
  `ParkedProjectRow.svelte` (activate, calm cap handling). `lib/ideas.ts`: `promoteTargets`, `defaultTarget`, `projectName`, `ideaPatch`; `IdeaRow.test.ts`;
  `lib/projects.ts` `parkedProjects`.
- Settings: `screens/Settings.svelte` (`/settings`, linked from the header; one `<section>` per group, "Focus" first:
  active-project cap and questions per entry, saved as one PATCH of the changed fields; then "Appearance"; then "Data" (link to `exportURL()` from `client.ts`, the copy-the-DB-file note); then "Model"; then "Profiles"). `lib/settings.ts`: `settingsPatch`,
  `parseCap`, `capNote` (a cap below the active count pauses nothing), `fieldProblems` (422 → per-field). `screens/Settings.test.ts`.
  Model section (server-wide, ADR 0010): `components/ProviderSettings.svelte` on `getProvider`/`updateProvider` (`/api/provider`, in `client.ts`'s `serverWide`);
  `lib/provider.ts`: `providerForm`, `providerRequest` (PUT body or null; `api_key` only to replace or clear; local per-field checks), `providerProblems`, `fromEnv`/`envNote`, `providerLabel`, `fieldPlaceholder`. Tests: `lib/provider.test.ts`, `components/ProviderSettings.test.ts`.
- Appearance (theme and fonts): `fonts.ts` (imported by `main.ts`) bundles the `@fontsource-variable/*` faces. `lib/appearance.ts`: `themes`/`themeChoices`,
  `fonts` (setting value → label and CSS stack), `interfaceFontChoices`/`diaryFontChoices`, `appearancePatch`, `applyAppearance`
  (sets `data-theme` and overrides `--font-sans`/`--font-diary` on `<html>`), `restoreTheme` (called first in `main.ts`; the theme last shown,
  `localStorage` `waypoint.theme`, against a flash on load), `setAppearance` (after a save), `loadAppearance` (called by
  `ProfileWorkspace` on mount, so on every profile switch). Settings' Appearance section: `components/AppearanceSettings.svelte`
  (each choice applies at once and saves straight away, one save at a time, later choices coalesced; a failed save reverts to the last saved choice; theme swatches each carry their own `data-theme`). Diary text uses the `font-diary` utility. Tests: `lib/appearance.test.ts`,
  `components/AppearanceSettings.test.ts`.
- Theme: named tokens in `app.css` `@theme inline`, each reading a variable that the `[data-theme='…']` blocks set (Midnight is also `:root`),
  so anything with `data-theme` shows that theme (`canvas`/`surface`/`raised`/`line`, `ink`/`ink-muted`/
  `ink-faint`, `accent`/`accent-strong`/`accent-deep`/`on-accent`, `plan` (plan highlights; `lib/entries.ts` `highlightClass`), `shadow-glow`/`focus`/`edge`,
  `text-title`, `px-gutter`, `rounded-card`). Use these, not raw Tailwind colours. Thin scrollbars from tokens in `@layer base`.
  `lib/theme.test.ts` checks every theme: all colours set, AA contrast for the text pairs, accents away from `danger`.
- `components/ui/`: `Button` (`primary`/`secondary`/`quiet`/`danger`, `size` `md`/`sm`/`xs`), `Input`, `Textarea` (`bind:value`; grows to
  `max-h-64`, or `max-h-96` with `tall`, then scrolls, via `lib/autosize.ts`),
  `Card` (`glow` for the one thing to notice), `Panel` (row container).
  Rich text (ADR 0009; task notes, project why and done, idea bodies; stored as markdown): `RichTextEditor` (`bind:value` markdown, `id`, `labelledby`
  naming it, toolbar, link address box; only edits write the value) and `RichText` (read-only view); both use `lib/richtext.ts`
  (`richTextExtensions`, `markdownOf`, `plainText` for one-line previews such as the Projects list) and the `.rich-text` styles in `app.css`. In tests the editor is `(el as HTMLElement & { editor }).editor`
  on the `#id` element; `RichTextEditor.svelte.test.ts`, `lib/richtext.test.ts`. `npm --prefix web test`, `npm --prefix web run check`.
- Icons: `@lucide/svelte`, imported one per path (`import BookOpen from '@lucide/svelte/icons/book-open'`) so only
  used icons are bundled; decorative icons get `aria-hidden="true"`, and icon-only buttons an `aria-label`.
- Component tests: `// @vitest-environment jsdom` at the top, then Svelte's `mount`/`flushSync` with a stubbed
  `fetch`; `components/ChangesetReview.test.ts` is the pattern. Svelte delegates events, so dispatch them
  with `bubbles: true`.
- E2E: `web/e2e/smoke.spec.ts` (Playwright, `make e2e`) drives the real binary with the mock extractor;
  `web/playwright.config.ts` starts it on port 7429 with a temp data dir and an empty config. Select by role and label.

## Test fixtures (reuse these; don't write new setup)

All use a real temp SQLite DB migrated with `db.Migrations()`.

- `store/store_test.go`: `newTestStore(t) (*Store, *sql.DB, *clock)`, `inTx`, `exec`, `count`,
  `seedProject`, `seedTask`, `seedEntry`, `seedRun`. `delete_test.go`: `linkTo`.
- `changeset/apply_test.go`: `newFixture(t) fixture` (in `validate_test.go`, two projects with tasks, blockers, an idea, and a milestone each (`mA`, `mB`); with `withCap(t, s, limit, after)`), `propose(t, f, runID, ops...)`, `newRun`,
  `opStatuses`, `taskCount`, `dbSnapshot` (rollback checks). `links_test.go`: `observeOps` (pending
  links to every op's entity), `linkedFrom`, `pendingLinks`. `validate_test.go`: `fields(t, err)`. `project_delete_test.go`: `scopedEntries`.
- `pipeline/snapshot_test.go`: `newFixture(t)` with `f.project/task/blocker/idea/entry/observe`.
  `worker_test.go`: `f.worker(extractor, notifier)`, `f.save`, `f.run`, `f.stored`, `processNext`,
  `fakeExtractor`, `events` notifier.
- `briefing/service_test.go`: `newFixture(t)` with `f.manual(ops...)`, `f.project`, `f.task`,
  `f.entry`, `f.briefing`, `fixedBriefer`, `hookBriefer`, `chanNotifier`.
- `briefing/llm_test.go`: `fakeCaller{input, err}` (records the `ToolCall`), `newLLM(caller)`, `llmState()`.
- `api/profiles_test.go`: `oneProfile(h, db)` (fake `Profiles` with profile "t"), `profileRouter(ProfileDeps)`;
  data paths in api tests are `/api/profiles/t/…`. `profile/manager_test.go`: `newEnv(t)`, `e.open()` (manager behind the
  real router; open again to restart), `a.call`, `a.events(id)`, `legacyMigrations` (schema before 00007).
- `api/routes_test.go`: `newServer(t)`; `api/integration_test.go`: `startApp(t, dbPath, extractor)`
  (full wiring over HTTP + SSE); `api/events_test.go`: `newEventsServer`, `openStream`.
- `db/schema_test.go`: `migrated(t)`, `seed`, `noFKViolations`.
- `extract/mock/mock_test.go`: `run(t, snap, project, body)`.
- `extract/openaicompat/openaicompat_test.go`: `newServer(t, status, reply)` (fake chat completions server), `toolReply`, `contentReply`, `input()`.
- `extract/anthropic/anthropic_test.go`: `newServer(t, status, reply)` (fake Messages server), `reply(stopReason, blocks...)`, `toolUse`, `text`, `input()`.
- `testdata/entries/`: eval fixtures, `NN-slug.md` (the entry) + `NN-slug.json` (`snapshot`, `project_id`, `reply_to`, `asked` (questions added to a recent entry for that fixture only), `notes`, `expected` result, `forbid` op types);
  `snapshots/waypoint.json` (Waypoint mid-Stage 2, Garden shed, paused Learn Spanish; task 10 stalled, 12 size l; IDs: projects 1-3, tasks 10-16/50-52/70, milestones 40-41, blockers 20-21/60, ideas 30-32) and `snapshots/empty.json`.
  `eval/fixtures_test.go` loads them all; `write(t, dir, name, content)` builds a throwaway fixture dir.
- `extract/validate_test.go`: `input()` (entry + snapshot), `opDraft`, `obs`, `link`, `fields`.
