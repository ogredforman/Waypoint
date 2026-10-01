<script lang="ts">
  import { onMount } from 'svelte'
  import { ApiError, getJSON, postJSON } from '../lib/api/client'
  import type { CreateEntryResponse, Project } from '../lib/api/types.gen'
  import { entryRequest, isSubmitShortcut, resolveScope, scopeOptions, screenProjectID } from '../lib/diary'
  import { clearDraft, draft } from '../lib/diaryDraft.svelte'
  import { current } from '../lib/navigation.svelte'
  import { addEntry } from '../lib/timeline.svelte'
  import Button from './ui/Button.svelte'
  import Textarea from './ui/Textarea.svelte'
  import { fieldClass } from './ui/field'

  // The diary input: free text, an optional project it's about (the project on screen by
  // default), and Ctrl/Cmd+Enter to save. The draft lives in diaryDraft so it outlasts this copy.

  let projects = $state<Project[]>([])
  let saving = $state(false)
  let notice = $state('')
  let saved = $state(false)

  const scope = $derived(resolveScope(draft.choice, current.path, screenProjectID(current.route)))
  const options = $derived(scopeOptions(projects, scope))

  function pick(e: Event) {
    const v = (e.currentTarget as HTMLSelectElement).value
    draft.choice = { projectID: v === '' ? null : Number(v), path: current.path }
  }

  async function save() {
    const req = entryRequest(draft.body, scope)
    if (!req || saving) return
    saving = true
    notice = ''
    saved = false
    // Clear the box straight away; if the save fails, put the text back so nothing is lost.
    const { body, choice } = draft
    clearDraft()
    try {
      const res = await postJSON<CreateEntryResponse>('/api/entries', req)
      saved = true
      addEntry(res.entry)
    } catch (err) {
      if (draft.body.trim() === '') {
        draft.body = body
        draft.choice = choice
      }
      notice = err instanceof ApiError ? err.message : "Couldn't save the entry. Your text is back in the box."
    } finally {
      saving = false
    }
  }

  function onkeydown(e: KeyboardEvent) {
    if (!isSubmitShortcut(e)) return
    e.preventDefault()
    save()
  }

  onMount(async () => {
    try {
      projects = await getJSON<Project[]>('/api/projects')
    } catch {
      projects = [] // the entry can still be saved without a project
    }
  })
</script>

<form
  class="space-y-2"
  onsubmit={(e) => {
    e.preventDefault()
    save()
  }}
>
  <label for="diary-entry" class="sr-only">Diary entry</label>
  <Textarea
    id="diary-entry"
    rows={5}
    class="resize-y"
    placeholder="What did you do, what's in the way, what's on your mind?"
    bind:value={draft.body}
    oninput={() => (saved = false)}
    {onkeydown}
  />

  <div class="flex flex-wrap items-center gap-2">
    <label class="flex min-w-0 flex-1 items-center gap-2 text-label text-ink-muted">
      <span class="shrink-0">About</span>
      <select class={[fieldClass, 'min-w-0 py-1.5 text-sm']} value={scope === null ? '' : String(scope)} onchange={pick}>
        <option value="">No project</option>
        {#if scope !== null && !options.some((p) => p.id === scope)}
          <!-- The project list hasn't loaded (or failed to); still show what the entry will be about. -->
          <option value={String(scope)}>This project</option>
        {/if}
        {#each options as p (p.id)}
          <option value={String(p.id)}>{p.title}</option>
        {/each}
      </select>
    </label>
    <Button type="submit" disabled={saving || draft.body.trim() === ''}>{saving ? 'Saving…' : 'Save'}</Button>
  </div>

  <p class="text-xs text-ink-muted" aria-live="polite">
    {#if notice}
      {notice}
    {:else if saved}
      Saved. Waypoint is reading it now.
    {:else}
      Ctrl+Enter or ⌘+Enter to save.
    {/if}
  </p>
</form>
