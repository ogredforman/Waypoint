<script lang="ts">
  import { onMount } from 'svelte'
  import { ApiError, getJSON, postJSON } from '../lib/api/client'
  import { latest, liveReload } from '../lib/live'
  import type { Blocker, Task } from '../lib/api/types.gen'
  import { newBlockerRequest, openTasks, splitBlockers } from '../lib/blockers'
  import BlockerRow from './BlockerRow.svelte'
  import Button from './ui/Button.svelte'
  import Input from './ui/Input.svelte'
  import Panel from './ui/Panel.svelte'

  let { projectId }: { projectId: number } = $props()

  let blockers = $state<Blocker[] | null>(null)
  let loadError = $state('')
  let tasks = $state<Task[]>([]) // all of the project's tasks
  let description = $state('')
  let taskId = $state<number | ''>('')
  let adding = $state(false)
  let addProblem = $state('')

  // The field look without full width, to sit in the add row.
  const selectClass =
    'mt-1 max-w-64 rounded-control border border-line bg-canvas px-3 py-2 text-ink transition focus:border-accent focus:shadow-focus focus:outline-none'

  const pickable = $derived(openTasks(tasks))
  const groups = $derived(blockers === null ? null : splitBlockers(blockers))

  const loads = latest()
  const taskLoads = latest()

  // loadTasks reads the project's tasks for the pickers. If it fails, they
  // keep what they had; a blocker can still go on the project alone.
  async function loadTasks() {
    const current = taskLoads()
    try {
      const list = await getJSON<Task[]>(`/api/projects/${projectId}/tasks`)
      if (!current()) return
      tasks = list
      if (taskId !== '' && !openTasks(list).some((t) => t.id === taskId)) taskId = ''
    } catch {
      // Keep the picker as it is.
    }
  }

  async function load() {
    loadTasks()
    const current = loads()
    try {
      const list = await getJSON<Blocker[]>(`/api/projects/${projectId}/blockers`)
      if (!current()) return
      blockers = list
      loadError = ''
    } catch (err) {
      if (current()) loadError = err instanceof ApiError ? err.message : "Couldn't load the blockers."
    }
  }

  function replace(b: Blocker) {
    blockers = (blockers ?? []).map((x) => (x.id === b.id ? b : x))
  }

  function removed(id: number) {
    blockers = (blockers ?? []).filter((x) => x.id !== id)
  }

  async function add(e: SubmitEvent) {
    e.preventDefault()
    addProblem = ''
    if (description.trim() === '') {
      addProblem = "Say what's in the way."
      return
    }
    adding = true
    try {
      const b = await postJSON<Blocker>(`/api/projects/${projectId}/blockers`, newBlockerRequest(description, taskId))
      blockers = [...(blockers ?? []), b]
      description = ''
      taskId = ''
    } catch (err) {
      addProblem = err instanceof ApiError ? err.message : "Couldn't add the blocker."
    } finally {
      adding = false
    }
  }

  onMount(() => {
    load()
    return liveReload(load, () => projectId)
  })
</script>

<section aria-labelledby="blockers-heading" class="space-y-3">
  <h2 id="blockers-heading" class="text-lg font-semibold">Blockers</h2>

  <form class="flex flex-wrap items-start gap-2" onsubmit={add} novalidate>
    <div class="min-w-48 flex-1">
      <label for="new-blocker-description" class="sr-only">New blocker</label>
      <Input
        id="new-blocker-description"
        bind:value={description}
        placeholder="Add a blocker"
        autocomplete="off"
        aria-invalid={addProblem ? 'true' : undefined}
        aria-describedby={addProblem ? 'new-blocker-problem' : undefined}
      />
    </div>
    <label for="new-blocker-task" class="sr-only">Blocks task (optional)</label>
    <select id="new-blocker-task" bind:value={taskId} class={selectClass}>
      <option value="">No task</option>
      {#each pickable as t (t.id)}<option value={t.id}>{t.title}</option>{/each}
    </select>
    <Button type="submit" class="mt-1" disabled={adding}>{adding ? 'Adding…' : 'Add'}</Button>
  </form>
  {#if addProblem}<p id="new-blocker-problem" class="text-sm text-accent">{addProblem}</p>{/if}

  {#if loadError}
    <p class="text-ink-muted">
      {loadError}
      <Button variant="quiet" size="xs" class="ml-1" onclick={load}>Try again</Button>
    </p>
  {:else if groups === null}
    <p class="text-ink-muted">Loading…</p>
  {:else}
    {#if groups.open.length === 0}
      <p class="text-ink-muted">Nothing in the way.</p>
    {:else}
      <div>
        <h3 class="mb-1 text-label font-medium tracking-wide text-ink-muted uppercase">Open</h3>
        <Panel>
          <ul class="divide-y divide-line" aria-label="Open">
            {#each groups.open as b (b.id)}
              <BlockerRow blocker={b} {tasks} onchanged={replace} ondeleted={removed} />
            {/each}
          </ul>
        </Panel>
      </div>
    {/if}
    {#if groups.resolved.length > 0}
      <div>
        <h3 class="mb-1 text-label font-medium tracking-wide text-ink-muted uppercase">Resolved</h3>
        <Panel>
          <ul class="divide-y divide-line" aria-label="Resolved">
            {#each groups.resolved as b (b.id)}
              <BlockerRow blocker={b} {tasks} onchanged={replace} ondeleted={removed} />
            {/each}
          </ul>
        </Panel>
      </div>
    {/if}
  {/if}
</section>
