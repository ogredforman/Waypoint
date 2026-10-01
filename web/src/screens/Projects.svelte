<script lang="ts">
  import { onMount } from 'svelte'
  import NewProjectForm from '../components/NewProjectForm.svelte'
  import Button from '../components/ui/Button.svelte'
  import Panel from '../components/ui/Panel.svelte'
  import { ApiError, getJSON } from '../lib/api/client'
  import type { Project, Settings } from '../lib/api/types.gen'
  import { latest, liveReload } from '../lib/live'
  import { pageHeading } from '../lib/navigation.svelte'
  import { countActive, groupByStatus } from '../lib/projects'
  import { paths } from '../lib/router'

  let projects = $state<Project[] | null>(null)
  let loadError = $state('')
  let cap = $state<number | null>(null)
  let formOpen = $state(false)

  const groups = $derived(groupByStatus(projects ?? []))
  const activeCount = $derived(countActive(projects ?? []))

  const loads = latest()

  async function load() {
    const current = loads()
    try {
      const list = await getJSON<Project[]>('/api/projects')
      if (!current()) return
      projects = list
      loadError = ''
    } catch (err) {
      if (current()) loadError = err instanceof ApiError ? err.message : "Couldn't load your projects."
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
    return liveReload(load)
  })
</script>

<section class="space-y-6">
  <div class="flex items-center justify-between gap-4">
    <h1 class="text-title font-semibold tracking-tight" {@attach pageHeading}>Projects</h1>
    {#if projects && projects.length > 0 && !formOpen}
      <Button onclick={() => (formOpen = true)}>New project</Button>
    {/if}
  </div>

  {#if loadError}
    <p class="text-ink-muted">
      {loadError}
      <button type="button" class="ml-1 text-accent underline underline-offset-2" onclick={load}>Try again</button>
    </p>
  {:else if projects === null}
    <p class="text-ink-muted">Loading…</p>
  {:else}
    {#if projects.length === 0}
      <p class="text-ink-muted">No projects yet. Start with one thing you'd like to move forward; it can be small.</p>
    {/if}

    {#if formOpen || projects.length === 0}
      <NewProjectForm {activeCount} {cap} oncreated={created} oncancel={projects.length > 0 ? () => (formOpen = false) : undefined} />
    {/if}

    {#each groups as group (group.status)}
      <div>
        <h2 class="text-label font-medium tracking-wide text-ink-muted uppercase">{group.label}</h2>
        <Panel class="mt-2">
          <ul class="divide-y divide-line">
            {#each group.projects as p (p.id)}
              <li>
                <a href={paths.project(p.id)} class="block px-4 py-3 transition hover:bg-raised">
                  <span class="font-medium text-ink">{p.title}</span>
                  {#if p.why}
                    <span class="mt-0.5 line-clamp-1 block text-sm text-ink-muted">{p.why}</span>
                  {/if}
                </a>
              </li>
            {/each}
          </ul>
        </Panel>
      </div>
    {/each}
  {/if}
</section>
