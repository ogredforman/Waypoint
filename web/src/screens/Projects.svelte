<script lang="ts">
  import { onMount } from 'svelte'
  import NewProjectForm from '../components/NewProjectForm.svelte'
  import { ApiError, getJSON } from '../lib/api/client'
  import type { Project, Settings } from '../lib/api/types.gen'
  import { countActive, groupByStatus } from '../lib/projects'
  import { paths } from '../lib/router'

  let projects = $state<Project[] | null>(null)
  let loadError = $state('')
  let cap = $state<number | null>(null)
  let formOpen = $state(false)

  const groups = $derived(groupByStatus(projects ?? []))
  const activeCount = $derived(countActive(projects ?? []))

  async function load() {
    loadError = ''
    try {
      projects = await getJSON<Project[]>('/api/projects')
    } catch (err) {
      loadError = err instanceof ApiError ? err.message : "Couldn't load your projects."
    }
  }

  async function loadCap() {
    try {
      cap = (await getJSON<Settings>('/api/settings')).active_project_cap
    } catch {
      cap = null // the server still enforces the cap on create
    }
  }

  function created(p: Project) {
    projects = [...(projects ?? []), p]
    formOpen = false
  }

  onMount(() => {
    load()
    loadCap()
  })
</script>

<section class="space-y-6">
  <div class="flex items-center justify-between gap-4">
    <h1 class="text-2xl font-semibold tracking-tight">Projects</h1>
    {#if projects && projects.length > 0 && !formOpen}
      <button type="button" class="rounded-md bg-stone-800 px-3 py-1.5 text-sm font-medium text-white hover:bg-stone-700" onclick={() => (formOpen = true)}>
        New project
      </button>
    {/if}
  </div>

  {#if loadError}
    <p class="text-stone-600">
      {loadError}
      <button type="button" class="ml-1 text-stone-900 underline underline-offset-2" onclick={load}>Try again</button>
    </p>
  {:else if projects === null}
    <p class="text-stone-500">Loading…</p>
  {:else}
    {#if projects.length === 0}
      <p class="text-stone-600">No projects yet. Start with one thing you'd like to move forward; it can be small.</p>
    {/if}

    {#if formOpen || projects.length === 0}
      <NewProjectForm {activeCount} {cap} oncreated={created} oncancel={projects.length > 0 ? () => (formOpen = false) : undefined} />
    {/if}

    {#each groups as group (group.status)}
      <div>
        <h2 class="text-sm font-medium tracking-wide text-stone-500 uppercase">{group.label}</h2>
        <ul class="mt-2 divide-y divide-stone-200 rounded-lg border border-stone-200 bg-white">
          {#each group.projects as p (p.id)}
            <li>
              <a href={paths.project(p.id)} class="block px-4 py-3 hover:bg-stone-50">
                <span class="font-medium text-stone-900">{p.title}</span>
                {#if p.why}
                  <span class="mt-0.5 line-clamp-1 block text-sm text-stone-600">{p.why}</span>
                {/if}
              </a>
            </li>
          {/each}
        </ul>
      </div>
    {/each}
  {/if}
</section>
