<script lang="ts">
  import type { Briefing, Project } from '../lib/api/types.gen'
  import { paths } from '../lib/router'
  import Card from './ui/Card.svelte'

  // One active project's briefing on the Now screen (SPEC §7.3): the one next action first, then
  // the context around it. briefing is null when it couldn't be loaded.
  let { project, briefing, error = '' }: { project: Project; briefing: Briefing | null; error?: string } = $props()
</script>

<article class="space-y-4" aria-labelledby="briefing-{project.id}">
  <h2 id="briefing-{project.id}" class="text-lg font-semibold tracking-tight">
    <a href={paths.project(project.id)} class="hover:text-accent">{project.title}</a>
  </h2>

  {#if error}
    <p class="text-sm text-ink-muted">{error}</p>
  {:else if briefing}
    {#if briefing.next_task}
      <Card glow>
        <p class="text-label font-medium tracking-wide text-accent uppercase">Next</p>
        <p class="mt-1 text-lg font-medium">
          <a href={paths.task(briefing.next_task.id)} class="hover:text-accent">{briefing.next_task.title}</a>
        </p>
        {#if briefing.next_reason}<p class="mt-2 text-sm text-ink-muted">{briefing.next_reason}</p>{/if}
        {#if briefing.excerpt}
          <blockquote class="mt-3 border-l-2 border-accent/40 pl-3 text-sm text-ink-muted italic">
            {briefing.excerpt.text}
          </blockquote>
        {/if}
      </Card>
    {:else}
      <Card>
        <p class="text-ink-muted">
          Nothing here is ready to start yet. Write a few lines in the diary about what could come next; one small step
          is enough.
        </p>
      </Card>
    {/if}

    {#if briefing.recap}
      <div>
        <h3 class="text-label font-medium tracking-wide text-ink-muted uppercase">Where you left off</h3>
        <p class="mt-1 text-sm">{briefing.recap}</p>
      </div>
    {/if}

    {#if briefing.blockers.length > 0}
      <div>
        <h3 class="text-label font-medium tracking-wide text-ink-muted uppercase">In the way</h3>
        <ul class="mt-1 list-disc space-y-1 pl-5 text-sm">
          {#each briefing.blockers as b (b.id)}<li>{b.description}</li>{/each}
        </ul>
      </div>
    {/if}

    {#if briefing.alternatives.length > 0}
      <div>
        <h3 class="text-label font-medium tracking-wide text-ink-muted uppercase">Low on energy? Try instead</h3>
        <ul class="mt-1 space-y-1 text-sm">
          {#each briefing.alternatives as t (t.id)}
            <li><a href={paths.task(t.id)} class="text-ink hover:text-accent">{t.title}</a></li>
          {/each}
        </ul>
      </div>
    {/if}
  {:else}
    <p class="text-sm text-ink-muted">Loading…</p>
  {/if}
</article>
