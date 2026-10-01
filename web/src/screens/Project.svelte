<script lang="ts">
  import { onMount } from 'svelte'
  import ProjectBlockers from '../components/ProjectBlockers.svelte'
  import ProjectHeader from '../components/ProjectHeader.svelte'
  import ProjectIdeas from '../components/ProjectIdeas.svelte'
  import ProjectStatus from '../components/ProjectStatus.svelte'
  import ProjectTasks from '../components/ProjectTasks.svelte'
  import { ApiError, getJSON } from '../lib/api/client'
  import type { Project, Settings } from '../lib/api/types.gen'
  import { latest, liveReload } from '../lib/live'
  import { pageHeading } from '../lib/navigation.svelte'
  import { countActive } from '../lib/projects'
  import { paths } from '../lib/router'

  let { id }: { id: number } = $props()

  let project = $state<Project | null>(null)
  let missing = $state(false)
  let loadError = $state('')
  // projects is only for counting active ones against the cap; the server has the final say.
  let projects = $state<Project[]>([])
  let cap = $state<number | null>(null)
  // tasksVersion remounts the task list when an idea is promoted into it.
  let tasksVersion = $state(0)

  const activeCount = $derived(countActive(projects))

  const loads = latest()
  const capLoads = latest()

  async function load() {
    const current = loads()
    try {
      const p = await getJSON<Project>(`/api/projects/${id}`)
      if (!current()) return
      project = p
      loadError = ''
      missing = false
    } catch (err) {
      if (!current()) return
      if (err instanceof ApiError && err.code === 'not_found') missing = true
      else loadError = err instanceof ApiError ? err.message : "Couldn't load this project."
    }
  }

  async function loadCapInfo() {
    const current = capLoads()
    try {
      const [list, settings] = await Promise.all([getJSON<Project[]>('/api/projects'), getJSON<Settings>('/api/settings')])
      if (!current()) return
      projects = list
      cap = settings.active_project_cap
    } catch {
      if (current()) cap = null // an unknown cap never blocks; the server still enforces it
    }
  }

  function updated(p: Project) {
    project = p
    projects = projects.map((q) => (q.id === p.id ? p : q))
  }

  onMount(() => {
    load()
    loadCapInfo()
    // The tasks, blockers, and ideas below keep themselves current.
    const offs = [
      liveReload(load, () => id),
      // Any project's status change moves the active count.
      liveReload(loadCapInfo),
    ]
    return () => offs.forEach((off) => off())
  })
</script>

<section data-project-id={id} class="space-y-6">
  <a href={paths.projects} class="text-sm text-ink-muted hover:text-ink">← Projects</a>

  {#if missing}
    <h1 class="text-title font-semibold tracking-tight" {@attach pageHeading}>Project</h1>
    <p class="text-ink-muted">This project isn't here. The Projects list has all of them.</p>
  {:else if loadError}
    <h1 class="text-title font-semibold tracking-tight" {@attach pageHeading}>Project</h1>
    <p class="text-ink-muted">
      {loadError}
      <button type="button" class="ml-1 text-accent underline underline-offset-2" onclick={load}>Try again</button>
    </p>
  {:else if project === null}
    <p class="text-ink-muted">Loading…</p>
  {:else}
    <ProjectHeader {project} onsaved={updated} />
    <ProjectStatus {project} {activeCount} {cap} onchanged={updated} />
    {#key tasksVersion}<ProjectTasks projectId={project.id} />{/key}
    <ProjectBlockers projectId={project.id} />
    <ProjectIdeas
      projectId={project.id}
      onpromoted={(r) => {
        if (r.task?.project_id === project?.id) tasksVersion++
      }}
    />
  {/if}
</section>
