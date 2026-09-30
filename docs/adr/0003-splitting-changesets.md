# 0003. Splitting a changeset when it is applied

- **Status:** Accepted
- **Date:** 2026-09-29
- **PLAN task:** 1.5 (applier)

## Context

A changeset from one diary entry can mix things the user is sure about ("mark the task done") with
things they want to think about (a parked idea, a new project). Reviewing it as one batch forces a
decision on everything at once. Applying ops one by one would leave half-applied changesets
lingering in review, which works against the no-guilt principle (SPEC §5).

The active-project cap (PLAN 1.3) raises the same question from the other side. An extraction
changeset can propose making a project active when the cap is already reached. That op fails, but
the rest of the changeset doesn't depend on it: only ops that *create* something can be referenced
by later ops, and a status change creates nothing.

The first schema allowed one changeset per extraction run (`changesets.extraction_run_id UNIQUE`).

## Options considered

1. **Batch only; every op must be decided.** Simple, but a single undecided op blocks the rest, and
   the cap fails the whole changeset.
2. **Incremental per-op apply.** Each decision applies at once and the changeset stays pending until
   all are decided. Temp IDs have to be remembered across requests, and "pending" comes to mean
   "partly applied": the pile-up we want to avoid.
3. **Batch with a split.** Apply what is decided in one transaction and resolve that changeset. The
   ops left undecided, or held back by the cap, move to a new pending changeset that points at the
   original.

## Decision

Option 3. Apply takes decisions for any non-empty subset of ops. Decided ops are applied or rejected
and the changeset is resolved. The rest move, keeping their positions, to a new pending changeset
with `parent_changeset_id` set and the same source and extraction run. Temp refs to rows created
before the split are rewritten to the real IDs; refs to ops that moved with them stay temp refs.

- An op that isn't rejected can't use what a rejected op creates, and an applied op can't use what
  an undecided op creates. The apply fails with a validation error rather than silently moving or
  rejecting the op the user accepted.
- On an extraction changeset, an op that would go over the active-project cap moves to the new
  changeset instead of failing, keeping the user's edit if they made one. Ops apply in position
  order, so the cap is checked against the state at that op's position; an op that parks a project
  later in the same changeset doesn't make room for an earlier one. If nothing else was decided, the cap error is returned and the
  changeset stays pending, since splitting would only move all of it.
- On a manual changeset (a button click), going over the cap fails the apply, and the UI shows the
  prompt to pause or park a project.
- Any other failure still rolls back the whole apply.

## Consequences

- Migration 00003 rebuilds `changesets`: `extraction_run_id` is no longer unique, and
  `parent_changeset_id` is added. Going down drops split-off changesets and their ops.
- One extraction run can have several changesets, linked by `parent_changeset_id`. Anything that
  finds "the changeset for a run" must expect a chain.
- A moved op's stored payload can differ from what the extractor proposed: temp refs are replaced by
  the IDs they resolved to, and an edited op held back by the cap carries the edit.
- The review UI (PLAN 1.18) has one Apply button that applies whatever has been decided; the rest
  stays in review as a new, smaller changeset.
