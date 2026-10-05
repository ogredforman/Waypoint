<script lang="ts">
  import { ApiError, deleteJSON, patchJSON, postJSON } from '../lib/api/client'
  import type { Idea, Project, PromoteIdeaRequest, PromoteIdeaResponse, PromoteTarget } from '../lib/api/types.gen'
  import { defaultTarget, ideaPatch, projectName, promoteTargets, type IdeaForm } from '../lib/ideas'
  import { paths } from '../lib/router'
  import Button from './ui/Button.svelte'
  import Input from './ui/Input.svelte'
  import RichText from './ui/RichText.svelte'
  import RichTextEditor from './ui/RichTextEditor.svelte'

  let {
    idea,
    projects,
    showProject = false,
    onpromoted,
    onchanged,
    ondeleted,
  }: {
    idea: Idea
    // projects are where a promoted task can go; null while they load.
    projects: Project[] | null
    showProject?: boolean
    onpromoted: (r: PromoteIdeaResponse) => void
    onchanged: (i: Idea) => void
    ondeleted: (id: number) => void
  } = $props()

  let editing = $state(false)
  let form = $state<IdeaForm>({ title: '', body: '' })
  let titleProblem = $state('')
  let promoting = $state(false)
  let confirmingDelete = $state(false)
  let to = $state<PromoteTarget>('task')
  let targetId = $state<number | null>(null)
  let saving = $state(false)
  let notice = $state('')

  const targets = $derived(promoteTargets(projects ?? []))
  const name = $derived(projectName(idea, projects ?? []))
  const idPrefix = $derived(`idea-${idea.id}`)

  function edit() {
    form = { title: idea.title, body: idea.body }
    titleProblem = ''
    notice = ''
    promoting = false
    confirmingDelete = false
    editing = true
  }

  async function save(e: SubmitEvent) {
    e.preventDefault()
    titleProblem = ''
    notice = ''
    if (form.title.trim() === '') {
      titleProblem = 'Give it a short title.'
      return
    }
    const patch = ideaPatch(idea, form)
    if (patch === null) {
      editing = false
      return
    }
    saving = true
    try {
      onchanged(await patchJSON<Idea>(`/api/ideas/${idea.id}`, patch))
      editing = false
    } catch (err) {
      notice = err instanceof ApiError ? err.message : "Couldn't save the idea."
    } finally {
      saving = false
    }
  }

  function openPromote() {
    confirmingDelete = false
    notice = ''
    targetId = defaultTarget(idea, targets)
    to = targetId === null ? 'project' : 'task'
    promoting = true
  }

  async function promote() {
    const req: PromoteIdeaRequest = { to }
    if (to === 'task') {
      if (targetId === null) return
      req.project_id = targetId
    }
    notice = ''
    saving = true
    try {
      onpromoted(await postJSON<PromoteIdeaResponse>(`/api/ideas/${idea.id}/promote`, req))
    } catch (err) {
      notice = err instanceof ApiError ? err.message : "Couldn't promote the idea."
      saving = false
    }
  }

  async function remove() {
    notice = ''
    saving = true
    try {
      await deleteJSON(`/api/ideas/${idea.id}`)
      ondeleted(idea.id)
    } catch (err) {
      notice = err instanceof ApiError ? err.message : "Couldn't delete the idea."
      saving = false
    }
  }
</script>

<li class="px-4 py-3">
  {#if editing}
    <form class="space-y-3" aria-label="Edit idea" onsubmit={save} novalidate>
      <div>
        <label for="{idPrefix}-title" class="text-sm font-medium text-ink">Idea</label>
        <Input
          id="{idPrefix}-title"
          bind:value={form.title}
          autofocus
          autocomplete="off"
          aria-invalid={titleProblem ? 'true' : undefined}
          aria-describedby={titleProblem ? `${idPrefix}-title-problem` : undefined}
        />
        {#if titleProblem}<p id="{idPrefix}-title-problem" class="mt-1 text-sm text-accent">{titleProblem}</p>{/if}
      </div>
      <div>
        <span id="{idPrefix}-body-label" class="text-sm font-medium text-ink">Details</span>
        <RichTextEditor id="{idPrefix}-body" labelledby="{idPrefix}-body-label" bind:value={form.body} />
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
        <p class="text-ink">{idea.title}</p>
        {#if idea.body}<RichText value={idea.body} class="mt-0.5 text-sm text-ink-muted" />{/if}
        {#if showProject}
          {#if name !== null && idea.project_id !== null}
            <a href={paths.project(idea.project_id)} class="text-sm text-ink-muted hover:text-accent">{name}</a>
          {:else}
            <p class="text-sm text-ink-faint">No project</p>
          {/if}
        {/if}
      </div>
      <div class="flex shrink-0 items-center gap-1">
        <Button variant="quiet" size="sm" onclick={edit}>Edit</Button>
        <Button variant="secondary" size="sm" disabled={promoting} onclick={openPromote}>Promote</Button>
        <Button
          variant="danger"
          size="sm"
          onclick={() => {
            promoting = false
            confirmingDelete = true
          }}>Delete</Button
        >
      </div>
    </div>
    {#if promoting}
      <form
        class="mt-2 flex flex-wrap items-center gap-x-4 gap-y-2 text-sm"
        aria-label="Promote idea"
        onsubmit={(e) => {
          e.preventDefault()
          promote()
        }}
      >
        <div class="flex items-center gap-2">
          <label class="flex items-center gap-2 text-ink-muted">
            <input type="radio" name="{idPrefix}-to" value="task" bind:group={to} disabled={targets.length === 0} />
            Task in
          </label>
          <label for="{idPrefix}-project" class="sr-only">Project for the task</label>
          <select
            id="{idPrefix}-project"
            bind:value={targetId}
            disabled={to !== 'task' || targets.length === 0}
            class="rounded-control border border-line bg-canvas px-2 py-1 text-sm text-ink-muted focus:border-accent focus:outline-none"
          >
            {#each targets as p (p.id)}<option value={p.id}>{p.title}</option>{/each}
          </select>
        </div>
        <label class="flex items-center gap-2 text-ink-muted">
          <input type="radio" name="{idPrefix}-to" value="project" bind:group={to} />
          New parked project
        </label>
        <div class="flex items-center gap-2">
          <Button
            type="submit"
            variant="secondary"
            size="sm"
            disabled={saving || (to === 'task' && targetId === null)}>Promote idea</Button
          >
          <Button variant="quiet" size="sm" onclick={() => (promoting = false)}>Cancel</Button>
        </div>
      </form>
    {/if}
    {#if confirmingDelete}
      <div class="mt-2 flex items-center gap-2 text-sm" role="group" aria-label="Confirm delete">
        <span class="text-ink-muted">Delete this idea?</span>
        <Button variant="danger" size="sm" disabled={saving} onclick={remove}>Delete idea</Button>
        <Button variant="quiet" size="sm" onclick={() => (confirmingDelete = false)}>Keep it</Button>
      </div>
    {/if}
    {#if notice}<p class="mt-2 text-sm text-accent" role="status">{notice}</p>{/if}
  {/if}
</li>
