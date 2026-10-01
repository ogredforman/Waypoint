<script lang="ts">
  import { onMount } from 'svelte'
  import { repliedQuestion } from '../lib/entries'
  import { loadOlder, loadTimeline, timeline } from '../lib/timeline.svelte'
  import EntryCard from './EntryCard.svelte'
  import Button from './ui/Button.svelte'

  // The diary's entries, newest first, a page at a time. The list itself lives in the shared
  // timeline module; this copy only shows it and reloads it when it opens.

  const titles = $derived(new Map(timeline.projects.map((p) => [p.id, p.title])))
  const details = $derived(timeline.items.map((i) => i.detail))

  onMount(loadTimeline)
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
  <ol class="space-y-4" aria-label="Diary entries">
    {#each timeline.items as item (item.entry.id)}
      <li>
        <EntryCard
          {item}
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
