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
  0003 splitting changesets, 0004 diary panel layout, 0005 profiles, 0006 archived projects.

## Wiring (`cmd/waypoint/main.go`)

`config.Load` → `profile.Open(ctx, profile.Options{Dir: cfg.ProfilesDir(), LegacyDB: cfg.LegacyDBPath(),
Seed: cfg.SettingsDefaults(), Extractor: mock.New(…), …})` → `api.NewRouter(api.Deps{Logger, Web, Profiles})` →
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
  `ErrNotFound` for missing rows. Entries are insert-only. Settings (`settings.go`): `GetSettings`
  (defaults if no row), `Store.SeedSettings` (insert if missing), `Tx.UpdateSettings`. Pending observation links (to `$new:N`,
  keyed by change op): `CreatePendingLink`, `ListPendingLinksByOp`, `ResolvePendingLinks`, `DropPendingLinks`.
  Profile row (`profile.go`): `GetProfile` (name, created), `Tx.SetProfileName`.
- **changeset**: every mutation of projects/tasks/blockers/ideas. `ops.go`: one payload struct per
  op type (`CreateTask`, `UpdateTask`, `SplitTask`, `DeleteTask`, `ParkIdea`, …) implementing
  `Payload`; `DecodePayload`, `ValidateOps`; `Nullable[T]` (`To`, `Clear`) for PATCH-style fields.
  `ref.go`: `Ref` (`ID(n)` or `Temp(pos)` for `$new:N`), `CheckRefs`. `validate.go`: cross-row
  rules (same project, no parent cycles). `apply.go`: `Applier.Apply(ctx, changesetID,
  map[opID]Decision)` → `Result{Changeset, Created, SplitOff, HeldByCap, Projects}`; one
  transaction, full rollback, active-project cap read from the settings in the apply tx, undecided/capped ops split into a new pending changeset (ADR 0003);
  resolves pending observation links on applied creating ops, drops them on rejected ones;
  calls `AfterApply.Applied`. `manual.go`: `Applier.ApplyManual(ctx, ops ...Payload)` for
  `source=manual`. `reject.go`: `RejectPending`. `AfterApplies` chains hooks (briefings, then the SSE hub).
- **extract**: provider-neutral boundary. `Extractor{Extract, Name}`, `ExtractInput` (entry, snapshot, `ReplyTo` question),
  `ExtractionResult` (observations, links, `ChangeOpDraft`s, `QuestionDraft`s), `StateSnapshot`
  (`Has`), `Validate` (shared validator, `MaxQuestions = 2`). `LinkDraft.TargetID` is a `changeset.Ref`,
  so a link can name `$new:N`; the worker stores those pending.
  **extract/mock**: deterministic line-prefix extractor (SPEC §10.4), `mock.New(latency)`.
- **pipeline**: `SnapshotBuilder.Build(ctx, entry)`; `Worker`: `SaveEntry` (entry + pending run in
  one tx), `Reprocess`, `Wake`, `Run(ctx)` (claims runs, extracts, validates, stores result +
  changeset (keeping only `questions_per_entry` questions), emits `Event`s via `Notifier`, calls `AfterRun.EntryProcessed`). Failures recorded on
  the run, never partial state.
- **briefing**: `Briefer` interface, `Heuristic` implementation (`heuristic.go`), `Service`
  (`Refresh`, `Get` → `View`; drops recap entries the Briefer wasn't shown; implements `changeset.AfterApply` and `pipeline.AfterRun`; notifies
  `BriefingUpdated`). Briefings are cached per project in the DB; the recap (`[]domain.RecapEntry`, newest first, points with observation kinds) in `briefing_recap_points`.
