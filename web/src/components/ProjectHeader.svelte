<script lang="ts">
  import { ApiError, patchJSON } from '../lib/api/client'
  import type { Project } from '../lib/api/types.gen'
  import { pageHeading } from '../lib/navigation.svelte'
  import { projectFields, projectPatch } from '../lib/projects'
  import Button from './ui/Button.svelte'
  import Card from './ui/Card.svelte'
  import Input from './ui/Input.svelte'
  import Textarea from './ui/Textarea.svelte'

  let { project, onsaved }: { project: Project; onsaved: (p: Project) => void } = $props()

  let editing = $state(false)
  let title = $state('')
  let why = $state('')
  let definitionOfDone = $state('')
  let saving = $state(false)
  let problems = $state<Record<string, string>>({})
  let notice = $state('')

  const shownFields = ['title', 'why', 'definition_of_done']

  function edit() {
    const f = projectFields(project)
    title = f.title
    why = f.why
    definitionOfDone = f.definitionOfDone
    problems = {}
    notice = ''
    editing = true
  }

  async function save(e: SubmitEvent) {
    e.preventDefault()
    problems = {}
    notice = ''
    if (title.trim() === '') {
      problems = { title: 'Give it a short title.' }
      return
    }
    const patch = projectPatch(project, { title, why, definitionOfDone })
    if (patch === null) {
      editing = false
      return
    }
    saving = true
    try {
      onsaved(await patchJSON<Project>(`/api/projects/${project.id}`, patch))
      editing = false
    } catch (err) {
      if (err instanceof ApiError && err.code === 'invalid' && err.problems.length > 0) {
        problems = Object.fromEntries(err.problems.map((p) => [p.field, p.problem]))
        // Problems on fields the form doesn't show must still be seen.
        if (err.problems.some((p) => !shownFields.includes(p.field))) notice = err.message
      } else {
        notice = err instanceof ApiError ? err.message : "Couldn't save the project."
      }
    } finally {
      saving = false
    }
  }
</script>

{#if editing}
  <!-- The screen keeps its <h1> while the title is being edited. -->
  <h1 class="sr-only">{project.title}</h1>
  <Card>
    <form class="space-y-4" onsubmit={save} novalidate>
      <div>
        <label for="edit-project-title" class="text-sm font-medium text-ink">Title</label>
        <Input
          id="edit-project-title"
          bind:value={title}
          autofocus
          autocomplete="off"
          aria-invalid={problems.title ? 'true' : undefined}
          aria-describedby={problems.title ? 'edit-project-title-problem' : undefined}
        />
        {#if problems.title}
          <p id="edit-project-title-problem" class="mt-1 text-sm text-accent">{problems.title}</p>
        {/if}
      </div>

      <div>
        <label for="edit-project-why" class="text-sm font-medium text-ink">Why it matters</label>
        <Textarea id="edit-project-why" rows={2} bind:value={why} />
        {#if problems.why}<p class="mt-1 text-sm text-accent">{problems.why}</p>{/if}
      </div>

      <div>
        <label for="edit-project-dod" class="text-sm font-medium text-ink">What "done" looks like</label>
        <Textarea id="edit-project-dod" rows={2} bind:value={definitionOfDone} />
        {#if problems.definition_of_done}<p class="mt-1 text-sm text-accent">{problems.definition_of_done}</p>{/if}
      </div>

      {#if notice}
        <p class="text-sm text-accent" role="status">{notice}</p>
      {/if}

      <div class="flex gap-2">
        <Button type="submit" disabled={saving}>{saving ? 'Saving…' : 'Save'}</Button>
        <Button variant="quiet" onclick={() => (editing = false)}>Cancel</Button>
      </div>
    </form>
  </Card>
{:else}
  <header>
    <div class="flex items-start justify-between gap-4">
      <h1 class="text-title font-semibold tracking-tight" {@attach pageHeading}>{project.title}</h1>
      <Button variant="secondary" onclick={edit}>Edit</Button>
    </div>
    <dl class="mt-3 space-y-3">
      <div>
        <dt class="text-label font-medium tracking-wide text-ink-muted uppercase">Why it matters</dt>
        <dd class="mt-0.5 whitespace-pre-line text-ink">
          {#if project.why}{project.why}{:else}<span class="text-ink-faint">Not written yet.</span>{/if}
        </dd>
      </div>
      <div>
        <dt class="text-label font-medium tracking-wide text-ink-muted uppercase">What "done" looks like</dt>
        <dd class="mt-0.5 whitespace-pre-line text-ink">
          {#if project.definition_of_done}{project.definition_of_done}{:else}<span class="text-ink-faint">Not written yet.</span>{/if}
        </dd>
      </div>
    </dl>
  </header>
{/if}
