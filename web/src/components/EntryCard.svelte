<script lang="ts">
  import type { Question } from '../lib/api/types.gen'
  import { observationHref, observationLabel, pendingOpCount, runState, segments, shownQuestions } from '../lib/entries'
  import { paths } from '../lib/router'
  import type { TimelineItem } from '../lib/timeline.svelte'
  import FollowUpQuestion from './FollowUpQuestion.svelte'
  import Card from './ui/Card.svelte'

  // One diary entry in the timeline: when it was written, what it's about, its text with the parts
  // Waypoint picked up highlighted (each opens what it's about), and its follow-up questions: open ones
  // to answer or set aside, answered ones as a quiet line (the answer is a newer entry, above this one).

  let {
    item,
    projectTitle,
    replyTo,
  }: { item: TimelineItem; projectTitle: string | null; replyTo: Question | null } = $props()

  const entry = $derived(item.entry)
  const detail = $derived(item.detail)
  const segs = $derived(segments(entry.body, detail?.observations ?? []))
  const when = $derived(new Date(entry.created_at).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' }))
  const pending = $derived(detail ? pendingOpCount(detail) : 0)
  const questions = $derived(detail ? shownQuestions(detail) : [])
  const run = $derived(detail ? runState(detail) : 'done')
</script>

<Card class="space-y-3">
  <div class="flex flex-wrap items-baseline gap-x-2 text-xs text-ink-muted">
    <time datetime={entry.created_at}>{when}</time>
    {#if entry.project_id !== null}
      <span aria-hidden="true">·</span>
      <a href={paths.project(entry.project_id)} class="hover:text-ink">{projectTitle ?? 'Project'}</a>
    {/if}
  </div>

  {#if entry.reply_to_question_id !== null}
    <p class="text-xs text-ink-muted">
      {replyTo ? `Answering: ${replyTo.text}` : 'Answering an earlier question'}
    </p>
  {/if}

  <p class="text-sm leading-relaxed break-words whitespace-pre-wrap text-ink">{#each segs as s, i (i)}{@const href = s.observation && observationHref(s.observation)}{#if !s.observation}{s.text}{:else if href}<a {href} title={observationLabel(s.observation)} class="rounded-sm bg-accent/15 underline decoration-accent/50 underline-offset-2 hover:bg-accent/25">{s.text}</a>{:else}<mark title={observationLabel(s.observation)} class="rounded-sm bg-accent/15 text-ink">{s.text}</mark>{/if}{/each}</p>

  {#if item.error}
    <p class="text-xs text-ink-muted">{item.error}</p>
  {:else if run === 'reading'}
    <p class="text-xs text-ink-muted">Waypoint is reading this…</p>
  {:else if run === 'failed'}
    <p class="text-xs text-ink-muted">Waypoint couldn't read this entry. It's saved as written.</p>
  {/if}

  {#if pending > 0}
    <p class="text-xs text-ink-muted">{pending === 1 ? '1 proposed change' : `${pending} proposed changes`} to review</p>
  {/if}

  {#each questions as q (q.id)}
    {#if q.status === 'open'}
      <FollowUpQuestion question={q} {entry} />
    {:else}
      <p class="border-l-2 border-line pl-3 text-xs text-ink-muted">{q.text} · answered above</p>
    {/if}
  {/each}
</Card>
