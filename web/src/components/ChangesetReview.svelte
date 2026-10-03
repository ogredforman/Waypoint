<script lang="ts">
  import { ApiError, postJSON } from '../lib/api/client'
  import type { ApplyRequest, ApplyResponse, ChangeOp, Changeset, Entry, Project } from '../lib/api/types.gen'
  import {
    accept,
    acceptRest,
    applyRequest,
    describe,
    guessedProject,
    needsProject,
    noProject,
    payloadOf,
    reject,
    rejectedUse,
    undecide,
    type Choices,
    type Payload,
  } from '../lib/review'
  import { pickable } from '../lib/projects'
  import { loadEntry } from '../lib/timeline.svelte'
  import ChangeOpRow from './ChangeOpRow.svelte'
  import Button from './ui/Button.svelte'

  // A pending changeset under the entry that produced it (SPEC §7.1 step 7). The user decides each
  // op, then applies; undecided ops stay pending for later, in a changeset split off this one. For an
  // entry with no project, a picker shows the project the new tasks, blockers, and ideas were put in
  // and lets the user choose another one, or none.

  let {
    changeset,
    entry,
    projects,
    onnotice,
  }: { changeset: Changeset; entry: Entry; projects: Project[]; onnotice: (message: string) => void } = $props()

  const ops = $derived(changeset.ops.filter((o) => o.status === 'pending'))
  const guessed = $derived(guessedProject(entry, changeset))
  let choices = $state<Choices>({})
  // undefined until the user picks: then a project ID, or null for none.
  let picked = $state<number | null | undefined>(undefined)
  let busy = $state(false)
  let notice = $state('')

  const target = $derived(picked === undefined ? guessed : picked)
  const choosable = $derived(projects.filter((p) => pickable(p, guessed)))
  const decidedCount = $derived(ops.filter((o) => choices[o.id]).length)

  // whyNotAccept explains why an op can't be accepted right now, or is null if it can.
  function whyNotAccept(op: ChangeOp): string | null {
    if (guessed !== null && target === null && needsProject(op, guessed)) {
      return 'Tasks and blockers need a project. Pick one above to accept this.'
    }
    const dep = rejectedUse(ops, choices, op.id)
    return dep ? `This needs “${describe(dep, ops, choices)}”, which you rejected. Undo that to accept this.` : null
  }

  function pick(value: string) {
    if (guessed === null) return
    picked = value === '' ? null : Number(value)
    if (picked === null) choices = noProject(ops, choices, guessed)
  }

  async function apply(all: boolean) {
    if (busy) return
    const final = all ? acceptRest(ops, choices) : choices
    const move = guessed === null ? null : { from: guessed, to: target ?? null }
    const req: ApplyRequest = { ops: applyRequest(ops, final, move) }
    if (req.ops.length === 0) return
    busy = true
    notice = ''
    try {
      const res = await postJSON<ApplyResponse>(`/api/changesets/${changeset.id}/apply`, req)
      const held = res.held_by_cap.length
      if (held > 0) {
        onnotice(
          held === 1
            ? "1 change is waiting below: it would make one more project active than you've set as your limit."
            : `${held} changes are waiting below: they would make more projects active than you've set as your limit.`,
        )
      }
      await loadEntry(entry.id)
    } catch (err) {
      notice = err instanceof ApiError ? err.message : "Couldn't apply these changes. Nothing was changed."
      if (err instanceof ApiError && err.code === 'not_pending') await loadEntry(entry.id)
    } finally {
      busy = false
    }
  }
</script>

<section class="space-y-3 border-l-2 border-accent/50 pl-3" aria-label="Proposed changes">
  <p class="text-xs text-ink-muted">{ops.length === 1 ? '1 proposed change' : `${ops.length} proposed changes`}</p>

  {#if guessed !== null}
    <div class="flex flex-wrap items-center gap-2 text-sm">
      <label for="cs-{changeset.id}-project" class="text-ink-muted">Project</label>
      <select
        id="cs-{changeset.id}-project"
        class="rounded-control border border-line bg-surface px-2 py-1 text-sm text-ink"
        value={target === null ? '' : String(target)}
        onchange={(e) => pick(e.currentTarget.value)}
        disabled={busy}
      >
        {#each choosable as p (p.id)}
          <option value={String(p.id)}>{p.title}</option>
        {/each}
        {#if !choosable.some((p) => p.id === guessed)}
          <option value={String(guessed)}>The guessed project</option>
        {/if}
        <option value="">No project</option>
      </select>
      {#if target === guessed}
        <span class="text-xs text-ink-muted">(guessed)</span>
      {:else if target === null}
        <span class="text-xs text-ink-muted">New tasks and blockers need a project, so they're set aside.</span>
      {/if}
    </div>
  {/if}

  <ul class="divide-y divide-line">
    {#each ops as op (op.id)}
      <ChangeOpRow
        {op}
        sentence={describe(op, ops, choices)}
        choice={choices[op.id]}
        payload={payloadOf(op, choices)}
        blockedBy={whyNotAccept(op)}
        disabled={busy}
        onaccept={() => (choices = accept(ops, choices, op.id))}
        onreject={() => (choices = reject(ops, choices, op.id))}
        onundo={() => (choices = undecide(choices, op.id))}
        onedit={(payload: Payload) => (choices = accept(ops, { ...choices, [op.id]: { action: 'edit', payload } }, op.id))}
      />
    {/each}
  </ul>

  <div class="flex flex-wrap items-center gap-2">
    <Button onclick={() => apply(false)} disabled={busy || decidedCount === 0}>{busy ? 'Applying…' : 'Apply'}</Button>
    <Button variant="secondary" onclick={() => apply(true)} disabled={busy}>Accept all</Button>
    {#if decidedCount > 0 && decidedCount < ops.length}
      <span class="text-xs text-ink-muted">The rest stay here for later.</span>
    {/if}
  </div>
  {#if notice}
    <p class="text-xs text-ink-muted" aria-live="polite">{notice}</p>
  {/if}
</section>
