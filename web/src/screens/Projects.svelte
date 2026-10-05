<script lang="ts">
  import ChevronRight from '@lucide/svelte/icons/chevron-right'
  import { onMount } from 'svelte'
  import NewProjectForm from '../components/NewProjectForm.svelte'
  import Button from '../components/ui/Button.svelte'
  import Input from '../components/ui/Input.svelte'
  import Panel from '../components/ui/Panel.svelte'
  import { ApiError, getJSON, patchJSON } from '../lib/api/client'
  import type { ProjectStatus } from '../lib/api/enums.gen'
  import type { Project, Settings } from '../lib/api/types.gen'
  import { latest, liveReload } from '../lib/live'
  import { pageHeading } from '../lib/navigation.svelte'
  import { collapsedByDefault, countActive, filterProjects, groupByStatus } from '../lib/projects'
  import { plainText } from '../lib/richtext'
  import { paths } from '../lib/router'

  let projects = $state<Project[] | null>(null)
  let loadError = $state('')
  let cap = $state<number | null>(null)
  let formOpen = $state(false)
  let restoring = $state<number | null>(null)
  let restoreError = $state('')

  let query = $state('')
  // opened holds the sections the user opened or closed; the rest follow collapsedByDefault.
  let opened = $state<Partial<Record<ProjectStatus, boolean>>>({})

  const filtering = $derived(query.trim() !== '')
  const groups = $derived(groupByStatus(filterProjects(projects ?? [], query)))
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

  // isOpen: while filtering, every section with a match shows, so nothing found is hidden.
  function isOpen(status: ProjectStatus): boolean {
    return filtering || (opened[status] ?? !collapsedByDefault(status))
  }

  function toggle(status: ProjectStatus) {
    opened[status] = !isOpen(status)
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

    {#if projects.length > 0}
      <Input type="search" bind:value={query} aria-label="Filter projects" placeholder="Filter projects" class="w-full sm:max-w-xs" />
    {/if}

    {#if filtering && groups.length === 0}
      <p class="text-ink-muted">No projects match “{query.trim()}”.</p>
    {/if}

    {#each groups as group (group.status)}
      {@const open = isOpen(group.status)}
      <div>
        <h2 class="text-label font-medium tracking-wide text-ink-muted uppercase">
          {#if collapsedByDefault(group.status)}
            <button
              type="button"
              class="inline-flex items-center gap-1 rounded uppercase transition hover:text-ink"
              aria-expanded={open}
              disabled={filtering}
              onclick={() => toggle(group.status)}
            >
              <ChevronRight aria-hidden="true" class={['size-3.5 transition-transform', open && 'rotate-90']} />
              {group.label}
              <span class="font-normal normal-case">({group.projects.length})</span>
            </button>
          {:else}
            {group.label}
          {/if}
        </h2>
        {#if group.status === 'archived' && restoreError}
          <p class="mt-1 text-sm text-ink-muted" role="status">{restoreError}</p>
        {/if}
        {#if open}
          <Panel class="mt-2">
            <ul class="divide-y divide-line">
              {#each group.projects as p (p.id)}
                <li class="flex items-center">
                  <a href={paths.project(p.id)} class="block min-w-0 flex-1 px-4 py-3 transition hover:bg-raised">
                    <span class="font-medium text-ink">{p.title}</span>
                    {#if p.why}
                      <span class="mt-0.5 line-clamp-1 block text-sm text-ink-muted">{plainText(p.why)}</span>
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
        {/if}
      </div>
    {/each}
  {/if}
</section>
