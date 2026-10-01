<script lang="ts">
  import { onMount } from 'svelte'
  import { ApiError, getJSON } from '../lib/api/client'
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

  async function load() {
    loadError = ''
    try {
      ideas = await getJSON<Idea[]>(`/api/ideas?project_id=${projectId}`)
    } catch (err) {
      loadError = err instanceof ApiError ? err.message : "Couldn't load the ideas."
    }
  }

  async function loadProjects() {
    try {
      projects = await getJSON<Project[]>('/api/projects')
    } catch {
      projects = null // promoting to a new project still works
    }
  }

  function removed(id: number) {
    ideas = (ideas ?? []).filter((i) => i.id !== id)
  }

  onMount(() => {
    load()
    loadProjects()
  })
</script>

<section aria-labelledby="ideas-heading" class="space-y-3">
  <h2 id="ideas-heading" class="text-lg font-semibold">Ideas</h2>

  {#if loadError}
    <p class="text-ink-muted">
      {loadError}
      <button type="button" class="ml-1 text-accent underline underline-offset-2" onclick={load}>Try again</button>
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
            ondeleted={removed}
          />
        {/each}
      </ul>
    </Panel>
  {/if}
</section>
