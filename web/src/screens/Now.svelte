<script lang="ts">
  import { onMount } from 'svelte'
  import BriefingCard from '../components/BriefingCard.svelte'
  import { ApiError, getJSON } from '../lib/api/client'
  import type { Briefing, Project } from '../lib/api/types.gen'
  import { on, onReconnect } from '../lib/events'
  import { activeProjects } from '../lib/projects'
  import { paths } from '../lib/router'

  // The home screen (SPEC §11): a briefing per active project, kept current by server events.

  interface Item {
    project: Project
    briefing: Briefing | null
    error: string
  }

  let items = $state<Item[] | null>(null)
  let loadError = $state('')

  // Answers that a newer request replaced are dropped: loadGen counts project
  // list loads, briefingLoads holds the latest briefing request per project.
  let loadGen = 0
  const briefingLoads = new Map<number, number>()
  let briefingSeq = 0

  // load (re)loads the active projects. Briefings already shown stay until their
  // fresh copy arrives, so a reload doesn't flicker. With all false, only the
  // projects without a briefing yet fetch one: briefing.updated covers the rest.
  async function load(all = true) {
    const gen = ++loadGen
    let active: Project[]
    try {
      active = activeProjects(await getJSON<Project[]>('/api/projects'))
    } catch (err) {
      if (gen === loadGen) loadError = err instanceof ApiError ? err.message : "Couldn't load your projects."
      return
    }
    if (gen !== loadGen) return
    loadError = ''
    const shown = new Map((items ?? []).map((i) => [i.project.id, i]))
    items = active.map((project) => {
      const old = shown.get(project.id)
      return { project, briefing: old?.briefing ?? null, error: old?.error ?? '' }
    })
    // Each briefing loads on its own, so one that fails doesn't hide the others.
    await Promise.all(items.filter((i) => all || i.briefing === null).map((i) => loadBriefing(i.project.id)))
  }

  // loadBriefing fetches one project's briefing into whichever list is shown
  // when it arrives.
  async function loadBriefing(projectID: number) {
    const seq = ++briefingSeq
    briefingLoads.set(projectID, seq)
    const current = () => (briefingLoads.get(projectID) === seq ? items?.find((i) => i.project.id === projectID) : undefined)
    try {
      const briefing = await getJSON<Briefing>(`/api/projects/${projectID}/briefing`)
      const item = current()
      if (item) {
        item.briefing = briefing
        item.error = ''
      }
    } catch (err) {
      const item = current()
      if (item) item.error = err instanceof ApiError ? err.message : "Couldn't load this briefing."
    }
  }

  // briefingUpdated refetches one briefing, or the whole list for a project not
  // shown yet (e.g. one just activated).
  function briefingUpdated(projectID: number) {
    if (items?.some((i) => i.project.id === projectID)) void loadBriefing(projectID)
    else void load(false)
  }

  onMount(() => {
    load()
    const offs = [
      on('briefing.updated', (e) => briefingUpdated(e.project_id)),
      // A project may have been activated, paused, renamed, or finished.
      on('changeset.applied', () => load(false)),
      onReconnect(() => load()),
    ]
    return () => offs.forEach((off) => off())
  })
</script>

<section class="space-y-8">
  <h1 class="text-title font-semibold tracking-tight">Now</h1>

  {#if loadError}
    <p class="text-ink-muted">
      {loadError}
      <button type="button" class="ml-1 text-accent underline underline-offset-2" onclick={() => load()}>Try again</button>
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
