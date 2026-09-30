<script lang="ts">
  import { onMount } from 'svelte'
  import BriefingCard from '../components/BriefingCard.svelte'
  import { ApiError, getJSON } from '../lib/api/client'
  import type { Briefing, Project } from '../lib/api/types.gen'
  import { activeProjects } from '../lib/projects'
  import { paths } from '../lib/router'

  // The home screen (SPEC §11): a briefing per active project. Live updates arrive in 1.22.

  interface Item {
    project: Project
    briefing: Briefing | null
    error: string
  }

  let items = $state<Item[] | null>(null)
  let loadError = $state('')

  async function load() {
    loadError = ''
    let active: Project[]
    try {
      active = activeProjects(await getJSON<Project[]>('/api/projects'))
    } catch (err) {
      loadError = err instanceof ApiError ? err.message : "Couldn't load your projects."
      return
    }
    items = active.map((project) => ({ project, briefing: null, error: '' }))
    const list = items // a later load replaces it; late answers for this one are dropped
    // Each briefing loads on its own, so one that fails doesn't hide the others.
    await Promise.all(
      active.map(async (project, i) => {
        try {
          const briefing = await getJSON<Briefing>(`/api/projects/${project.id}/briefing`)
          if (items === list) list[i].briefing = briefing
        } catch (err) {
          if (items === list) list[i].error = err instanceof ApiError ? err.message : "Couldn't load this briefing."
        }
      }),
    )
  }

  onMount(load)
</script>

<section class="space-y-8">
  <h1 class="text-title font-semibold tracking-tight">Now</h1>

  {#if loadError}
    <p class="text-ink-muted">
      {loadError}
      <button type="button" class="ml-1 text-accent underline underline-offset-2" onclick={load}>Try again</button>
    </p>
  {:else if items === null}
    <p class="text-ink-muted">Loading…</p>
  {:else if items.length === 0}
    <p class="text-ink-muted">
      No active projects right now. Pick one thing to move forward on the
      <a href={paths.projects} class="text-accent underline underline-offset-2">Projects</a> page; it can be small.
    </p>
  {:else}
    {#each items as item (item.project.id)}
      <BriefingCard project={item.project} briefing={item.briefing} error={item.error} />
    {/each}
  {/if}
</section>
