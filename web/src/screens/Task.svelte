<script lang="ts">
  import { onMount } from 'svelte'
  import Card from '../components/ui/Card.svelte'
  import { ApiError, getJSON } from '../lib/api/client'
  import { latest, liveReload } from '../lib/live'
  import type { Project, TaskDetail } from '../lib/api/types.gen'
  import { kindLabel } from '../lib/entries'
  import { pageHeading } from '../lib/navigation.svelte'
  import { paths } from '../lib/router'
  import { observationTimeline, statusLabels } from '../lib/tasks'

  let { id }: { id: number } = $props()

  let detail = $state<TaskDetail | null>(null)
  let projectTitle = $state<string | null>(null)
  let missing = $state(false)
  let loadError = $state('')

  const task = $derived(detail?.task ?? null)
  const status = $derived(statusLabels.find((s) => s.status === task?.status)?.label ?? '')
  const timeline = $derived(observationTimeline(detail?.observations ?? []))

  function when(at: string): string {
    return new Date(at).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' })
  }

  const loads = latest()

  async function load() {
    const current = loads()
    let d: TaskDetail
    try {
      d = await getJSON<TaskDetail>(`/api/tasks/${id}`)
    } catch (err) {
      if (!current()) return
      if (err instanceof ApiError && err.code === 'not_found') missing = true
      else loadError = err instanceof ApiError ? err.message : "Couldn't load this task."
      return
    }
    if (!current()) return
    detail = d
    loadError = ''
    missing = false
    try {
      const title = (await getJSON<Project>(`/api/projects/${d.task.project_id}`)).title
      if (current()) projectTitle = title
    } catch {
      if (current()) projectTitle = null // the back link still works without the name
    }
  }

  onMount(() => {
    load()
    // Until the task has loaded its project is unknown, so any changeset reloads it.
    return liveReload(load, () => detail?.task.project_id)
  })
</script>

<section data-task-id={id} class="space-y-6">
  {#if missing}
    <a href={paths.projects} class="text-sm text-ink-muted hover:text-ink">← Projects</a>
    <h1 class="text-title font-semibold tracking-tight text-ink" {@attach pageHeading}>Task</h1>
    <p class="text-ink-muted">This task isn't here. Its project's page has the ones that are.</p>
  {:else if loadError}
    <h1 class="text-title font-semibold tracking-tight text-ink" {@attach pageHeading}>Task</h1>
    <p class="text-ink-muted">
      {loadError}
      <button type="button" class="ml-1 text-accent underline underline-offset-2" onclick={load}>Try again</button>
    </p>
  {:else if task === null || detail === null}
    <p class="text-ink-muted">Loading…</p>
  {:else}
    <a href={paths.project(task.project_id)} class="text-sm text-ink-muted hover:text-ink">← {projectTitle ?? 'Project'}</a>

    <header class="space-y-2">
      <h1 class="text-title font-semibold tracking-tight text-ink" {@attach pageHeading}>{task.title}</h1>
      <p class="flex flex-wrap gap-x-3 text-sm text-ink-muted">
        <span>{status}</span>
        {#if task.size}<span>Size {task.size}</span>{/if}
        {#if task.energy}<span>{task.energy} energy</span>{/if}
      </p>
      {#if task.notes}<p class="text-sm leading-relaxed whitespace-pre-wrap text-ink">{task.notes}</p>{/if}
    </header>

    <section class="space-y-2" aria-labelledby="task-origin">
      <h2 id="task-origin" class="font-medium text-ink">Where it came from</h2>
      {#if detail.origin_entry}
        <Card data-origin-entry={detail.origin_entry.id}>
          <p class="text-xs text-ink-faint">{when(detail.origin_entry.created_at)}</p>
          <p class="mt-1 text-sm leading-relaxed break-words whitespace-pre-wrap text-ink">{detail.origin_entry.body}</p>
        </Card>
      {:else}
        <p class="text-sm text-ink-muted">Added by hand.</p>
      {/if}
    </section>

    <section class="space-y-2" aria-labelledby="task-timeline">
      <h2 id="task-timeline" class="font-medium text-ink">Everything you've written about it</h2>
      {#if timeline.length === 0}
        <p class="text-sm text-ink-muted">Nothing yet. Diary entries that mention this task will gather here.</p>
      {:else}
        <ol class="space-y-3">
          {#each timeline as g (g.entryId)}
            <li data-entry-id={g.entryId}>
              <Card>
                <p class="text-xs text-ink-faint">{when(g.at)}</p>
                <ul class="mt-2 space-y-2">
                  {#each g.observations as o (o.id)}
                    <li data-observation-id={o.id} class="text-sm">
                      <span class="text-ink-muted">{kindLabel(o.kind)}:</span>
                      <span class="text-ink">{o.summary}</span>
                      {#if o.quote}<blockquote class="mt-1 border-l-2 border-accent/40 pl-2 text-ink-muted">“{o.quote}”</blockquote>{/if}
                    </li>
                  {/each}
                </ul>
              </Card>
            </li>
          {/each}
        </ol>
      {/if}
    </section>
  {/if}
</section>
