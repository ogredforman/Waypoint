# 0008. Deleting projects

- **Status:** Accepted
- **Date:** 2026-10-03
- **PLAN task:** 1.5.13 (backend), 1.5.13b (UI)

## Context

ADR 0002 ruled out deleting projects because entries reference them and entries are immutable. Since
then, archiving (ADR 0006) gives a soft way out of view, and deleting entries (ADR 0007) settled that
an entry may go whole, and that clearing a reference to it is cleanup, not an edit. A project created
by mistake, or one the user is sure they want gone, still can't be removed, and a project list the
user can't prune works against the product as much as a task list they can't.

What hangs off a project: tasks (with subtasks and dependencies, always within the project),
blockers, ideas tied to it, milestones, its cached briefing and recap points, observation links to
any of these, and the entries scoped to it (`entries.project_id`). Older change ops name it in their
payloads. None of the foreign keys to `projects` has an `ON DELETE` action.

## Options considered

1. **No deletes; archive only.** Simple, but the user can never truly remove a project.
2. **Delete the project with its contents, always keeping the entries** (unscoped). Safe for the
   diary, but someone deleting a project created in the wrong profile wants its entries gone too.
3. **Delete the project with its contents, and let the user choose for its entries**: keep them
   unscoped, or delete them whole as ADR 0007 does.

For where the entry deletion runs: in the handler before the project op (two transactions, so a
failure halfway leaves entries gone and the project kept), or inside the `delete_project` op (one
transaction).

## Decision

Option 3, as one op: `delete_project` {project_id, delete_entries?} in a `source=manual` changeset.
`store.Tx.DeleteProject` removes the observation links to the project and everything in it, the
briefing, dependencies, blockers, tasks, ideas, and milestones, clears `project_id` on the entries
still scoped to it, and deletes the row. With `delete_entries`, the applier first deletes each scoped
entry with `Tx.DeleteEntry`, in the same transaction, and refuses (`run_in_progress`) if any of their
runs is running. Only the entries scoped to the project are its entries; an unscoped entry that
mentioned it only loses the links.

Extraction never proposes `delete_project`: the shared validator rejects it. Deleting a project is
the user's explicit, confirmed choice, and an extraction changeset deleting entries could delete its
own entry while it is being applied.

Pending changesets whose ops refer to the deleted project or its rows stay; those ops fail validation
when applied and the user rejects them, as with deleted tasks (ADR 0002).

## Consequences

- A migration rebuilds `change_ops` for the new op type. `pending_observation_links` references it,
  so the rebuild copies the rows aside and recreates the table under its own name.
- The applied payload records the deleted project, how many tasks, blockers, ideas, and milestones
  went with it, and the IDs of its entries (kept or deleted, per `delete_entries`).
- `changeset.Result.DeletedEntries` names the entries deleted with a project; the SSE hub publishes
  `entry.deleted` for each, and the briefing service refreshes every active project, since those
  entries may be in any recap. A deleted project is skipped when refreshing briefings.
- A new table that references projects or anything in them must extend `DeleteProject`;
  `TestDeleteProjectRemovesWhatIsInIt` checks for foreign key violations.
- Keeping the entries doesn't wait for their running runs: a run that finishes after the project is
  gone proposes ops that fail validation when applied, and the user rejects them.
- `GET /api/projects/{id}/deletion` gives the counts the confirmation shows (PLAN 1.5.13b).
