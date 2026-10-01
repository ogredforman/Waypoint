<script lang="ts">
  import Button from '../components/ui/Button.svelte'
  import { onMount } from 'svelte'
  import IdeaRow from '../components/IdeaRow.svelte'
  import ParkedProjectRow from '../components/ParkedProjectRow.svelte'
  import Panel from '../components/ui/Panel.svelte'
  import { ApiError, getJSON } from '../lib/api/client'
  import type { Idea, Project, PromoteIdeaResponse, Settings } from '../lib/api/types.gen'
  import { latest, liveReload } from '../lib/live'
  import { pageHeading } from '../lib/navigation.svelte'
  import { countActive, parkedProjects } from '../lib/projects'

  let ideas = $state<Idea[] | null>(null)
  let projects = $state<Project[] | null>(null)
  let loadError = $state('')
  let cap = $state<number | null>(null)

  const parked = $derived(parkedProjects(projects ?? []))
  const activeCount = $derived(countActive(projects ?? []))

  const loads = latest()

  async function load() {
    const current = loads()
    try {
      const [i, p] = await Promise.all([getJSON<Idea[]>('/api/ideas'), getJSON<Project[]>('/api/projects')])
      if (!current()) return
      ideas = i
      projects = p
      loadError = ''
    } catch (err) {
      if (current()) loadError = err instanceof ApiError ? err.message : "Couldn't load the parking lot."
    }
  }

  async function loadCap() {
    try {
      cap = (await getJSON<Settings>('/api/settings')).active_project_cap
    } catch {
      cap = null // the server still enforces the cap on activation
    }
  }

  function promoted(r: PromoteIdeaResponse) {
    ideas = (ideas ?? []).filter((i) => i.id !== r.idea.id)
    if (r.project) projects = [...(projects ?? []), r.project]
  }

  function deleted(id: number) {
    ideas = (ideas ?? []).filter((i) => i.id !== id)
  }

  function activated(p: Project) {
    projects = (projects ?? []).map((q) => (q.id === p.id ? p : q))
  }

  onMount(() => {
    load()
    loadCap()
    // Ideas aren't in a changeset's project_ids, so any applied changeset reloads.
    return liveReload(load)
  })
</script>

<section class="space-y-8">
  <div>
    <h1 class="text-title font-semibold tracking-tight" {@attach pageHeading}>Parking lot</h1>
    <p class="mt-2 text-ink-muted">Ideas and parked projects wait here until you're ready for them.</p>
  </div>

  {#if loadError}
    <p class="text-ink-muted">
      {loadError}
      <Button variant="quiet" size="xs" class="ml-1" onclick={load}>Try again</Button>
    </p>
  {:else if ideas === null || projects === null}
    <p class="text-ink-muted">Loading…</p>
  {:else}
    <section aria-labelledby="parked-ideas-heading" class="space-y-3">
      <h2 id="parked-ideas-heading" class="text-lg font-semibold">Ideas</h2>
      {#if ideas.length === 0}
        <p class="text-ink-muted">No parked ideas. New ones from your diary land here.</p>
      {:else}
        <Panel>
          <ul class="divide-y divide-line" aria-label="Parked ideas">
            {#each ideas as idea (idea.id)}
              <IdeaRow {idea} {projects} showProject onpromoted={promoted} ondeleted={deleted} />
            {/each}
          </ul>
        </Panel>
      {/if}
    </section>

    <section aria-labelledby="parked-projects-heading" class="space-y-3">
      <h2 id="parked-projects-heading" class="text-lg font-semibold">Parked projects</h2>
      {#if parked.length === 0}
        <p class="text-ink-muted">No parked projects.</p>
      {:else}
        <Panel>
          <ul class="divide-y divide-line" aria-label="Parked projects">
            {#each parked as project (project.id)}
              <ParkedProjectRow {project} {activeCount} {cap} onactivated={activated} />
            {/each}
          </ul>
        </Panel>
      {/if}
    </section>
  {/if}
</section>
