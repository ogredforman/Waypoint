<script lang="ts">
  import type { Snippet } from 'svelte'
  import { ApiError, deleteJSON, patchJSON } from '../lib/api/client'
  import { energyValues, taskSizeValues, type TaskStatus } from '../lib/api/enums.gen'
  import type { Task } from '../lib/api/types.gen'
  import { paths } from '../lib/router'
  import { statusLabels, taskFields, taskPatch, type Direction, type TaskForm } from '../lib/tasks'
  import Button from './ui/Button.svelte'
  import Input from './ui/Input.svelte'
  import Textarea from './ui/Textarea.svelte'
  import { fieldClass } from './ui/field'

  let {
    task,
    canMoveUp,
    canMoveDown,
    moving = false,
    onchanged,
    ondeleted,
    onmove,
    children,
  }: {
    task: Task
    canMoveUp: boolean
    canMoveDown: boolean
    moving?: boolean
    onchanged: (t: Task) => void
    ondeleted: (id: number) => void
    onmove: (dir: Direction) => void
    // children are the task's steps, nested under it.
    children?: Snippet
  } = $props()

  let editing = $state(false)
  let confirmingDelete = $state(false)
  let form = $state<TaskForm>({ title: '', notes: '', size: '', energy: '' })
  let saving = $state(false)
  let problems = $state<Record<string, string>>({})
  let notice = $state('')

  const shownFields = ['title', 'notes', 'size', 'energy']
  const closed = $derived(task.status === 'done' || task.status === 'dropped')
  const idPrefix = $derived(`task-${task.id}`)

  function edit() {
    form = taskFields(task)
    problems = {}
    notice = ''
    confirmingDelete = false
    editing = true
  }

  function failed(err: unknown, fallback: string) {
    if (err instanceof ApiError && err.code === 'invalid' && err.problems.length > 0) {
      problems = Object.fromEntries(err.problems.map((p) => [p.field, p.problem]))
      // Problems on fields the form doesn't show must still be seen.
      if (err.problems.some((p) => !shownFields.includes(p.field))) notice = err.message
    } else {
      notice = err instanceof ApiError ? err.message : fallback
    }
  }

  async function save(e: SubmitEvent) {
    e.preventDefault()
    problems = {}
    notice = ''
    if (form.title.trim() === '') {
      problems = { title: 'Give it a short title.' }
      return
    }
    const patch = taskPatch(task, form)
    if (patch === null) {
      editing = false
      return
    }
    saving = true
    try {
      onchanged(await patchJSON<Task>(`/api/tasks/${task.id}`, patch))
      editing = false
    } catch (err) {
      failed(err, "Couldn't save the task.")
    } finally {
      saving = false
    }
  }

  async function setStatus(select: HTMLSelectElement) {
    const status = select.value as TaskStatus
    if (status === task.status) return
    notice = ''
    saving = true
    try {
      onchanged(await patchJSON<Task>(`/api/tasks/${task.id}`, { status }))
    } catch (err) {
      notice = err instanceof ApiError ? err.message : "Couldn't change the task's status."
      select.value = task.status // show the status the task still has
    } finally {
      saving = false
    }
  }

  async function remove() {
    notice = ''
    saving = true
    try {
      await deleteJSON(`/api/tasks/${task.id}`)
      ondeleted(task.id)
    } catch (err) {
      notice = err instanceof ApiError ? err.message : "Couldn't delete the task."
      saving = false
    }
  }
</script>

