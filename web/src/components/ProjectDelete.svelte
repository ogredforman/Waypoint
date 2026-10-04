<script lang="ts">
  import { ApiError, deleteProject, getProjectDeletion } from '../lib/api/client'
  import type { Project, ProjectDeletion } from '../lib/api/types.gen'
  import { navigate } from '../lib/navigation.svelte'
  import { deletionSummary, entriesLabel } from '../lib/projects'
  import { paths } from '../lib/router'
  import Button from './ui/Button.svelte'

  // Deleting the project (ADR 0008): the confirmation says what goes with it
  // and asks about its diary entries, kept (unscoped) by default.

  let { project }: { project: Project } = $props()

  let confirming = $state(false)
  let preview = $state<ProjectDeletion | null>(null)
  let entries = $state<'keep' | 'delete'>('keep')
  let busy = $state(false)
  let notice = $state('')

  async function start() {
    if (busy) return
    notice = ''
    busy = true
    try {
      preview = await getProjectDeletion(project.id)
      entries = 'keep'
      confirming = true
    } catch (err) {
      notice = err instanceof ApiError ? err.message : "Couldn't check what's in this project."
    } finally {
      busy = false
    }
  }

  async function remove() {
    if (busy || !preview) return
    notice = ''
    busy = true
    try {
      await deleteProject(project.id, preview.entries > 0 ? entries : 'keep')
      navigate(paths.projects)
    } catch (err) {
      if (err instanceof ApiError && err.code === 'not_found') {
        navigate(paths.projects) // already deleted, perhaps in another tab
      } else if (err instanceof ApiError && err.code === 'run_in_progress') {
        notice = 'Waypoint is still reading one of its diary entries. Try again in a moment, or keep the entries.'
      } else {
        notice = err instanceof ApiError ? err.message : "Couldn't delete the project."
      }
    } finally {
      busy = false
    }
  }

  function cancel() {
    confirming = false
    notice = ''
  }
</script>

<section aria-label="Delete project" class="border-t border-line pt-4">
  {#if !confirming}
    <Button variant="danger" size="sm" disabled={busy} onclick={start}>Delete project…</Button>
  {:else if preview}
    <div class="space-y-3 text-sm" role="group" aria-label="Confirm delete">
      <p class="text-ink">
        Delete <strong>{project.title}</strong>? {deletionSummary(preview)} This can't be undone.
      </p>
      {#if preview.entries > 0}
        <fieldset class="space-y-1">
          <legend class="mb-1 text-ink-muted">What about {entriesLabel(preview.entries)}?</legend>
          <label class="flex items-center gap-2 text-ink">
            <input type="radio" name="project-entries" value="keep" bind:group={entries} />
            Keep {preview.entries === 1 ? 'it' : 'them'} in the diary, not tied to a project
          </label>
          <label class="flex items-center gap-2 text-ink">
            <input type="radio" name="project-entries" value="delete" bind:group={entries} />
            Delete {preview.entries === 1 ? 'it' : 'them'} too
          </label>
        </fieldset>
      {/if}
      <div class="flex flex-wrap items-center gap-2">
        <Button variant="danger" size="sm" disabled={busy} onclick={remove}>{busy ? 'Deleting…' : 'Delete project'}</Button>
        <Button variant="quiet" size="sm" disabled={busy} onclick={cancel}>Keep the project</Button>
      </div>
    </div>
  {/if}
  {#if notice}<p class="mt-2 text-sm text-accent" role="status">{notice}</p>{/if}
</section>
