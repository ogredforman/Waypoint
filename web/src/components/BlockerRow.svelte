<script lang="ts">
  import { ApiError, deleteJSON, patchJSON } from '../lib/api/client'
  import type { Blocker, Task } from '../lib/api/types.gen'
  import { blockerTargets, moveBlockerRequest } from '../lib/blockers'
  import { paths } from '../lib/router'
  import Button from './ui/Button.svelte'

  let {
    blocker,
    tasks,
    showTask = true,
    onchanged,
    ondeleted,
  }: {
    blocker: Blocker
    // The project's tasks. Given, an open blocker can be moved between them
    // and the whole project.
    tasks?: Task[]
    // Whether to link to the task the blocker is on.
    showTask?: boolean
    onchanged: (b: Blocker) => void
    ondeleted: (id: number) => void
  } = $props()

  let confirmingDelete = $state(false)
  let saving = $state(false)
  let notice = $state('')

  const resolved = $derived(blocker.status === 'resolved')
  const targets = $derived(tasks && !resolved ? blockerTargets(tasks, blocker) : null)
  const taskTitle = $derived(tasks?.find((t) => t.id === blocker.task_id)?.title)

  const selectClass =
    'max-w-64 rounded-control border border-line bg-canvas px-2 py-1 text-sm text-ink transition focus:border-accent focus:shadow-focus focus:outline-none'

  async function resolve() {
    notice = ''
    saving = true
    try {
      onchanged(await patchJSON<Blocker>(`/api/blockers/${blocker.id}`, { status: 'resolved' }))
    } catch (err) {
      notice = err instanceof ApiError ? err.message : "Couldn't resolve the blocker."
    } finally {
      saving = false
    }
  }

  async function move(e: Event & { currentTarget: HTMLSelectElement }) {
    const select = e.currentTarget
    const value = select.value
    notice = ''
    saving = true
    try {
      onchanged(await patchJSON<Blocker>(`/api/blockers/${blocker.id}`, moveBlockerRequest(value === '' ? '' : Number(value))))
    } catch (err) {
      select.value = String(blocker.task_id ?? '')
      notice = err instanceof ApiError ? err.message : "Couldn't move the blocker."
    } finally {
      saving = false
    }
  }

  async function remove() {
    notice = ''
    saving = true
    try {
      await deleteJSON(`/api/blockers/${blocker.id}`)
      ondeleted(blocker.id)
    } catch (err) {
      notice = err instanceof ApiError ? err.message : "Couldn't delete the blocker."
      saving = false
    }
  }
</script>

<li class="px-4 py-3" data-blocker-id={blocker.id}>
  <div class="flex items-start gap-3">
    <div class="min-w-0 flex-1">
      <p class={['whitespace-pre-line', resolved ? 'text-ink-muted' : 'text-ink']}>{blocker.description}</p>
      {#if targets}
        <label class="mt-1 flex items-center gap-2 text-sm text-ink-muted">
          Blocks
          <select class={selectClass} value={String(blocker.task_id ?? '')} disabled={saving} onchange={move}>
            <option value="">Whole project</option>
            {#each targets as t (t.id)}<option value={String(t.id)}>{t.title}</option>{/each}
          </select>
        </label>
        {#if showTask && blocker.task_id !== null}
          <a href={paths.task(blocker.task_id)} class="text-sm text-ink-muted hover:text-accent">Open the task →</a>
        {/if}
      {:else if showTask && blocker.task_id !== null}
        <a href={paths.task(blocker.task_id)} class="text-sm text-ink-muted hover:text-accent">Blocks {taskTitle ?? 'a task'} →</a>
      {/if}
    </div>
    <div class="flex shrink-0 items-center gap-1">
      {#if !resolved}
        <Button variant="secondary" size="sm" disabled={saving} onclick={resolve}>Resolve</Button>
      {/if}
      <Button variant="danger" size="sm" onclick={() => (confirmingDelete = true)}>Delete</Button>
    </div>
  </div>
  {#if confirmingDelete}
    <div class="mt-2 flex items-center gap-2 text-sm" role="group" aria-label="Confirm delete">
      <span class="text-ink-muted">Delete this blocker?</span>
      <Button variant="danger" size="sm" disabled={saving} onclick={remove}>Delete blocker</Button>
      <Button variant="quiet" size="sm" onclick={() => (confirmingDelete = false)}>Keep it</Button>
    </div>
  {/if}
  {#if notice}<p class="mt-2 text-sm text-accent" role="status">{notice}</p>{/if}
</li>
