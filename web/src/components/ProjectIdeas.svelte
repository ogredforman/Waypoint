<script lang="ts">
  import Button from './ui/Button.svelte'
  import { onMount } from 'svelte'
  import { ApiError, getJSON } from '../lib/api/client'
  import { latest, liveReload } from '../lib/live'
  import type { Idea, Project, PromoteIdeaResponse } from '../lib/api/types.gen'
  import IdeaRow from './IdeaRow.svelte'
  import Panel from './ui/Panel.svelte'

  let {
    projectId,
    onpromoted,
  }: {
    projectId: number
    // onpromoted lets the Project screen show a task promoted into it.
    onpromoted?: (r: PromoteIdeaResponse) => void
  } = $props()

  // Parked ideas only (the endpoint's default).
  let ideas = $state<Idea[] | null>(null)
  // projects are where a promoted task can go.
  let projects = $state<Project[] | null>(null)
  let loadError = $state('')

  const loads = latest()

  async function load() {
    const current = loads()
    try {
      const list = await getJSON<Idea[]>(`/api/ideas?project_id=${projectId}`)
      if (!current()) return
      ideas = list
      loadError = ''
    } catch (err) {
      if (current()) loadError = err instanceof ApiError ? err.message : "Couldn't load the ideas."
    }
  }

  const projectLoads = latest()

  async function loadProjects() {
    const current = projectLoads()
    try {
      const list = await getJSON<Project[]>('/api/projects')
      if (current()) projects = list
    } catch {
      if (current()) projects = null // promoting to a new project still works
    }
  }

  function changed(idea: Idea) {
    ideas = (ideas ?? []).map((i) => (i.id === idea.id ? idea : i))
  }

  function removed(id: number) {
    ideas = (ideas ?? []).filter((i) => i.id !== id)
  }

  function reload() {
    load()
    loadProjects()
  }

  onMount(() => {
    reload()
    // Ideas aren't in a changeset's project_ids, so any applied changeset reloads them.
    return liveReload(reload)
  })
</script>

<section aria-labelledby="ideas-heading" class="space-y-3">
  <h2 id="ideas-heading" class="text-lg font-semibold">Ideas</h2>

  {#if loadError}
    <p class="text-ink-muted">
      {loadError}
      <Button variant="quiet" size="xs" class="ml-1" onclick={load}>Try again</Button>
    </p>
  {:else if ideas === null}
    <p class="text-ink-muted">Loading…</p>
  {:else if ideas.length === 0}
    <p class="text-ink-muted">No parked ideas for this project.</p>
  {:else}
    <Panel>
      <ul class="divide-y divide-line" aria-label="Parked ideas">
        {#each ideas as idea (idea.id)}
          <IdeaRow
            {idea}
            {projects}
            onpromoted={(r) => {
              removed(r.idea.id)
              onpromoted?.(r)
            }}
            onchanged={changed}
            ondeleted={removed}
          />
        {/each}
      </ul>
    </Panel>
  {/if}
</section>
