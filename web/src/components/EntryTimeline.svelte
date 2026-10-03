<script lang="ts">
  import { onMount, tick, untrack } from 'svelte'
  import { repliedQuestion } from '../lib/entries'
  import { loadOlder, loadThrough, loadTimeline, timeline } from '../lib/timeline.svelte'
  import EntryCard from './EntryCard.svelte'
  import Button from './ui/Button.svelte'

  // The diary's entries, newest first, a page at a time. The list itself lives in the shared
  // timeline module; this copy only shows it and reloads it when it opens. With focusEntry (from
  // /diary?entry=N) it loads back as far as that entry, scrolls to it, and highlights it briefly.

  let { focusEntry = null }: { focusEntry?: number | null } = $props()

  // highlightFor is how long a linked entry stays highlighted.
  const highlightFor = 2000

  let list = $state<HTMLOListElement>()
  let highlighted = $state<number | null>(null)
  let missing = $state(false)

  const titles = $derived(new Map(timeline.projects.map((p) => [p.id, p.title])))
  const details = $derived(timeline.items.map((i) => i.detail))

  onMount(() => {
    if (focusEntry === null) loadTimeline()
  })

  $effect(() => {
    const id = focusEntry
    if (id === null) return
    let stale = false
    let timer: ReturnType<typeof setTimeout> | undefined
    missing = false
    // untrack: the loads read the timeline, which mustn't rerun this.
    untrack(() => loadThrough(id)).then(async (result) => {
      if (stale) return
      if (result !== 'found') {
        missing = result === 'absent'
        return
      }
      await tick()
      if (stale) return
      list?.querySelector(`[data-entry-id="${id}"]`)?.scrollIntoView({ block: 'center' })
      highlighted = id
      timer = setTimeout(() => {
        if (highlighted === id) highlighted = null
      }, highlightFor)
    })
    return () => {
      stale = true
      clearTimeout(timer)
      highlighted = null
    }
  })
</script>

{#if timeline.status === 'error'}
  <p class="text-sm text-ink-muted">
    {timeline.error}
    <Button variant="quiet" size="xs" class="ml-1" onclick={loadTimeline}>Try again</Button>
  </p>
{:else if timeline.status !== 'ready' && timeline.items.length === 0}
  <p class="text-sm text-ink-muted">Loading…</p>
{:else if timeline.items.length === 0}
  <div class="space-y-3 text-sm text-ink-muted">
    <p>Write about what you did, what's in the way, or what you're thinking. Waypoint will turn it into tasks you can review.</p>
    <p>Your entries will line up here, newest first.</p>
  </div>
{:else}
  {#if missing}
    <p class="text-sm text-ink-muted">That entry isn't in your diary. Here are the ones that are.</p>
  {/if}
  <ol class="space-y-4" aria-label="Diary entries" bind:this={list}>
    {#each timeline.items as item (item.entry.id)}
      <li data-entry-id={item.entry.id}>
        <EntryCard
          {item}
          highlighted={highlighted === item.entry.id}
          projectTitle={item.entry.project_id === null ? null : (titles.get(item.entry.project_id) ?? null)}
          replyTo={repliedQuestion(item.entry, details)}
          projects={timeline.projects}
        />
      </li>
    {/each}
  </ol>
  {#if timeline.error}
    <p class="text-sm text-ink-muted" aria-live="polite">{timeline.error}</p>
  {/if}
  {#if timeline.nextBefore !== null}
    <Button variant="secondary" onclick={loadOlder} disabled={timeline.loadingOlder}>
      {timeline.loadingOlder ? 'Loading…' : 'Show older entries'}
    </Button>
  {/if}
{/if}
