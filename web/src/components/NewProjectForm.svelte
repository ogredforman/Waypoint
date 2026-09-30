<script lang="ts">
  import { ApiError, postJSON } from '../lib/api/client'
  import type { Project } from '../lib/api/types.gen'
  import { atCap, capMessage, newProjectRequest } from '../lib/projects'

  let {
    activeCount,
    cap,
    oncreated,
    oncancel,
  }: {
    activeCount: number
    cap: number | null
    oncreated: (p: Project) => void
    oncancel?: () => void
  } = $props()

  let title = $state('')
  let why = $state('')
  let definitionOfDone = $state('')
  let startActive = $state(false)
  let saving = $state(false)
  let problems = $state<Record<string, string>>({})
  let notice = $state('')
  // capHit is set when the server turned down an active start the client thought had room.
  let capHit = $state(false)

  const shownFields = ['title', 'why', 'definition_of_done']

  const full = $derived(capHit || atCap(activeCount, cap))

  $effect(() => {
    if (full) startActive = false
  })

  async function submit(e: SubmitEvent) {
    e.preventDefault()
    problems = {}
    notice = ''
    if (title.trim() === '') {
      problems = { title: 'Give it a short title.' }
      return
    }
    saving = true
    try {
      const p = await postJSON<Project>('/api/projects', newProjectRequest({ title, why, definitionOfDone, startActive }))
      title = why = definitionOfDone = ''
      startActive = false
      capHit = false
      oncreated(p)
    } catch (err) {
      if (err instanceof ApiError && err.code === 'active_cap') {
        capHit = true
        notice = "You've reached the active-project limit, so this one is set to start parked. Create it when you're ready."
      } else if (err instanceof ApiError && err.code === 'invalid' && err.problems.length > 0) {
        problems = Object.fromEntries(err.problems.map((p) => [p.field, p.problem]))
        // Problems on fields the form doesn't show must still be seen.
        if (err.problems.some((p) => !shownFields.includes(p.field))) notice = err.message
      } else {
        notice = err instanceof ApiError ? err.message : "Couldn't create the project."
      }
    } finally {
      saving = false
    }
  }

  const inputClass =
    'mt-1 w-full rounded-md border border-stone-300 bg-white px-3 py-2 text-stone-900 focus:border-stone-500 focus:outline-none'
</script>

<form class="space-y-4 rounded-lg border border-stone-200 bg-stone-50 p-4" onsubmit={submit} novalidate>
  <div>
    <label for="project-title" class="text-sm font-medium text-stone-700">Title</label>
    <!-- svelte-ignore a11y_autofocus -->
    <input
      id="project-title"
      class={inputClass}
      bind:value={title}
      autofocus
      autocomplete="off"
      aria-invalid={problems.title ? 'true' : undefined}
      aria-describedby={problems.title ? 'project-title-problem' : undefined}
    />
    {#if problems.title}
      <p id="project-title-problem" class="mt-1 text-sm text-amber-800">{problems.title}</p>
    {/if}
  </div>

  <div>
    <label for="project-why" class="text-sm font-medium text-stone-700">Why it matters <span class="font-normal text-stone-500">(optional)</span></label>
    <textarea id="project-why" class={inputClass} rows="2" bind:value={why}></textarea>
    {#if problems.why}<p class="mt-1 text-sm text-amber-800">{problems.why}</p>{/if}
  </div>

  <div>
    <label for="project-dod" class="text-sm font-medium text-stone-700">
      What "done" looks like <span class="font-normal text-stone-500">(optional)</span>
    </label>
    <textarea id="project-dod" class={inputClass} rows="2" bind:value={definitionOfDone}></textarea>
    {#if problems.definition_of_done}<p class="mt-1 text-sm text-amber-800">{problems.definition_of_done}</p>{/if}
  </div>

  <fieldset>
    <legend class="text-sm font-medium text-stone-700">Start</legend>
    <div class="mt-1 space-y-1">
      <label class="flex items-center gap-2 text-stone-800">
        <input type="radio" name="project-start" value={false} bind:group={startActive} />
        Park it for now
      </label>
      <label class="flex items-center gap-2" class:text-stone-800={!full} class:text-stone-400={full}>
        <input type="radio" name="project-start" value={true} bind:group={startActive} disabled={full} />
        Make it active and start working on it
      </label>
    </div>
    {#if full && !notice && cap !== null}
      <p class="mt-2 text-sm text-stone-600">{capMessage(activeCount, cap)}</p>
    {/if}
  </fieldset>

  {#if notice}
    <p class="text-sm text-amber-800" role="status">{notice}</p>
  {/if}

  <div class="flex gap-2">
    <button type="submit" class="rounded-md bg-stone-800 px-4 py-2 text-sm font-medium text-white hover:bg-stone-700 disabled:opacity-60" disabled={saving}>
      {saving ? 'Creating…' : 'Create project'}
    </button>
    {#if oncancel}
      <button type="button" class="rounded-md px-4 py-2 text-sm text-stone-600 hover:text-stone-900" onclick={oncancel}>Cancel</button>
    {/if}
  </div>
</form>
