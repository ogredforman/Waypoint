<script lang="ts">
  import { ApiError, postJSON } from '../lib/api/client'
  import type { CreateEntryResponse, Entry, Question } from '../lib/api/types.gen'
  import { entryRequest, isSubmitShortcut } from '../lib/diary'
  import { addEntry, loadEntry } from '../lib/timeline.svelte'
  import Button from './ui/Button.svelte'
  import Textarea from './ui/Textarea.svelte'

  // One open follow-up question under the entry that raised it (SPEC §7.2). Answering writes a new
  // entry that replies to it, about the same project; "Not now" dismisses it without fuss.

  let { question, entry }: { question: Question; entry: Entry } = $props()

  let answering = $state(false)
  let body = $state('')
  let busy = $state(false)
  let notice = $state('')

  async function answer() {
    const req = entryRequest(body, entry.project_id)
    if (!req || busy) return
    busy = true
    notice = ''
    try {
      const res = await postJSON<CreateEntryResponse>('/api/entries', { ...req, reply_to_question_id: question.id })
      body = ''
      answering = false
      await Promise.all([addEntry(res.entry), loadEntry(entry.id)])
    } catch (err) {
      notice = err instanceof ApiError ? err.message : "Couldn't save your answer. It's still in the box."
    } finally {
      busy = false
    }
  }

  async function dismiss() {
    if (busy) return
    busy = true
    notice = ''
    try {
      await postJSON(`/api/questions/${question.id}/dismiss`, {})
      await loadEntry(entry.id)
    } catch (err) {
      notice = err instanceof ApiError ? err.message : "Couldn't set the question aside."
      // It may have been answered or dismissed elsewhere; show where it stands now.
      await loadEntry(entry.id)
    } finally {
      busy = false
    }
  }

  function onkeydown(e: KeyboardEvent) {
    if (!isSubmitShortcut(e)) return
    e.preventDefault()
    answer()
  }
</script>

<div class="space-y-2 border-l-2 border-accent/50 pl-3">
  <p class="text-sm text-ink">{question.text}</p>
  {#if answering}
    <form
      class="space-y-2"
      onsubmit={(e) => {
        e.preventDefault()
        answer()
      }}
    >
      <label for="answer-{question.id}" class="sr-only">Your answer</label>
      <Textarea id="answer-{question.id}" rows={3} class="resize-y text-sm" bind:value={body} {onkeydown} />
      <div class="flex flex-wrap gap-2">
        <Button type="submit" disabled={busy || body.trim() === ''}>{busy ? 'Saving…' : 'Save answer'}</Button>
        <Button variant="quiet" onclick={() => (answering = false)}>Cancel</Button>
      </div>
    </form>
  {:else}
    <div class="flex flex-wrap gap-2">
      <Button variant="secondary" onclick={() => (answering = true)} disabled={busy}>Answer</Button>
      <Button variant="quiet" onclick={dismiss} disabled={busy}>Not now</Button>
    </div>
  {/if}
  {#if notice}
    <p class="text-xs text-ink-muted" aria-live="polite">{notice}</p>
  {/if}
</div>
