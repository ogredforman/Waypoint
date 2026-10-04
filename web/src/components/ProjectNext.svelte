<script lang="ts">
  import { stepLabel } from '../lib/tasks'
  import Button from './ui/Button.svelte'
  import Card from './ui/Card.svelte'
  import { onMount } from 'svelte'
  import { ApiError, getBriefing } from '../lib/api/client'
  import type { Briefing } from '../lib/api/types.gen'
  import { on, onReconnect } from '../lib/events'
  import { latest } from '../lib/live'
  import { paths } from '../lib/router'

  // The project's one next action, from its briefing (SPEC §7.3), at the top of the Project screen.
  let { projectId }: { projectId: number } = $props()

  let briefing = $state<Briefing | null>(null)
  let loadError = $state('')

  const loads = latest()

  async function load() {
    const current = loads()
    try {
      const b = await getBriefing(projectId)
      if (!current()) return
      briefing = b
      loadError = ''
    } catch (err) {
      if (current()) loadError = err instanceof ApiError ? err.message : "Couldn't load the next step."
    }
  }

  onMount(() => {
    load()
    // The server refreshes the briefing after every change that could move it.
    const offs = [
      on('briefing.updated', (e) => {
        if (e.project_id === projectId) load()
      }),
      onReconnect(load),
    ]
    return () => offs.forEach((off) => off())
  })
</script>

<section aria-labelledby="next-heading">
  <h2 id="next-heading" class="sr-only">Next step</h2>
  {#if loadError}
    <p class="text-ink-muted">
      {loadError}
      <Button variant="quiet" size="xs" class="ml-1" onclick={load}>Try again</Button>
    </p>
  {:else if briefing === null}
    <p class="text-ink-muted">Loading…</p>
  {:else if briefing.next_task}
    <Card glow>
      <p class="text-label font-medium tracking-wide text-accent uppercase">Next</p>
      <p class="mt-1 text-lg font-medium">
        <a href={paths.task(briefing.next_task.id)} class="hover:text-accent">{briefing.next_task.title}</a>
      </p>
      {#if briefing.next_step}
        <p class="mt-1 text-sm text-ink-muted">
          {stepLabel(briefing.next_step)}
          <a href={paths.task(briefing.next_step.parent_task_id)} class="text-ink hover:text-accent">{briefing.next_step.parent_title}</a>
        </p>
      {/if}
      {#if briefing.next_reason}<p class="mt-2 text-sm text-ink-muted">{briefing.next_reason}</p>{/if}
    </Card>
  {:else}
    <Card>
      <p class="text-ink-muted">
        Nothing here is ready to start yet. A few lines in the diary about what could come next are enough; one small
        step will do.
      </p>
    </Card>
  {/if}
</section>
