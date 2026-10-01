<script lang="ts">
  import { ApiError, deleteJSON, postJSON } from '../lib/api/client'
  import type { Idea, Project, PromoteIdeaRequest, PromoteIdeaResponse, PromoteTarget } from '../lib/api/types.gen'
  import { defaultTarget, projectName, promoteTargets } from '../lib/ideas'
  import { paths } from '../lib/router'
  import Button from './ui/Button.svelte'

  let {
    idea,
    projects,
    showProject = false,
    onpromoted,
    ondeleted,
  }: {
    idea: Idea
    // projects are where a promoted task can go; null while they load.
    projects: Project[] | null
    showProject?: boolean
    onpromoted: (r: PromoteIdeaResponse) => void
    ondeleted: (id: number) => void
  } = $props()

  let promoting = $state(false)
  let confirmingDelete = $state(false)
  let to = $state<PromoteTarget>('task')
  let targetId = $state<number | null>(null)
  let saving = $state(false)
  let notice = $state('')

  const targets = $derived(promoteTargets(projects ?? []))
  const name = $derived(projectName(idea, projects ?? []))
  const idPrefix = $derived(`idea-${idea.id}`)

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
  <div class="flex items-start gap-3">
    <div class="min-w-0 flex-1">
      <p class="text-ink">{idea.title}</p>
      {#if idea.body}<p class="mt-0.5 text-sm whitespace-pre-line text-ink-muted">{idea.body}</p>{/if}
      {#if showProject}
        {#if name !== null && idea.project_id !== null}
          <a href={paths.project(idea.project_id)} class="text-sm text-ink-muted hover:text-accent">{name}</a>
        {:else}
          <p class="text-sm text-ink-faint">No project</p>
        {/if}
      {/if}
    </div>
    <div class="flex shrink-0 items-center gap-1">
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
</li>
