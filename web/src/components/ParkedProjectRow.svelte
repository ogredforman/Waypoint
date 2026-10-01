<script lang="ts">
  import { ApiError, patchJSON } from '../lib/api/client'
  import type { Project } from '../lib/api/types.gen'
  import { activateCapMessage, atCap } from '../lib/projects'
  import { paths } from '../lib/router'
  import Button from './ui/Button.svelte'

  let {
    project,
    activeCount,
    cap,
    onactivated,
  }: {
    project: Project
    activeCount: number
    cap: number | null
    onactivated: (p: Project) => void
  } = $props()

  let saving = $state(false)
  let notice = $state('')
  // capHit is set when the server turned down an activation the client thought had room.
  let capHit = $state(false)

  const full = $derived(capHit || atCap(activeCount, cap))

  async function activate() {
    notice = ''
    saving = true
    try {
      onactivated(await patchJSON<Project>(`/api/projects/${project.id}`, { status: 'active' }))
    } catch (err) {
      if (err instanceof ApiError && err.code === 'active_cap') {
        capHit = true
        notice =
          "You've reached the active-project limit, so this one stays parked for now. Pause or finish another project to make room."
      } else {
        notice = err instanceof ApiError ? err.message : "Couldn't activate the project."
      }
      saving = false
    }
  }
</script>

<li class="px-4 py-3">
  <div class="flex items-start gap-3">
    <div class="min-w-0 flex-1">
      <a href={paths.project(project.id)} class="text-ink hover:text-accent">{project.title}</a>
      {#if project.why}<p class="mt-0.5 text-sm whitespace-pre-line text-ink-muted">{project.why}</p>{/if}
    </div>
    <Button variant="secondary" class="shrink-0 px-3 py-1" disabled={full || saving} onclick={activate}>Activate</Button>
  </div>
  {#if notice}
    <p class="mt-2 text-sm text-accent" role="status">{notice}</p>
  {:else if full && cap !== null}
    <p class="mt-2 text-sm text-ink-muted">{activateCapMessage(activeCount, cap)}</p>
  {/if}
</li>
