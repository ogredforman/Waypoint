<script lang="ts">
  import { ApiError, deleteJSON, patchJSON, postJSON } from '../lib/api/client'
  import type { MoveTaskRequest, Task } from '../lib/api/types.gen'
  import { paths } from '../lib/router'
  import { newStepRequest, sortSteps, statusLabels, stepStatus, type Direction } from '../lib/tasks'
  import Button from './ui/Button.svelte'
  import Input from './ui/Input.svelte'
  import Panel from './ui/Panel.svelte'

  // steps are the task's direct subtasks, in order: a checklist the user
  // ticks off, reorders, adds to, and deletes from.
  let { task, steps = $bindable() }: { task: Task; steps: Task[] } = $props()

  let title = $state('')
  let adding = $state(false)
  let addProblem = $state('')
  let busy = $state(false)
  let notice = $state('')
  let confirming = $state<number | null>(null)

  function put(changed: Task[]) {
    const byId = new Map(changed.map((t) => [t.id, t]))
    steps = sortSteps(steps.map((s) => byId.get(s.id) ?? s))
  }

  async function act(fallback: string, fn: () => Promise<void>) {
    notice = ''
    busy = true
    try {
      await fn()
    } catch (err) {
      notice = err instanceof ApiError ? err.message : fallback
    } finally {
      busy = false
    }
  }

  function tick(step: Task, box: HTMLInputElement) {
    const status = stepStatus(box.checked)
    act("Couldn't change the step.", async () => {
      try {
        put([await patchJSON<Task>(`/api/tasks/${step.id}`, { status })])
      } catch (err) {
        box.checked = step.status === 'done' // show the status the step still has
        throw err
      }
    })
  }

  function move(step: Task, direction: Direction) {
    act("Couldn't move the step.", async () => {
      put(await postJSON<Task[]>(`/api/tasks/${step.id}/move`, { direction } satisfies MoveTaskRequest))
    })
  }

  function remove(step: Task) {
    act("Couldn't delete the step.", async () => {
      await deleteJSON(`/api/tasks/${step.id}`)
      steps = steps.filter((s) => s.id !== step.id)
      confirming = null
    })
  }

  async function add(e: SubmitEvent) {
    e.preventDefault()
    addProblem = ''
    if (title.trim() === '') {
      addProblem = 'Give the step a short title.'
      return
    }
    adding = true
    try {
      const s = await postJSON<Task>(`/api/projects/${task.project_id}/tasks`, newStepRequest(title, task.id))
      steps = sortSteps([...steps, s])
      title = ''
    } catch (err) {
      addProblem = err instanceof ApiError ? err.message : "Couldn't add the step."
    } finally {
      adding = false
    }
  }

  // A step that is in progress or dropped says so; todo and done show in the checkbox.
  function statusNote(s: Task): string {
    return s.status === 'in_progress' || s.status === 'dropped' ? (statusLabels.find((l) => l.status === s.status)?.label ?? '') : ''
  }
</script>

<section class="space-y-2" aria-labelledby="task-steps">
  <h2 id="task-steps" class="font-medium text-ink">Steps</h2>
  {#if steps.length === 0}
    <p class="text-sm text-ink-muted">No steps yet. If this feels big, what's the first small step?</p>
  {:else}
    <Panel>
      <ol class="divide-y divide-line" aria-label="Steps">
        {#each steps as s, i (s.id)}
          {@const closed = s.status === 'done' || s.status === 'dropped'}
          <li class="px-4 py-2" data-step-id={s.id}>
            <div class="flex items-center gap-3">
              <input
                id="step-{s.id}-done"
                type="checkbox"
                checked={s.status === 'done'}
                disabled={busy}
                onchange={(e) => tick(s, e.currentTarget)}
                class="size-4 accent-accent"
              />
              <label for="step-{s.id}-done" class="sr-only">Done: {s.title}</label>
              <a href={paths.task(s.id)} class={['min-w-0 flex-1 hover:text-accent', closed ? 'text-ink-muted line-through' : 'text-ink']}>{s.title}</a>
              {#if statusNote(s)}<span class="text-xs text-ink-muted">{statusNote(s)}</span>{/if}
              <div class="flex shrink-0 items-center gap-1">
                <Button
                  variant="quiet"
                  size="xs"
                  aria-label="Move {s.title} up"
                  disabled={i === 0 || busy}
                  onclick={() => move(s, 'up')}
                  class="disabled:invisible">↑</Button
                >
                <Button
                  variant="quiet"
                  size="xs"
                  aria-label="Move {s.title} down"
                  disabled={i === steps.length - 1 || busy}
                  onclick={() => move(s, 'down')}
                  class="disabled:invisible">↓</Button
                >
                <Button variant="danger" size="sm" aria-label="Delete {s.title}" onclick={() => (confirming = s.id)}>Delete</Button>
              </div>
            </div>
            {#if confirming === s.id}
              <div class="mt-2 flex items-center gap-2 text-sm" role="group" aria-label="Confirm delete">
                <span class="text-ink-muted">Delete this step?</span>
                <Button variant="danger" size="sm" disabled={busy} onclick={() => remove(s)}>Delete step</Button>
                <Button variant="quiet" size="sm" onclick={() => (confirming = null)}>Keep it</Button>
              </div>
            {/if}
          </li>
        {/each}
      </ol>
    </Panel>
  {/if}
  {#if notice}<p class="text-sm text-accent" role="status">{notice}</p>{/if}
  <form class="flex flex-wrap items-start gap-2" onsubmit={add} novalidate>
    <div class="min-w-48 flex-1">
      <label for="task-step-title" class="sr-only">New step</label>
      <Input
        id="task-step-title"
        bind:value={title}
        placeholder="Add a step"
        autocomplete="off"
        aria-invalid={addProblem ? 'true' : undefined}
        aria-describedby={addProblem ? 'task-step-problem' : undefined}
      />
    </div>
    <Button type="submit" class="mt-1" disabled={adding}>{adding ? 'Adding…' : 'Add step'}</Button>
  </form>
  {#if addProblem}<p id="task-step-problem" class="text-sm text-accent">{addProblem}</p>{/if}
</section>
