<script lang="ts">
  import { onMount } from 'svelte'
  import { ApiError, getJSON } from '../lib/api/client'
  import type { Blocker } from '../lib/api/types.gen'
  import { splitBlockers } from '../lib/blockers'
  import BlockerRow from './BlockerRow.svelte'
  import Panel from './ui/Panel.svelte'

  let { projectId }: { projectId: number } = $props()

  let blockers = $state<Blocker[] | null>(null)
  let loadError = $state('')

  const groups = $derived(blockers === null ? null : splitBlockers(blockers))

  async function load() {
    loadError = ''
    try {
      blockers = await getJSON<Blocker[]>(`/api/projects/${projectId}/blockers`)
    } catch (err) {
      loadError = err instanceof ApiError ? err.message : "Couldn't load the blockers."
    }
  }

  function replace(b: Blocker) {
    blockers = (blockers ?? []).map((x) => (x.id === b.id ? b : x))
  }

  function removed(id: number) {
    blockers = (blockers ?? []).filter((x) => x.id !== id)
  }

  onMount(load)
</script>

<section aria-labelledby="blockers-heading" class="space-y-3">
  <h2 id="blockers-heading" class="text-lg font-semibold">Blockers</h2>

  {#if loadError}
    <p class="text-ink-muted">
      {loadError}
      <button type="button" class="ml-1 text-accent underline underline-offset-2" onclick={load}>Try again</button>
    </p>
  {:else if groups === null}
    <p class="text-ink-muted">Loading…</p>
  {:else}
    {#if groups.open.length === 0}
      <p class="text-ink-muted">Nothing in the way.</p>
    {:else}
      <div>
        <h3 class="mb-1 text-label font-medium tracking-wide text-ink-muted uppercase">Open</h3>
        <Panel>
          <ul class="divide-y divide-line" aria-label="Open">
            {#each groups.open as b (b.id)}
              <BlockerRow blocker={b} onchanged={replace} ondeleted={removed} />
            {/each}
          </ul>
        </Panel>
      </div>
    {/if}
    {#if groups.resolved.length > 0}
      <div>
        <h3 class="mb-1 text-label font-medium tracking-wide text-ink-muted uppercase">Resolved</h3>
        <Panel>
          <ul class="divide-y divide-line" aria-label="Resolved">
            {#each groups.resolved as b (b.id)}
              <BlockerRow blocker={b} onchanged={replace} ondeleted={removed} />
            {/each}
          </ul>
        </Panel>
      </div>
    {/if}
  {/if}
</section>