- **profile**: `Manager` runs every profile (ADR 0005): `Open` (removes half-built `*.db.new`, moves the legacy
  `waypoint.db` in as "Personal", starts each `profiles/<id>.db`), `List`, `Create` (optionally copying another profile's appearance), `Rename`, `Delete`, `Serve`, `Ping`,
  `EndStreams`, `Close`; implements `api.Profiles`. `files.go`: `prepare` (migrate, name, seed, checkpoint, close), `removeDB`.
- **api**: chi router (`router.go`, route list in `handlers.routes`), one file per resource
  (`projects.go`, `tasks.go`, `blockers.go`, `ideas.go`, `entries.go`, `changesets.go`, `settings.go`),
  `profiles.go` (`Profiles` interface, `/api/profiles` routes, hands `/api/profiles/{profile}/*` to `Profiles.Serve`);
  `NewRouter(Deps{Logger, Web, Profiles})` is the root, `NewProfileHandler(ProfileDeps{…})` one profile's data routes and `/events`,
  `projects.go` also has `GET /projects/{id}/observations` (paged by entry; `pageQuery` in `entries.go` reads `before`/`limit`),
  `convert.go` (domain → apitypes), `respond.go` (JSON encode/decode, `h.fail` maps errors to the
  single error shape, `badRequest`), `events.go` (`Hub`: SSE `/api/events`, `Publish`, `Notify`,
  `BriefingUpdated`, `Applied` → `changeset.applied`), `serve.go`. Handlers are thin: decode, call a service, encode.
  **api/apitypes**: request/response/event DTOs; `gen/` generates `web/src/lib/api/*.gen.ts`
  (`make gen`, checked by `make lint`).
- **config**: `Load(args, env, goos, stderr)` (flags > env > `config.toml`), `EnsureDataDir`, `ProfilesDir`, `LegacyDBPath`,
  `SettingsDefaults` (the preference values that seed a new database).
- **web**: embeds `web/dist`, `NewHandler` serves the SPA with fallback.
- `tools/hooks`: Claude Code hooks (edit guard, formatter, stop check). Not app code.

## Frontend (`web/src`)

Svelte 5 runes + TS + Tailwind, Vitest. `main.ts` → `App.svelte`. `lib/api/client.ts` is the
typed API client (the only place that calls `fetch`); `apiPath` puts `/api/…` under the profile set by `useProfile`,
with generated `types.gen.ts`/`enums.gen.ts`; named calls (`listProjects`, `getSettings`, `updateSettings`,
`listProfiles`/`createProfile`/`renameProfile`/`deleteProfile`, `listProjectObservations`, `getBriefing`) are thin wrappers over `getJSON`/`patchJSON`, and new code uses them.
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
  `loadOlder`, `loadThrough` (pages back to one entry), `loadEntry`, `addEntry`, `reprocessEntry`, `followEvents`). `lib/entries.ts`: `segments`, `observationHref`, detail helpers,
  reading again (`canReprocess`, `reprocessImpact`, `reprocessWarning`). `components/EntryCard.test.ts`.
  Entry links: `screens/Diary` passes `?entry=N` as `focusEntry` through `DiaryContent` to `EntryTimeline` (`loadThrough`, scroll, brief
  `highlighted` ring on `EntryCard`); `components/EntryTimeline.test.ts`.
