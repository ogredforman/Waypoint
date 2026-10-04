<script lang="ts">
  import { onMount } from 'svelte'
  import { ApiError, getJSON, patchJSON, postJSON } from '../lib/api/client'
  import { latest, liveReload } from '../lib/live'
  import { energyValues, taskSizeValues, type Energy, type TaskSize } from '../lib/api/enums.gen'
  import type { MoveTaskRequest, Task } from '../lib/api/types.gen'
  import { nestTasks, newTaskRequest, type Direction } from '../lib/tasks'
  import TaskRow from './TaskRow.svelte'
  import Button from './ui/Button.svelte'
  import Input from './ui/Input.svelte'
  import Panel from './ui/Panel.svelte'
  
  let { projectId }: { projectId: number } = $props()

  let tasks = $state<Task[] | null>(null)
  let loadError = $state('')
  let title = $state('')
  let size = $state<TaskSize | ''>('')
  let energy = $state<Energy | ''>('')
  let adding = $state(false)
  let addProblem = $state('')
  let moving = $state(false)
  let moveNotice = $state('')

  // The field look without full width, to sit in the add row.
  const selectClass =
    'mt-1 rounded-control border border-line bg-canvas px-3 py-2 text-ink transition focus:border-accent focus:shadow-focus focus:outline-none'

  const tree = $derived(nestTasks(tasks ?? []))
  const groups = $derived(tree.groups)

  const loads = latest()

  async function load() {
    const current = loads()
    try {
      const list = await getJSON<Task[]>(`/api/projects/${projectId}/tasks`)
      if (!current()) return
      tasks = list
      loadError = ''
    } catch (err) {
      if (current()) loadError = err instanceof ApiError ? err.message : "Couldn't load the tasks."
    }
  }

  function replace(t: Task) {
    tasks = (tasks ?? []).map((x) => (x.id === t.id ? t : x))
  }

  function removed(id: number) {
    tasks = (tasks ?? []).filter((x) => x.id !== id)
  }

  async function add(e: SubmitEvent) {
    e.preventDefault()
    addProblem = ''
    if (title.trim() === '') {
      addProblem = 'Give it a short title.'
      return
    }
    adding = true
    try {
      const t = await postJSON<Task>(`/api/projects/${projectId}/tasks`, newTaskRequest({ title, size, energy }))
      tasks = [...(tasks ?? []), t]
      title = ''
      size = ''
      energy = ''
    } catch (err) {
      addProblem = err instanceof ApiError ? err.message : "Couldn't add the task."
    } finally {
      adding = false
    }
  }

  // move swaps the task with its neighbour in one request and puts the
  // tasks the server moved in place. If it fails, the list reloads to show
  // what the server has, keeping the list if that fails too.
  async function move(t: Task, dir: Direction) {
    moveNotice = ''
    moving = true
    try {
      const moved = await postJSON<Task[]>(`/api/tasks/${t.id}/move`, { direction: dir } satisfies MoveTaskRequest)
      moved.forEach(replace)
    } catch (err) {
      moveNotice = err instanceof ApiError ? err.message : "Couldn't move the task."
      const current = loads()
      try {
        const list = await getJSON<Task[]>(`/api/projects/${projectId}/tasks`)
        if (current()) tasks = list
      } catch {
        // Keep the list as it is; the notice already says the move failed.
      }
    } finally {
      moving = false
    }
  }

  onMount(() => {
    load()
    return liveReload(load, () => projectId)
  })
</script>

<!-- rows lists sibling tasks, each with its steps nested under it. -->
{#snippet rows(list: Task[])}
  {#each list as t, i (t.id)}
    {@const steps = tree.steps.get(t.id) ?? []}
    <TaskRow
      task={t}
      canMoveUp={i > 0}
      canMoveDown={i < list.length - 1}
      {moving}
      onchanged={replace}
      ondeleted={removed}
      onmove={(dir) => move(t, dir)}
    >
      {#if steps.length > 0}
        <ul class="mt-2 ml-4 divide-y divide-line border-l border-line" aria-label="Steps of {t.title}">
          {@render rows(steps)}
        </ul>
      {/if}
    </TaskRow>
  {/each}
{/snippet}

<section aria-labelledby="tasks-heading" class="space-y-3">
  <h2 id="tasks-heading" class="text-lg font-semibold">Tasks</h2>

  <form class="flex flex-wrap items-start gap-2" onsubmit={add} novalidate>
    <div class="min-w-48 flex-1">
      <label for="new-task-title" class="sr-only">New task</label>
      <Input
        id="new-task-title"
        bind:value={title}
        placeholder="Add a task"
        autocomplete="off"
        aria-invalid={addProblem ? 'true' : undefined}
        aria-describedby={addProblem ? 'new-task-problem' : undefined}
      />
    </div>
    <label for="new-task-size" class="sr-only">Size</label>
    <select id="new-task-size" bind:value={size} class={selectClass}>
      <option value="">Size</option>
      {#each taskSizeValues as s (s)}<option value={s}>{s}</option>{/each}
    </select>
    <label for="new-task-energy" class="sr-only">Energy</label>
    <select id="new-task-energy" bind:value={energy} class={selectClass}>
      <option value="">Energy</option>
      {#each energyValues as en (en)}<option value={en}>{en}</option>{/each}
    </select>
    <Button type="submit" class="mt-1" disabled={adding}>{adding ? 'Adding…' : 'Add'}</Button>
  </form>
  {#if addProblem}<p id="new-task-problem" class="text-sm text-accent">{addProblem}</p>{/if}
  {#if moveNotice}<p class="text-sm text-accent" role="status">{moveNotice}</p>{/if}

  {#if loadError}
    <p class="text-ink-muted">
      {loadError}
      <Button variant="quiet" size="xs" class="ml-1" onclick={load}>Try again</Button>
    </p>
  {:else if tasks === null}
    <p class="text-ink-muted">Loading…</p>
  {:else if groups.length === 0}
    <p class="text-ink-muted">No tasks yet. What's one small thing you could start with?</p>
  {:else}
    {#each groups as g (g.status)}
      <div>
        <h3 class="mb-1 text-label font-medium tracking-wide text-ink-muted uppercase">{g.label}</h3>
        <Panel>
          <ul class="divide-y divide-line" aria-label={g.label}>
            {@render rows(g.tasks)}
          </ul>
        </Panel>
      </div>
    {/each}
  {/if}
</section>