<li class="px-4 py-3">
  {#if editing}
    <form class="space-y-3" onsubmit={save} novalidate>
      <div>
        <label for="{idPrefix}-title" class="text-sm font-medium text-ink">Task</label>
        <Input
          id="{idPrefix}-title"
          bind:value={form.title}
          autofocus
          autocomplete="off"
          aria-invalid={problems.title ? 'true' : undefined}
          aria-describedby={problems.title ? `${idPrefix}-title-problem` : undefined}
        />
        {#if problems.title}
          <p id="{idPrefix}-title-problem" class="mt-1 text-sm text-accent">{problems.title}</p>
        {/if}
      </div>
      <div>
        <label for="{idPrefix}-notes" class="text-sm font-medium text-ink">Notes</label>
        <Textarea id="{idPrefix}-notes" rows={2} bind:value={form.notes} />
        {#if problems.notes}<p class="mt-1 text-sm text-accent">{problems.notes}</p>{/if}
      </div>
      <div class="flex gap-3">
        <div class="flex-1">
          <label for="{idPrefix}-size" class="text-sm font-medium text-ink">Size</label>
          <select id="{idPrefix}-size" bind:value={form.size} class={fieldClass}>
            <option value="">Not sized</option>
            {#each taskSizeValues as s (s)}<option value={s}>{s}</option>{/each}
          </select>
          {#if problems.size}<p class="mt-1 text-sm text-accent">{problems.size}</p>{/if}
        </div>
        <div class="flex-1">
          <label for="{idPrefix}-energy" class="text-sm font-medium text-ink">Energy</label>
          <select id="{idPrefix}-energy" bind:value={form.energy} class={fieldClass}>
            <option value="">Not set</option>
            {#each energyValues as en (en)}<option value={en}>{en}</option>{/each}
          </select>
          {#if problems.energy}<p class="mt-1 text-sm text-accent">{problems.energy}</p>{/if}
        </div>
      </div>
      {#if notice}<p class="text-sm text-accent" role="status">{notice}</p>{/if}
      <div class="flex gap-2">
        <Button type="submit" disabled={saving}>{saving ? 'Saving…' : 'Save'}</Button>
        <Button variant="quiet" onclick={() => (editing = false)}>Cancel</Button>
      </div>
    </form>
  {:else}
    <div class="flex items-start gap-3">
      <div class="min-w-0 flex-1">
        <a href={paths.task(task.id)} class={['hover:text-accent', closed ? 'text-ink-muted' : 'text-ink']}>{task.title}</a>
        {#if task.size || task.energy}
          <span class="ml-2 inline-flex gap-1 align-middle">
            {#if task.size}<span class="rounded-control border border-line px-1.5 text-xs text-ink-muted">{task.size}</span>{/if}
            {#if task.energy}<span class="rounded-control border border-line px-1.5 text-xs text-ink-muted">{task.energy} energy</span>{/if}
          </span>
        {/if}
        {#if task.notes}<p class="mt-0.5 text-sm whitespace-pre-line text-ink-muted">{task.notes}</p>{/if}
      </div>
      <div class="flex shrink-0 items-center gap-1">
        <Button
          variant="quiet"
          size="xs"
          aria-label="Move {task.title} up"
          disabled={!canMoveUp || moving}
          onclick={() => onmove('up')}
          class="disabled:invisible">↑</Button
        >
        <Button
          variant="quiet"
          size="xs"
          aria-label="Move {task.title} down"
          disabled={!canMoveDown || moving}
          onclick={() => onmove('down')}
          class="disabled:invisible">↓</Button
        >
        <label for="{idPrefix}-status" class="sr-only">Status of {task.title}</label>
        <select
          id="{idPrefix}-status"
          value={task.status}
          disabled={saving}
          onchange={(e) => setStatus(e.currentTarget)}
          class="rounded-control border border-line bg-canvas px-2 py-1 text-sm text-ink-muted focus:border-accent focus:outline-none"
        >
          {#each statusLabels as o (o.status)}<option value={o.status}>{o.label}</option>{/each}
        </select>
        <Button variant="secondary" size="sm" onclick={edit}>Edit</Button>
        <Button variant="danger" size="sm" onclick={() => (confirmingDelete = true)}>Delete</Button>
      </div>
    </div>
    {#if confirmingDelete}
      <div class="mt-2 flex items-center gap-2 text-sm" role="group" aria-label="Confirm delete">
        <span class="text-ink-muted">Delete this task?</span>
        <Button variant="danger" size="sm" disabled={saving} onclick={remove}>Delete task</Button>
        <Button variant="quiet" size="sm" onclick={() => (confirmingDelete = false)}>Keep it</Button>
      </div>
    {/if}
    {#if notice}<p class="mt-2 text-sm text-accent" role="status">{notice}</p>{/if}
  {/if}
  {@render children?.()}
</li>
