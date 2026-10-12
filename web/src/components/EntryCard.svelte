<script lang="ts">
  import { ApiError } from '../lib/api/client'
  import type { Project, Question } from '../lib/api/types.gen'
  import {
    canReprocess,
    failureText,
    highlightClass,
    observationHref,
    observationLabel,
    pendingChangesets,
    reprocessImpact,
    reprocessWarning,
    runState,
    segments,
    shownQuestions,
  } from '../lib/entries'
  import { paths } from '../lib/router'
  import { deleteEntry, loadEntry, reprocessEntry, type TimelineItem } from '../lib/timeline.svelte'
  import ChangesetReview from './ChangesetReview.svelte'
  import FollowUpQuestion from './FollowUpQuestion.svelte'
  import Button from './ui/Button.svelte'
  import Card from './ui/Card.svelte'

  // One diary entry in the timeline: when it was written, what it's about, its text with the parts
  // Waypoint picked up highlighted (each opens what it's about), and its follow-up questions: open ones
  // to answer or set aside, answered ones as a quiet line (the answer is a newer entry, above this one).
  // Changes proposed from it wait underneath for review. It can be read again: right there when the
  // last read failed, behind "More" otherwise, after a calm word about what that replaces. "More" also
  // offers deleting it, after saying what goes with it (ADR 0007).

  let {
    item,
    projectTitle,
    replyTo,
    projects,
    highlighted = false,
  }: {
    item: TimelineItem
    projectTitle: string | null
    replyTo: Question | null
    projects: Project[]
    // highlighted marks the entry a /diary?entry=N link opened, for a moment.
    highlighted?: boolean
  } = $props()

  const entry = $derived(item.entry)
  const detail = $derived(item.detail)
  const segs = $derived(segments(entry.body, detail?.observations ?? []))
  const when = $derived(new Date(entry.created_at).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' }))
  const pending = $derived(detail ? pendingChangesets(detail) : [])
  const questions = $derived(detail ? shownQuestions(detail) : [])
  const run = $derived(detail ? runState(detail) : 'done')
  // What applying a review had to say after the review itself went away, e.g. ops held back by the cap.
  let reviewNotice = $state('')

  // Reading again. While confirming, the warning follows live updates (e.g. a review applied in
  // another tab), and a run that starts meanwhile ends the confirmation.
  let moreOpen = $state(false)
  let confirming = $state(false)
  let rereading = $state(false)
  let rereadNotice = $state('')
  const warning = $derived(canReprocess(detail) ? reprocessWarning(reprocessImpact(detail)) : null)
  const asking = $derived(confirming && canReprocess(detail))
  $effect(() => {
    if (run === 'reading') confirming = false
  })

  function readAgain() {
    if (!canReprocess(detail)) return
    rereadNotice = ''
    if (warning) confirming = true
    else void confirmReadAgain()
  }

  function keep() {
    confirming = false
    moreOpen = false
  }

  // Deleting. Not offered while Waypoint is reading the entry; the server refuses then too.
  let confirmingDelete = $state(false)
  let deleting = $state(false)
  let deleteNotice = $state('')
  const askingDelete = $derived(confirmingDelete && run !== 'reading')

  function askDelete() {
    deleteNotice = ''
    rereadNotice = ''
    confirmingDelete = true
  }

  function keepEntry() {
    confirmingDelete = false
    moreOpen = false
  }

  async function confirmDelete() {
    deleting = true
    try {
      await deleteEntry(entry.id)
    } catch (err) {
      keepEntry()
      if (err instanceof ApiError && err.status === 409) {
        deleteNotice = "Waypoint is reading this entry. It can be deleted once that's done."
        void loadEntry(entry.id)
      } else {
        deleteNotice = err instanceof ApiError ? err.message : "Couldn't delete this entry."
      }
    } finally {
      deleting = false
    }
  }

  async function confirmReadAgain() {
    rereading = true
    try {
      await reprocessEntry(entry.id)
      keep()
    } catch (err) {
      confirming = false
      if (err instanceof ApiError && err.status === 409) {
        rereadNotice = 'Waypoint is already reading this entry.'
        void loadEntry(entry.id)
      } else {
        rereadNotice = err instanceof ApiError ? err.message : "Couldn't ask Waypoint to read this again."
      }
    } finally {
      rereading = false
    }
  }
</script>

<Card class={['space-y-3 motion-safe:transition-shadow motion-safe:duration-700', highlighted && 'ring-2 ring-accent']}>
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

  <p class="font-diary text-sm leading-relaxed break-words whitespace-pre-wrap text-ink">{#each segs as s, i (i)}{@const href = s.observation && observationHref(s.observation)}{#if !s.observation}{s.text}{:else if href}<a {href} title={observationLabel(s.observation)} class="rounded-sm underline underline-offset-2 {highlightClass(s.observation.kind, true)}">{s.text}</a>{:else}<mark title={observationLabel(s.observation)} class="rounded-sm text-ink {highlightClass(s.observation.kind, false)}">{s.text}</mark>{/if}{/each}</p>

  {#if item.error}
    <p class="text-xs text-ink-muted">{item.error}</p>
  {:else if run === 'reading'}
    <p class="flex items-center gap-2 text-xs text-ink-muted">
      <span class="size-1.5 rounded-full bg-accent motion-safe:animate-pulse" aria-hidden="true"></span>
      Waypoint is reading this…
    </p>
  {:else if run === 'failed'}
    <div class="flex flex-wrap items-center gap-2 text-xs text-ink-muted">
      <p>{failureText(detail?.run ?? null)}</p>
      {#if !asking}
        <Button variant="secondary" size="xs" disabled={rereading} onclick={readAgain}>Read again</Button>
      {/if}
    </div>
  {/if}
  {#if run !== 'reading' && !asking && !askingDelete}
    <div class="flex items-center gap-1 text-xs">
      <Button variant="quiet" size="xs" aria-expanded={moreOpen} onclick={() => (moreOpen = !moreOpen)}>More</Button>
      {#if moreOpen}
        {#if canReprocess(detail) && run !== 'failed'}
          <Button variant="quiet" size="xs" disabled={rereading} onclick={readAgain}>Read again</Button>
        {/if}
        <Button variant="danger" size="xs" onclick={askDelete}>Delete entry</Button>
      {/if}
    </div>
  {/if}

  {#if asking}
    <div class="flex flex-wrap items-center gap-2 text-sm" role="group" aria-label="Confirm reading again">
      <span class="text-ink-muted">{warning ?? 'Read this entry again?'}</span>
      <Button variant="secondary" size="sm" disabled={rereading} onclick={confirmReadAgain}>Read again</Button>
      <Button variant="quiet" size="sm" onclick={keep}>Keep these</Button>
    </div>
  {/if}
  {#if rereadNotice}<p class="text-xs text-ink-muted" role="status">{rereadNotice}</p>{/if}

  {#if askingDelete}
    <div class="flex flex-wrap items-center gap-2 text-sm" role="group" aria-label="Confirm deleting">
      <span class="text-ink-muted">
        Delete this entry? Its highlights, questions, and changes not yet reviewed go with it; changes you already
        applied stay. This can't be undone.
      </span>
      <Button variant="danger" size="sm" disabled={deleting} onclick={confirmDelete}>Delete</Button>
      <Button variant="quiet" size="sm" onclick={keepEntry}>Keep it</Button>
    </div>
  {/if}
  {#if deleteNotice}<p class="text-xs text-ink-muted" role="status">{deleteNotice}</p>{/if}

  {#each pending as cs (cs.id)}
    <ChangesetReview changeset={cs} {entry} {projects} onnotice={(m) => (reviewNotice = m)} />
  {/each}
  {#if reviewNotice}
    <p class="text-xs text-ink-muted" aria-live="polite">{reviewNotice}</p>
  {/if}

  {#each questions as q (q.id)}
    {#if q.status === 'open'}
      <FollowUpQuestion question={q} {entry} />
    {:else}
      <p class="border-l-2 border-line pl-3 text-xs text-ink-muted">{q.text} · answered above</p>
    {/if}
  {/each}
</Card>
