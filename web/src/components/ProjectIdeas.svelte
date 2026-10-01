<script lang="ts">
  import { onMount } from 'svelte'
  import { ApiError, getJSON } from '../lib/api/client'
  import type { Idea } from '../lib/api/types.gen'
  import Panel from './ui/Panel.svelte'

  let { projectId }: { projectId: number } = $props()

  // Parked ideas only (the endpoint's default). Promote and delete come with the Parking lot.
  let ideas = $state<Idea[] | null>(null)
  let loadError = $state('')

  async function load() {
    loadError = ''
    try {
      ideas = await getJSON<Idea[]>(`/api/ideas?project_id=${projectId}`)
    } catch (err) {
      loadError = err instanceof ApiError ? err.message : "Couldn't load the ideas."
    }
  }

  onMount(load)
</script>

<section aria-labelledby="ideas-heading" class="space-y-3">
  <h2 id="ideas-heading" class="text-lg font-semibold">Ideas</h2>

  {#if loadError}
    <p class="text-ink-muted">
      {loadError}
      <button type="button" class="ml-1 text-accent underline underline-offset-2" onclick={load}>Try again</button>
    </p>
  {:else if ideas === null}
    <p class="text-ink-muted">Loading…</p>
  {:else if ideas.length === 0}
    <p class="text-ink-muted">No parked ideas for this project.</p>
  {:else}
    <Panel>
      <ul class="divide-y divide-line" aria-label="Parked ideas">
        {#each ideas as idea (idea.id)}
          <li class="px-4 py-3">
            <p class="text-ink">{idea.title}</p>
            {#if idea.body}<p class="mt-0.5 text-sm whitespace-pre-line text-ink-muted">{idea.body}</p>{/if}
          </li>
        {/each}
      </ul>
    </Panel>
  {/if}
</section>
