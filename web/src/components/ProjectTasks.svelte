<script lang="ts">
  import { onMount } from 'svelte'
  import { ApiError, createMilestone, getJSON, listMilestones, moveMilestone, postJSON } from '../lib/api/client'
  import { latest, liveReload } from '../lib/live'
  import { energyValues, taskSizeValues, type Energy, type TaskSize } from '../lib/api/enums.gen'
  import type { Milestone, MoveTaskRequest, Task } from '../lib/api/types.gen'
  import { milestoneLabel, milestoneSections, sortMilestones, type MilestoneSection as Section } from '../lib/milestones'
  import { nestTasks, newTaskRequest, type Direction } from '../lib/tasks'
  import MilestoneSection from './MilestoneSection.svelte'
  import TaskRow from './TaskRow.svelte'
  import Button from './ui/Button.svelte'
  import Input from './ui/Input.svelte'
  import Panel from './ui/Panel.svelte'

  let { projectId }: { projectId: number } = $props()

  let tasks = $state<Task[] | null>(null)
  let milestones = $state<Milestone[]>([])
  let loadError = $state('')
  let title = $state('')
  let size = $state<TaskSize | ''>('')
  let energy = $state<Energy | ''>('')
  let milestone = $state<number | ''>('')
  let adding = $state(false)
  let addProblem = $state('')
  let moving = $state(false)
  let moveNotice = $state('')
  let milestoneTitle = $state('')
  let addingMilestone = $state(false)
  let milestoneProblem = $state('')

  // The field look without full width, to sit in the add row.
  const selectClass =
    'mt-1 rounded-control border border-line bg-canvas px-3 py-2 text-ink transition focus:border-accent focus:shadow-focus focus:outline-none'

  const tree = $derived(nestTasks(tasks ?? []))
  const ordered = $derived(sortMilestones(milestones))
  const sections = $derived(milestoneSections(tree, milestones))

  const loads = latest()

  async function fetchAll(): Promise<[Task[], Milestone[]]> {
    return Promise.all([getJSON<Task[]>(`/api/projects/${projectId}/tasks`), listMilestones(projectId)])
  }

  async function load() {
    const current = loads()
    try {
      const [list, ms] = await fetchAll()
      if (!current()) return
      tasks = list
      milestones = ms
      loadError = ''
    } catch (err) {
      if (current()) loadError = err instanceof ApiError ? err.message : "Couldn't load the tasks."
    }
  }

  // reload shows what the server has after a failed move, keeping the list
  // if that fails too; the notice already says the move failed.
  async function reload() {
    const current = loads()
    try {
      const [list, ms] = await fetchAll()
      if (!current()) return
      tasks = list
      milestones = ms
    } catch {
      // Keep the list as it is.
    }
  }

  function replace(t: Task) {
    tasks = (tasks ?? []).map((x) => (x.id === t.id ? t : x))
  }

  function removed(id: number) {
    tasks = (tasks ?? []).filter((x) => x.id !== id)
  }

  function replaceMilestone(m: Milestone) {
    milestones = milestones.map((x) => (x.id === m.id ? m : x))
  }

  // A deleted milestone's tasks stay, with no milestone.
  function removedMilestone(id: number) {
    milestones = milestones.filter((x) => x.id !== id)
    tasks = (tasks ?? []).map((t) => (t.milestone_id === id ? { ...t, milestone_id: null } : t))
    if (milestone === id) milestone = ''
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
      const t = await postJSON<Task>(`/api/projects/${projectId}/tasks`, newTaskRequest({ title, size, energy, milestone }))
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

  async function addMilestone(e: SubmitEvent) {
    e.preventDefault()
    milestoneProblem = ''
    if (milestoneTitle.trim() === '') {
      milestoneProblem = 'Give it a short title.'
      return
    }
    addingMilestone = true
    try {
      const m = await createMilestone(projectId, { title: milestoneTitle.trim() })
      milestones = [...milestones, m]
      milestoneTitle = ''
    } catch (err) {
      milestoneProblem = err instanceof ApiError ? err.message : "Couldn't add the milestone."
    } finally {
      addingMilestone = false
    }
  }

  // move swaps the task with its neighbour in one request and puts the
  // tasks the server moved in place. If it fails, the list reloads.
  async function move(t: Task, dir: Direction) {
    moveNotice = ''
    moving = true
    try {
      const moved = await postJSON<Task[]>(`/api/tasks/${t.id}/move`, { direction: dir } satisfies MoveTaskRequest)
      moved.forEach(replace)
    } catch (err) {
      moveNotice = err instanceof ApiError ? err.message : "Couldn't move the task."
      await reload()
    } finally {
      moving = false
    }
  }

  async function moveSection(m: Milestone, dir: Direction) {
    moveNotice = ''
    moving = true
    try {
      ;(await moveMilestone(m.id, dir)).forEach(replaceMilestone)
    } catch (err) {
      moveNotice = err instanceof ApiError ? err.message : "Couldn't move the milestone."
      await reload()
    } finally {
      moving = false
    }
  }

  // label names a status group's list, with its milestone when there are any.
  function label(s: Section, group: string): string {
    if (milestones.length === 0) return group
    return `${s.milestone?.title ?? 'No milestone'}: ${group}`
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
      {milestones}
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

<!-- groups shows a section's tasks by status; headed h3 alone, h4 under a milestone. -->
{#snippet groups(s: Section)}
  {#each s.groups as g (g.status)}
    <div>
      <svelte:element
        this={milestones.length > 0 ? 'h4' : 'h3'}
        class="mb-1 text-label font-medium tracking-wide text-ink-muted uppercase">{g.label}</svelte:element
      >
      <Panel>
        <ul class="divide-y divide-line" aria-label={label(s, g.label)}>
          {@render rows(g.tasks)}
        </ul>
      </Panel>
    </div>
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
    {#if milestones.length > 0}
      <label for="new-task-milestone" class="sr-only">Milestone</label>
      <select id="new-task-milestone" bind:value={milestone} class={selectClass}>
        <option value="">No milestone</option>
        {#each ordered as m (m.id)}<option value={m.id}>{milestoneLabel(m)}</option>{/each}
      </select>
    {/if}
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
  {:else}
    {#if tasks.length === 0 && milestones.length === 0}
      <p class="text-ink-muted">No tasks yet. What's one small thing you could start with?</p>
    {/if}
    {#each sections as s, i (s.milestone?.id ?? 'none')}
      {#if s.milestone}
        {@const m = s.milestone}
        <MilestoneSection
          milestone={m}
          canMoveUp={i > 0}
          canMoveDown={i < ordered.length - 1}
          {moving}
          onchanged={replaceMilestone}
          ondeleted={removedMilestone}
          onmove={(dir) => moveSection(m, dir)}
        >
          {#if s.groups.length === 0}
            <p class="text-sm text-ink-muted">No tasks in this milestone yet.</p>
          {:else}
            {@render groups(s)}
          {/if}
        </MilestoneSection>
      {:else if milestones.length > 0}
        <section aria-labelledby="no-milestone-heading" class="space-y-2">
          <h3 id="no-milestone-heading" class="text-base font-semibold text-ink">No milestone</h3>
          {@render groups(s)}
        </section>
      {:else}
        {@render groups(s)}
      {/if}
    {/each}

    <form class="flex flex-wrap items-start gap-2" onsubmit={addMilestone} novalidate>
      <div class="min-w-48 flex-1">
        <label for="new-milestone-title" class="sr-only">New milestone</label>
        <Input
          id="new-milestone-title"
          bind:value={milestoneTitle}
          placeholder="Add a milestone, an outcome to group tasks under"
          autocomplete="off"
          aria-invalid={milestoneProblem ? 'true' : undefined}
          aria-describedby={milestoneProblem ? 'new-milestone-problem' : undefined}
        />
      </div>
      <Button type="submit" variant="secondary" class="mt-1" disabled={addingMilestone}
        >{addingMilestone ? 'Adding…' : 'Add milestone'}</Button
      >
    </form>
    {#if milestoneProblem}<p id="new-milestone-problem" class="text-sm text-accent">{milestoneProblem}</p>{/if}
  {/if}
</section>
