<script lang="ts">
  import { ApiError, postJSON } from '../lib/api/client'
  import type { Project } from '../lib/api/types.gen'
  import { atCap, capMessage, newProjectRequest } from '../lib/projects'
  import Button from './ui/Button.svelte'
  import Card from './ui/Card.svelte'
  import Input from './ui/Input.svelte'
  import RichTextEditor from './ui/RichTextEditor.svelte'

  let {
    activeCount,
    cap,
    oncreated,
    oncancel,
    startActive: initiallyActive = false,
  }: {
    activeCount: number
    cap: number | null
    oncreated: (p: Project) => void
    oncancel?: () => void
    // startActive is the start chosen when the form opens (and after a create).
    startActive?: boolean
  } = $props()

  let title = $state('')
  let why = $state('')
  let definitionOfDone = $state('')
  // svelte-ignore state_referenced_locally
  let startActive = $state(initiallyActive)
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
      startActive = initiallyActive
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

</script>

<Card>
  <form class="space-y-4" onsubmit={submit} novalidate>
    <div>
      <label for="project-title" class="text-sm font-medium text-ink">Title</label>
      <Input
        id="project-title"
        bind:value={title}
        autofocus
        autocomplete="off"
        aria-invalid={problems.title ? 'true' : undefined}
        aria-describedby={problems.title ? 'project-title-problem' : undefined}
      />
      {#if problems.title}
        <p id="project-title-problem" class="mt-1 text-sm text-accent">{problems.title}</p>
      {/if}
    </div>

    <div>
      <span id="project-why-label" class="text-sm font-medium text-ink">Why it matters <span class="font-normal text-ink-muted">(optional)</span></span>
      <RichTextEditor
        id="project-why"
        labelledby="project-why-label"
        bind:value={why}
        invalid={!!problems.why}
        describedby={problems.why ? 'project-why-problem' : undefined}
      />
      {#if problems.why}<p id="project-why-problem" class="mt-1 text-sm text-accent">{problems.why}</p>{/if}
    </div>

    <div>
      <span id="project-dod-label" class="text-sm font-medium text-ink">
        What "done" looks like <span class="font-normal text-ink-muted">(optional)</span>
      </span>
      <RichTextEditor
        id="project-dod"
        labelledby="project-dod-label"
        bind:value={definitionOfDone}
        invalid={!!problems.definition_of_done}
        describedby={problems.definition_of_done ? 'project-dod-problem' : undefined}
      />
      {#if problems.definition_of_done}
        <p id="project-dod-problem" class="mt-1 text-sm text-accent">{problems.definition_of_done}</p>
      {/if}
    </div>

    <fieldset>
      <legend class="text-sm font-medium text-ink">Start</legend>
      <div class="mt-1 space-y-1">
        <label class="flex items-center gap-2 text-ink">
          <input type="radio" name="project-start" value={false} bind:group={startActive} />
          Park it for now
        </label>
        <label class={['flex items-center gap-2', full ? 'text-ink-faint' : 'text-ink']}>
          <input type="radio" name="project-start" value={true} bind:group={startActive} disabled={full} />
          Make it active and start working on it
        </label>
      </div>
      {#if full && !notice && cap !== null}
        <p class="mt-2 text-sm text-ink-muted">{capMessage(activeCount, cap)}</p>
      {/if}
    </fieldset>

    {#if notice}
      <p class="text-sm text-accent" role="status">{notice}</p>
    {/if}

    <div class="flex gap-2">
      <Button type="submit" disabled={saving}>{saving ? 'Creating…' : 'Create project'}</Button>
      {#if oncancel}
        <Button variant="quiet" onclick={oncancel}>Cancel</Button>
      {/if}
    </div>
  </form>
</Card>
