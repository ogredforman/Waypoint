<script lang="ts">
  import { onMount } from 'svelte'
  import { ApiError, getJSON, postJSON } from '../lib/api/client'
  import type { CreateEntryResponse, Project } from '../lib/api/types.gen'
  import { entryRequest, isSubmitShortcut, knownScope, resolveScope, scopeOptions, screenProjectID } from '../lib/diary'
  import { clearDraft, draft, draftProfile, draftSession, restoreDraft } from '../lib/diaryDraft.svelte'
  import { latest, liveReload } from '../lib/live'
  import { current } from '../lib/navigation.svelte'
  import { addEntry } from '../lib/timeline.svelte'
  import ProjectPicker from './ProjectPicker.svelte'
  import Button from './ui/Button.svelte'
  import Textarea from './ui/Textarea.svelte'

  // The diary input: free text, an optional project it's about (the project on screen by
  // default), and Ctrl/Cmd+Enter to save. The draft lives in diaryDraft so it outlasts this copy.

  let projects = $state<Project[]>([])
  // loaded is set once the project list has loaded, so a scope it lacks is known to be gone.
  let loaded = $state(false)
  let saving = $state(false)
  let notice = $state('')
  let saved = $state(false)

  const scope = $derived(
    knownScope(resolveScope(draft.choice, current.path, screenProjectID(current.route)), loaded ? projects : null),
  )
  const options = $derived(scopeOptions(projects, scope))

  function pick(projectID: number | null) {
    draft.choice = { projectID, path: current.path }
  }

  async function save() {
    const req = entryRequest(draft.body, scope)
    if (!req || saving) return
    saving = true
    notice = ''
    saved = false
    // Clear the box straight away; if the save fails, put the text back so nothing is lost.
    const { body, choice } = draft
    // The profile it's written in: if the user switches while it saves, it
    // must not show up in, or come back to, the other profile's diary.
    const profile = draftProfile()
    const session = draftSession()
    clearDraft()
    try {
      const res = await postJSON<CreateEntryResponse>('/api/entries', req)
      saved = true
      if (draftSession() === session) addEntry(res.entry, res.run)
    } catch (err) {
      restoreDraft(profile, body, choice)
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

  // The picker follows projects created, renamed, or moved elsewhere (SPEC §10.5).
  // A failed load keeps the list it has; the entry can still be saved without a project.
  const loads = latest()
  async function load() {
    const fresh = loads()
    try {
      const list = await getJSON<Project[]>('/api/projects')
      if (!fresh()) return
      projects = list
      loaded = true
    } catch {
      // keep the list we have
    }
  }

  onMount(() => {
    load()
    return liveReload(load)
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
    tall
    class="font-diary"
    placeholder="What did you do, what's in the way, what's on your mind?"
    bind:value={draft.body}
    oninput={() => (saved = false)}
    {onkeydown}
  />

  <div class="flex flex-wrap items-center gap-2">
    <div class="flex min-w-0 flex-1 items-center gap-2 text-label text-ink-muted">
      <label for="diary-scope" class="shrink-0">About</label>
      <!-- Before the project list loads, a scope it lacks still shows what the entry will be about. -->
      <ProjectPicker
        id="diary-scope"
        class="min-w-0 flex-1"
        projects={options}
        value={scope}
        onpick={pick}
        missingLabel="This project"
      />
    </div>
    <Button type="submit" disabled={saving || draft.body.trim() === ''}>{saving ? 'Writing…' : 'Write Entry'}</Button>
  </div>

  <p class="text-xs text-ink-muted" aria-live="polite">
    {#if notice}
      {notice}
    {:else if saved}
      Saved. Waypoint is reading it now.
    {:else}
      Ctrl+Enter or ⌘+Enter to write it.
    {/if}
  </p>
</form>
