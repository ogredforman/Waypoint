# 0007. Deleting diary entries

- **Status:** Accepted
- **Date:** 2026-10-03
- **PLAN task:** 1.5.14 (and 1.5.13, whose "delete the entries too" reuses it)

## Context

Entries are immutable (SPEC §5): the system never edits what the user wrote. But an entry written by
mistake, in the wrong profile, or that the user simply doesn't want kept can't be removed, and a
diary the user can't prune works against the product as much as a task list they can't (ADR 0002).

Many rows hang off an entry: its extraction runs; their observations, observation links, and pending
links; follow-up questions (its own, and the one it may answer); changesets from its runs (pending,
split off, or already resolved) and their ops; briefing recap points; and the `origin_entry_id` and
`resolved_entry_id` of the tasks, blockers, and ideas its changesets created or resolved. Two schema
facts constrain the options: an extraction changeset must have a run (`CHECK ((source = 'extraction')
= (extraction_run_id IS NOT NULL))`), and the references from tasks, blockers, and ideas to entries
have no `ON DELETE` action.

## Options considered

1. **No deletes.** Keeps the audit trail whole; the user can't remove anything they wrote.
2. **Soft delete** (hidden entries). Every entry query must filter them, and the text stays.
3. **Hard delete of the entry with what came from it**, keeping the state already applied from it.

For the resolved changesets of its runs: keep them (a migration relaxing the CHECK so their run can
go), or delete them with the runs. For the references from tasks, blockers, and ideas: clear them in
the store's delete, or rebuild those tables with `ON DELETE SET NULL`.

## Decision

Option 3. `store.Tx.DeleteEntry` runs in one transaction and removes the entry with its runs,
observations (with their links and pending links), follow-up questions, and every changeset of its
runs with its ops, whatever its status; briefing recap points go by `ON DELETE CASCADE`. Changes
already applied from it stay: the tasks, blockers, and ideas lose their `origin_entry_id` (and a
blocker its `resolved_entry_id`). A question it answered becomes dismissed with no answer; entries
replying to its questions keep their text and lose `reply_to_question_id`.

Resolved changesets are deleted rather than kept: the record of which entry proposed a change has
little use once the entry is gone, and keeping them would need a migration and a changeset with no
source in every view of history. Clearing the references is reference cleanup, like deleting a task
clearing a briefing's next task (ADR 0002): it changes no field the user sees or edits, so it is
done in the store's delete rather than as a manual changeset, an exception to the rule that every
write to those tables goes through `internal/changeset`. The table rebuilds would give the same
result at much greater cost.

"Entries are immutable" still holds in the sense that matters: an entry's text is never edited, and
an entry goes whole or not at all.

## Consequences

- `DELETE /api/entries/{id}` (204) and an `entry.deleted` SSE event; open screens drop the entry or
  reload. Briefings of active projects are refreshed, since the entry may be in any recap.
- An entry can't be deleted while its run is running (`run_in_progress`), since the worker would then
  store results for a run that's gone. A pending run goes with the entry.
- A new table that references entries, runs, observations, questions, or changesets must extend
  `DeleteEntry`; `TestDeleteEntryRemovesWhatCameFromIt` checks for foreign key violations.
- Deleting a project with its entries (PLAN 1.5.13) calls `DeleteEntry` for each.
- The UI asks for confirmation, saying what goes and that applied changes stay.
