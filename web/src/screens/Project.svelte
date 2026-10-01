<script lang="ts">
  import { onMount } from 'svelte'
  import ProjectBlockers from '../components/ProjectBlockers.svelte'
  import ProjectHeader from '../components/ProjectHeader.svelte'
  import ProjectIdeas from '../components/ProjectIdeas.svelte'
  import ProjectStatus from '../components/ProjectStatus.svelte'
  import ProjectTasks from '../components/ProjectTasks.svelte'
  import { ApiError, getJSON } from '../lib/api/client'
  import type { Project, Settings } from '../lib/api/types.gen'
  import { countActive } from '../lib/projects'
  import { paths } from '../lib/router'

  let { id }: { id: number } = $props()

  let project = $state<Project | null>(null)
  let missing = $state(false)
  let loadError = $state('')
  // projects is only for counting active ones against the cap; the server has the final say.
  let projects = $state<Project[]>([])
  let cap = $state<number | null>(null)

  const activeCount = $derived(countActive(projects))

  async function load() {
    loadError = ''
    missing = false
    try {
      project = await getJSON<Project>(`/api/projects/${id}`)
    } catch (err) {
      if (err instanceof ApiError && err.code === 'not_found') missing = true
      else loadError = err instanceof ApiError ? err.message : "Couldn't load this project."
    }
  }

  async function loadCapInfo() {
    try {
      const [list, settings] = await Promise.all([getJSON<Project[]>('/api/projects'), getJSON<Settings>('/api/settings')])
      projects = list
      cap = settings.active_project_cap
    } catch {
      cap = null // an unknown cap never blocks; the server still enforces it
    }
  }

  function updated(p: Project) {
    project = p
    projects = projects.map((q) => (q.id === p.id ? p : q))
  }

  onMount(() => {
    load()
    loadCapInfo()
  })
</script>

<section data-project-id={id} class="space-y-6">
  <a href={paths.projects} class="text-sm text-ink-muted hover:text-ink">← Projects</a>

  {#if missing}
    <p class="text-ink-muted">This project isn't here. The Projects list has all of them.</p>
  {:else if loadError}
    <p class="text-ink-muted">
      {loadError}
      <button type="button" class="ml-1 text-accent underline underline-offset-2" onclick={load}>Try again</button>
    </p>
  {:else if project === null}
    <p class="text-ink-muted">Loading…</p>
  {:else}
    <ProjectHeader {project} onsaved={updated} />
    <ProjectStatus {project} {activeCount} {cap} onchanged={updated} />
    <ProjectTasks projectId={project.id} />
    <ProjectBlockers projectId={project.id} />
    <ProjectIdeas projectId={project.id} />
  {/if}
</section>
