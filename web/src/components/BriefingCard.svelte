<script lang="ts">
  import type { Briefing, Project } from '../lib/api/types.gen'
  import { relativeDay } from '../lib/dates'
  import { highlightClass, kindLabel } from '../lib/entries'
  import { paths } from '../lib/router'
  import Card from './ui/Card.svelte'

  // One active project's briefing on the Now screen (SPEC §7.3, §11), a block of its own: the one
  // next action first, then where you left off as a short dated list, then blockers and low-energy
  // alternatives, kept brief. briefing is null when it couldn't be loaded.
  let { project, briefing, error = '' }: { project: Project; briefing: Briefing | null; error?: string } = $props()

  const longDate = (at: string) => new Date(at).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' })
</script>

<article class="space-y-5 rounded-card border border-line bg-surface p-5" aria-labelledby="briefing-{project.id}">
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

    {#if briefing.recap.length > 0}
      <div>
        <h3 id="recap-{project.id}" class="text-label font-medium tracking-wide text-ink-muted uppercase">Where you left off</h3>
        <ol class="mt-2 space-y-3" aria-labelledby="recap-{project.id}">
          {#each briefing.recap as entry (entry.entry_id)}
            <li class="flex gap-3 text-sm">
              <time datetime={entry.created_at} title={longDate(entry.created_at)} class="w-24 shrink-0 text-ink-faint">
                {relativeDay(entry.created_at)}
              </time>
              <ul class="flex min-w-0 flex-wrap gap-1.5">
                {#each entry.points as point, i (i)}
                  <li class="rounded px-1.5 py-0.5 text-ink {highlightClass(point.kind, false)}" title={kindLabel(point.kind)}>
                    <span class="sr-only">{`${kindLabel(point.kind)}: `}</span>{point.text}
                  </li>
                {/each}
              </ul>
            </li>
          {/each}
        </ol>
      </div>
    {/if}

    {#if briefing.blockers.length > 0 || briefing.alternatives.length > 0}
      <div class="grid gap-4 text-sm sm:grid-cols-2">
        {#if briefing.blockers.length > 0}
          <div>
            <h3 id="blockers-{project.id}" class="text-label font-medium tracking-wide text-ink-muted uppercase">In the way</h3>
            <ul aria-labelledby="blockers-{project.id}" class="mt-1 list-disc space-y-1 pl-5 text-ink-muted">
              {#each briefing.blockers as b (b.id)}<li>{b.description}</li>{/each}
            </ul>
          </div>
        {/if}
        {#if briefing.alternatives.length > 0}
          <div>
            <h3 id="alternatives-{project.id}" class="text-label font-medium tracking-wide text-ink-muted uppercase">Low on energy? Try instead</h3>
            <ul aria-labelledby="alternatives-{project.id}" class="mt-1 space-y-1">
              {#each briefing.alternatives as t (t.id)}
                <li><a href={paths.task(t.id)} class="text-ink-muted hover:text-accent">{t.title}</a></li>
              {/each}
            </ul>
          </div>
        {/if}
      </div>
    {/if}
  {:else}
    <p class="text-sm text-ink-muted">Loading…</p>
  {/if}
</article>
