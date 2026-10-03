# 0006. Archived projects

- **Status:** Accepted
- **Date:** 2026-10-03
- **PLAN task:** 1.5.12

## Context

Projects pile up. A finished or abandoned project marked `done` still shows in the diary's
"About" picker when it's the current scope, in the Projects list, and in other places where only
current work belongs. The owner wants to put a project out of sight without deleting it (deleting
comes in 1.5.13 and loses the history). SPEC §5's "no guilt mechanics" applies: putting something
away should be calm and easy to undo.

## Options considered

1. **An `archived` status.** One more value in `projects.status`. Every place that already
   filters by status (Now shows active, the Parking lot shows parked, the snapshot takes active
   plus the entry's project) leaves it out with no change. Status changes already go through
   `update_project`, so archiving and restoring are ordinary manual changesets. SQLite can't alter
   a CHECK, so the `projects` table is rebuilt in a migration.
2. **An `archived_at` flag beside the status.** It keeps the status from before archiving, but every
   query and screen has to check two fields, and the status rules (the active cap) need a case for
   "active but archived".

## Decision

`archived` is a fifth project status (option 1). Any status can change to it, as with the others;
archiving an active project frees a slot under the cap. Restoring from the Projects screen sets the
project to `parked`, so bringing it back never runs into the cap or puts it on the Now screen
unasked. An archived project stays in the extraction snapshot only when the entry is scoped to it
(the existing rule for an entry's own project), so reading an old entry again still works; new
entries can't be scoped to it because the pickers hide it.

## Consequences

- Pickers use one rule, `pickable` in `web/src/lib/projects.ts`: done and archived projects are left
  out unless already chosen.
- `domain.ProjectStatuses()` and the CHECK in migration 00011 must stay in step (`db/enums_test.go`).
- Going down a migration turns archived projects into parked ones.