- Changeset review: `EntryCard` → `components/ChangesetReview.svelte` (one per pending changeset; project
  picker, Apply / Accept all) → `ChangeOpRow.svelte` (accept/reject/edit). `lib/review.ts`: pure logic
  (`reject`/`accept` cascades over `$new:N`, `guessedProject`, `noProject`, `applyRequest`, `describe` using
  the op's `refs`, `editFields`).
- Now: `screens/Now.svelte` loads active projects' briefings into `components/BriefingCard.svelte` (one block per project: next action, recap as a dated list, blockers and alternatives); `components/BriefingCard.test.ts`, `screens/Now.test.ts`.
  `lib/dates.ts` `relativeDay` ("today", "yesterday", "3 days ago", then the date).
  `lib/projects.ts`: grouping, `activeProjects`, `pickable` (the rule every project picker uses: no done or archived projects unless already chosen), cap helpers.
- Projects: `screens/Projects.svelte` (status groups, new project form, **Restore** on archived projects → parked); `screens/Projects.test.ts`.
- Project: `screens/Project.svelte` → `components/ProjectHeader.svelte` (view/edit title, why, definition of done)
  and `ProjectStatus.svelte` (status buttons, calm cap handling). `lib/projects.ts`: `projectPatch` (changed fields
  only), `statusOptions`, `activateCapMessage`. `lib/api/client.ts` also has `patchJSON`/`deleteJSON` (204 → undefined).
  Tasks: `components/ProjectTasks.svelte` (list grouped by status, add form, move up/down) → `TaskRow.svelte` (view,
  inline edit, status, delete with confirmation); move up/down is one `POST /api/tasks/{id}/move` (`api/tasks.go`
  `moveTask`, `movePositions`). `lib/tasks.ts`: `groupTasks`, `taskPatch`, `newTaskRequest`.
  Blockers: `components/ProjectBlockers.svelte` (open, then resolved) → `BlockerRow.svelte` (resolve, delete with confirmation);
  `ProjectBlockers` also has the add form with an optional open-task picker; `lib/blockers.ts`: `splitBlockers`, `openTasks`, `newBlockerRequest`. Ideas: `components/ProjectIdeas.svelte` (this project's parked ideas) → `IdeaRow.svelte`.
  Next action: `components/ProjectNext.svelte` (`getBriefing`, reloads on this project's `briefing.updated` and on reconnect); `components/ProjectNext.test.ts`.
  From the diary: `components/ProjectDiary.svelte` (`listProjectObservations`, grouped by `observationTimeline`, "Show more", reloads on `entry.processed` too); `components/ProjectDiary.test.ts`.
- Task detail: `screens/Task.svelte` (`GET /api/tasks/{id}`: fields, origin entry, linked observations grouped by entry
  via `lib/tasks.ts` `observationTimeline`; read-only, edits live on the Project screen). `lib/entries.ts` `kindLabel`.
- Parking lot: `screens/ParkingLot.svelte` (all parked ideas and parked projects) → `components/IdeaRow.svelte` (promote to a
  task in a chosen project or to a parked project, delete with confirmation; also used by `ProjectIdeas`) and
  `ParkedProjectRow.svelte` (activate, calm cap handling). `lib/ideas.ts`: `promoteTargets`, `defaultTarget`, `projectName`;
  `lib/projects.ts` `parkedProjects`.
- Settings: `screens/Settings.svelte` (`/settings`, linked from the header; one `<section>` per group, "Focus" first:
  active-project cap and questions per entry, saved as one PATCH of the changed fields; then "Appearance"; then "Profiles"). `lib/settings.ts`: `settingsPatch`,
  `parseCap`, `capNote` (a cap below the active count pauses nothing), `fieldProblems` (422 → per-field). `screens/Settings.test.ts`.
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
  `Card` (`glow` for the one thing to notice), `Panel` (row container). `npm --prefix web test`, `npm --prefix web run check`.
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
  `seedProject`, `seedTask`, `seedEntry`, `seedRun`.
- `changeset/apply_test.go`: `newFixture(t) fixture` (in `validate_test.go`, with `withCap(t, s, limit, after)`), `propose(t, f, runID, ops...)`, `newRun`,
  `opStatuses`, `taskCount`, `dbSnapshot` (rollback checks). `links_test.go`: `observeOps` (pending
  links to every op's entity), `linkedFrom`, `pendingLinks`. `validate_test.go`: `fields(t, err)`.
- `pipeline/snapshot_test.go`: `newFixture(t)` with `f.project/task/blocker/idea/entry/observe`.
  `worker_test.go`: `f.worker(extractor, notifier)`, `f.save`, `f.run`, `f.stored`, `processNext`,
  `fakeExtractor`, `events` notifier.
- `briefing/service_test.go`: `newFixture(t)` with `f.manual(ops...)`, `f.project`, `f.task`,
  `f.entry`, `f.briefing`, `fixedBriefer`.
- `api/profiles_test.go`: `oneProfile(h, db)` (fake `Profiles` with profile "t"), `profileRouter(ProfileDeps)`;
  data paths in api tests are `/api/profiles/t/…`. `profile/manager_test.go`: `newEnv(t)`, `e.open()` (manager behind the
  real router; open again to restart), `a.call`, `a.events(id)`, `legacyMigrations` (schema before 00007).
- `api/routes_test.go`: `newServer(t)`; `api/integration_test.go`: `startApp(t, dbPath, extractor)`
  (full wiring over HTTP + SSE); `api/events_test.go`: `newEventsServer`, `openStream`.
- `db/schema_test.go`: `migrated(t)`, `seed`, `noFKViolations`.
- `extract/mock/mock_test.go`: `run(t, snap, project, body)`.
