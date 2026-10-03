<script lang="ts">
  import { onMount } from 'svelte'
  import NewProjectForm from '../components/NewProjectForm.svelte'
  import Button from '../components/ui/Button.svelte'
  import Panel from '../components/ui/Panel.svelte'
  import { ApiError, getJSON, patchJSON } from '../lib/api/client'
  import type { Project, Settings } from '../lib/api/types.gen'
  import { latest, liveReload } from '../lib/live'
  import { pageHeading } from '../lib/navigation.svelte'
  import { countActive, groupByStatus } from '../lib/projects'
  import { paths } from '../lib/router'

  let projects = $state<Project[] | null>(null)
  let loadError = $state('')
  let cap = $state<number | null>(null)
  let formOpen = $state(false)
  let restoring = $state<number | null>(null)
  let restoreError = $state('')

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

  // restore brings an archived project back as parked, where it waits until
  // the user picks it up again.
  async function restore(p: Project) {
    if (restoring !== null) return
    restoring = p.id
    restoreError = ''
    try {
      const updated = await patchJSON<Project>(`/api/projects/${p.id}`, { status: 'parked' })
      projects = (projects ?? []).map((q) => (q.id === updated.id ? updated : q))
    } catch {
      restoreError = `Couldn't restore ${p.title}. Try again in a moment.`
    } finally {
      restoring = null
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
      <Button variant="quiet" size="xs" class="ml-1" onclick={load}>Try again</Button>
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
        {#if group.status === 'archived' && restoreError}
          <p class="mt-1 text-sm text-ink-muted" role="status">{restoreError}</p>
        {/if}
        <Panel class="mt-2">
          <ul class="divide-y divide-line">
            {#each group.projects as p (p.id)}
              <li class="flex items-center">
                <a href={paths.project(p.id)} class="block min-w-0 flex-1 px-4 py-3 transition hover:bg-raised">
                  <span class="font-medium text-ink">{p.title}</span>
                  {#if p.why}
                    <span class="mt-0.5 line-clamp-1 block text-sm text-ink-muted">{p.why}</span>
                  {/if}
                </a>
                {#if p.status === 'archived'}
                  <Button
                    variant="quiet"
                    size="sm"
                    class="mr-3 shrink-0"
                    aria-label={`Restore ${p.title}`}
                    disabled={restoring !== null}
                    onclick={() => restore(p)}>Restore</Button
                  >
                {/if}
              </li>
            {/each}
          </ul>
        </Panel>
      </div>
    {/each}
  {/if}
</section>
