<script lang="ts">
  import Button from './ui/Button.svelte'
  import Card from './ui/Card.svelte'
  import { onMount } from 'svelte'
  import { ApiError, listProjectObservations } from '../lib/api/client'
  import type { LinkedObservation } from '../lib/api/types.gen'
  import { highlightClass, kindLabel } from '../lib/entries'
  import { on } from '../lib/events'
  import { latest, liveReload } from '../lib/live'
  import { paths } from '../lib/router'
  import { observationTimeline } from '../lib/tasks'

  let { projectId }: { projectId: number } = $props()

  // pageSize is how many entries a page covers; the server's largest page is 50.
  const pageSize = 10
  const maxPage = 50

  let observations = $state<LinkedObservation[] | null>(null)
  let nextBefore = $state<number | null>(null)
  let loadError = $state('')
  let moreError = $state('')
  let loadingMore = $state(false)

  const timeline = $derived(observationTimeline(observations ?? []))

  function when(at: string): string {
    return new Date(at).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' })
  }

  function message(err: unknown, fallback: string): string {
    return err instanceof ApiError ? err.message : fallback
  }

  const loads = latest()

  // load fetches from the newest entry again, as many entries as are showing,
  // so a live update doesn't fold away pages already opened.
  async function load() {
    const current = loads()
    const limit = Math.min(maxPage, Math.max(pageSize, timeline.length))
    try {
      const page = await listProjectObservations(projectId, { limit })
      if (!current()) return
      observations = page.observations
      nextBefore = page.next_before
      loadError = ''
      moreError = ''
    } catch (err) {
      if (current()) loadError = message(err, "Couldn't load what the diary says about this project.")
    }
  }

  async function loadMore() {
    if (nextBefore === null) return
    const current = loads()
    loadingMore = true
    try {
      const page = await listProjectObservations(projectId, { before: nextBefore, limit: pageSize })
      if (!current()) return
      observations = [...(observations ?? []), ...page.observations]
      nextBefore = page.next_before
      moreError = ''
    } catch (err) {
      if (current()) moreError = message(err, "Couldn't load older entries.")
    } finally {
      loadingMore = false
    }
  }

  onMount(() => {
    load()
    // A processed entry may be about this project even when it wasn't scoped to it.
    const offs = [liveReload(load, () => projectId), on('entry.processed', () => load())]
    return () => offs.forEach((off) => off())
  })
</script>

<section aria-labelledby="diary-heading" class="space-y-3">
  <h2 id="diary-heading" class="text-lg font-semibold">From the diary</h2>

  {#if loadError}
    <p class="text-ink-muted">
      {loadError}
      <Button variant="quiet" size="xs" class="ml-1" onclick={load}>Try again</Button>
    </p>
  {:else if observations === null}
    <p class="text-ink-muted">Loading…</p>
  {:else if timeline.length === 0}
    <p class="text-ink-muted">Nothing yet. What you write in the diary about this project will gather here.</p>
  {:else}
    <ol class="space-y-3">
      {#each timeline as g (g.entryId)}
        <li data-entry-id={g.entryId}>
          <Card>
            <p class="text-xs"><a href={paths.diaryEntry(g.entryId)} class="text-ink-faint hover:text-accent">{when(g.at)}</a></p>
            <ul class="mt-2 space-y-2">
              {#each g.observations as o (o.id)}
                <li data-observation-id={o.id} class="text-sm">
                  <span class="text-ink-muted">{kindLabel(o.kind)}:</span>
                  <span class="rounded px-1 text-ink {highlightClass(o.kind, false)}">{o.summary}</span>
                  {#if o.quote}<blockquote class="mt-1 border-l-2 border-accent/40 pl-2 font-diary text-ink-muted">“{o.quote}”</blockquote>{/if}
                </li>
              {/each}
            </ul>
          </Card>
        </li>
      {/each}
    </ol>
    {#if moreError}<p class="text-sm text-ink-muted">{moreError}</p>{/if}
    {#if nextBefore !== null}
      <Button variant="quiet" size="sm" onclick={loadMore} disabled={loadingMore}>Show more</Button>
    {/if}
  {/if}
</section>
