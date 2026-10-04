# 0002. Deleting tasks, blockers, and ideas

- **Status:** Accepted; "projects can't be deleted" superseded by [0008](./0008-deleting-projects.md)
- **Date:** 2026-09-29
- **PLAN task:** 1.2 (store), 1.4 (op types and migration)

## Context

The first store design had no deletes: tasks can be `dropped`, ideas `discarded`, and blockers
`resolved`, and keeping every row keeps the audit trail simple. But if the user is certain they
don't want a task anymore, being unable to remove it is frustrating, and a planner full of junk the
user can't get rid of works against the product.

Constraints: every mutation of projects, tasks, blockers, and ideas goes through a changeset
(AGENTS.md). Entries are immutable (SPEC §5). Several tables point at tasks with foreign keys
(`tasks.parent_task_id`, `task_dependencies`, `blockers.task_id`, `briefings.next_task_id`), and
`observation_links` points at tasks, blockers, and ideas without one.

## Options considered

1. **No deletes; statuses only.** Simple and fully auditable, but the user can never truly remove
   something.
2. **Soft delete** (a `deleted_at` column). Keeps history, but every query must filter it out, and
   "deleted" becomes one more status next to dropped/discarded/resolved.
3. **Hard delete through a changeset op, cleaning up references.** The row is gone; the op records
   what was deleted.

## Decision

Option 3 for tasks, blockers, and ideas. Deleting runs in one transaction and cleans up what pointed
at the row:

- **Task:** subtasks become top-level tasks; dependencies in either direction are removed; blockers
  on the task stay on the project with no task; briefings drop it as the next action (and are
  recomputed); observation links to it are removed.
- **Blocker, idea:** observation links to it are removed.

Observations and entries are never removed; only the link goes. Projects can't be deleted: entries
reference them and entries are immutable. Parking or finishing a project takes it out of view.

The statuses stay as the soft option (dropped, discarded, resolved). Deleting is the explicit,
confirmed one.

## Consequences

- New op types `delete_task`, `delete_blocker`, `delete_idea` (SPEC §6.2). The `change_ops.op_type`
  CHECK list must change, which in SQLite means a new migration that rebuilds `change_ops`. That, the
  Go op types, and the payloads are PLAN 1.4.
- The applier stores the deleted row in the op's `applied_payload`, so history stays readable after
  the row is gone. Older ops (e.g. the `create_task`) keep pointing at an ID that no longer exists;
  anything that renders history must handle that.
- A pending changeset whose op references a deleted row fails validation at apply time for that op;
  the user rejects it.
- The store's `DeleteTask`, `DeleteBlocker`, and `DeleteIdea` (PLAN 1.2) own the cleanup rules. A
  new table that references tasks, blockers, or ideas must extend them.
- The UI asks for confirmation before deleting (PLAN 1.19, 1.21).
