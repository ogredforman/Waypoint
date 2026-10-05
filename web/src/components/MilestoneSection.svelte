<script lang="ts">
  import type { Snippet } from 'svelte'
  import { ApiError, deleteMilestone, updateMilestone } from '../lib/api/client'
  import type { MilestoneStatus } from '../lib/api/enums.gen'
  import type { Milestone } from '../lib/api/types.gen'
  import { milestoneIsClosed, milestonePatch, milestoneStatusLabels, type MilestoneForm } from '../lib/milestones'
  import type { Direction } from '../lib/tasks'
  import Button from './ui/Button.svelte'
  import Input from './ui/Input.svelte'
  import Textarea from './ui/Textarea.svelte'

  let {
    milestone,
    canMoveUp,
    canMoveDown,
    moving = false,
    onchanged,
    ondeleted,
    onmove,
    children,
  }: {
    milestone: Milestone
    canMoveUp: boolean
    canMoveDown: boolean
    moving?: boolean
    onchanged: (m: Milestone) => void
    ondeleted: (id: number) => void
    onmove: (dir: Direction) => void
    // children are the milestone's tasks.
    children: Snippet
  } = $props()

  let editing = $state(false)
  let confirmingDelete = $state(false)
  let form = $state<MilestoneForm>({ title: '', description: '' })
  let saving = $state(false)
  let problems = $state<Record<string, string>>({})
  let notice = $state('')
  // A done or dropped milestone starts folded; unfolding sticks until the
  // section is rebuilt.
  let unfolded = $state(false)

  const closed = $derived(milestoneIsClosed(milestone))
  const open = $derived(!closed || unfolded)
  const idPrefix = $derived(`milestone-${milestone.id}`)

  function edit() {
    form = { title: milestone.title, description: milestone.description }
    problems = {}
    notice = ''
    confirmingDelete = false
    editing = true
  }

  async function save(e: SubmitEvent) {
    e.preventDefault()
    problems = {}
    notice = ''
    if (form.title.trim() === '') {
      problems = { title: 'Give it a short title.' }
      return
    }
    const patch = milestonePatch(milestone, form)
    if (patch === null) {
      editing = false
      return
    }
    saving = true
    try {
      onchanged(await updateMilestone(milestone.id, patch))
      editing = false
    } catch (err) {
      if (err instanceof ApiError && err.code === 'invalid' && err.problems.length > 0) {
        problems = Object.fromEntries(err.problems.map((p) => [p.field, p.problem]))
        if (err.problems.some((p) => p.field !== 'title' && p.field !== 'description')) notice = err.message
      } else {
        notice = err instanceof ApiError ? err.message : "Couldn't save the milestone."
      }
    } finally {
      saving = false
    }
  }

  async function setStatus(select: HTMLSelectElement) {
    const status = select.value as MilestoneStatus
    if (status === milestone.status) return
    notice = ''
    saving = true
    try {
      onchanged(await updateMilestone(milestone.id, { status }))
    } catch (err) {
      notice = err instanceof ApiError ? err.message : "Couldn't change the milestone's status."
      select.value = milestone.status
    } finally {
      saving = false
    }
  }

  async function remove() {
    notice = ''
    saving = true
    try {
      await deleteMilestone(milestone.id)
      ondeleted(milestone.id)
    } catch (err) {
      notice = err instanceof ApiError ? err.message : "Couldn't delete the milestone."
      saving = false
    }
  }
</script>

<section aria-labelledby="{idPrefix}-heading" class="space-y-2">
  {#if editing}
    <form class="space-y-3" onsubmit={save} novalidate>
      <div>
        <label for="{idPrefix}-title" class="text-sm font-medium text-ink">Milestone</label>
        <Input
          id="{idPrefix}-title"
          bind:value={form.title}
          autofocus
          autocomplete="off"
          aria-invalid={problems.title ? 'true' : undefined}
          aria-describedby={problems.title ? `${idPrefix}-title-problem` : undefined}
        />
        {#if problems.title}<p id="{idPrefix}-title-problem" class="mt-1 text-sm text-accent">{problems.title}</p>{/if}
      </div>
      <div>
        <label for="{idPrefix}-description" class="text-sm font-medium text-ink">What it means to reach it</label>
        <Textarea id="{idPrefix}-description" bind:value={form.description} rows={2} />
        {#if problems.description}<p class="mt-1 text-sm text-accent">{problems.description}</p>{/if}
      </div>
      {#if notice}<p class="text-sm text-accent" role="status">{notice}</p>{/if}
      <div class="flex gap-2">
        <Button type="submit" disabled={saving}>{saving ? 'Saving…' : 'Save'}</Button>
        <Button variant="quiet" onclick={() => (editing = false)}>Cancel</Button>
      </div>
    </form>
    <h3 id="{idPrefix}-heading" class="sr-only">{milestone.title}</h3>
  {:else}
    <div class="flex items-start gap-3">
      <div class="min-w-0 flex-1">
        <h3 id="{idPrefix}-heading" class={['text-base font-semibold', closed ? 'text-ink-muted' : 'text-ink']}>
          {milestone.title}
        </h3>
        {#if milestone.description}<p class="text-sm whitespace-pre-line text-ink-muted">{milestone.description}</p>{/if}
      </div>
      <div class="flex shrink-0 items-center gap-1">
        {#if closed}
          <Button variant="quiet" size="xs" aria-expanded={open} onclick={() => (unfolded = !unfolded)}>
            {open ? 'Hide tasks' : 'Show tasks'}
          </Button>
        {/if}
        <Button
          variant="quiet"
          size="xs"
          aria-label="Move milestone {milestone.title} up"
          disabled={!canMoveUp || moving}
          onclick={() => onmove('up')}
          class="disabled:invisible">↑</Button
        >
        <Button
          variant="quiet"
          size="xs"
          aria-label="Move milestone {milestone.title} down"
          disabled={!canMoveDown || moving}
          onclick={() => onmove('down')}
          class="disabled:invisible">↓</Button
        >
        <label for="{idPrefix}-status" class="sr-only">Status of milestone {milestone.title}</label>
        <select
          id="{idPrefix}-status"
          value={milestone.status}
          disabled={saving}
          onchange={(e) => setStatus(e.currentTarget)}
          class="rounded-control border border-line bg-canvas px-2 py-1 text-sm text-ink-muted focus:border-accent focus:outline-none"
        >
          {#each milestoneStatusLabels as o (o.status)}<option value={o.status}>{o.label}</option>{/each}
        </select>
        <Button variant="secondary" size="sm" aria-label="Edit milestone {milestone.title}" onclick={edit}>Edit</Button>
        <Button variant="danger" size="sm" aria-label="Delete milestone {milestone.title}" onclick={() => (confirmingDelete = true)}
          >Delete</Button
        >
      </div>
    </div>
    {#if confirmingDelete}
      <div class="flex flex-wrap items-center gap-2 text-sm" role="group" aria-label="Confirm delete">
        <span class="text-ink-muted">Delete this milestone? Its tasks stay in the project, with no milestone.</span>
        <Button variant="danger" size="sm" disabled={saving} onclick={remove}>Delete milestone</Button>
        <Button variant="quiet" size="sm" onclick={() => (confirmingDelete = false)}>Keep it</Button>
      </div>
    {/if}
    {#if notice}<p class="text-sm text-accent" role="status">{notice}</p>{/if}
  {/if}
  {#if open}{@render children()}{/if}
</section>
