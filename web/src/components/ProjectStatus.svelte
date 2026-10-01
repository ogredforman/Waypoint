<script lang="ts">
  import { ApiError, patchJSON } from '../lib/api/client'
  import type { ProjectStatus } from '../lib/api/enums.gen'
  import type { Project } from '../lib/api/types.gen'
  import { activateCapMessage, statusOptions } from '../lib/projects'

  let {
    project,
    activeCount,
    cap,
    onchanged,
  }: {
    project: Project
    activeCount: number
    cap: number | null
    onchanged: (p: Project) => void
  } = $props()

  let saving = $state(false)
  let notice = $state('')
  // capHit is set when the server turned down an activation the client thought had room.
  let capHit = $state(false)

  const options = $derived(
    statusOptions(project.status, activeCount, cap).map((o) => ({
      ...o,
      disabled: o.disabled || (o.status === 'active' && capHit && project.status !== 'active'),
    })),
  )
  const activeBlocked = $derived(options.some((o) => o.status === 'active' && o.disabled))

  async function change(status: ProjectStatus) {
    if (status === project.status || saving) return
    notice = ''
    saving = true
    try {
      const p = await patchJSON<Project>(`/api/projects/${project.id}`, { status })
      capHit = false
      onchanged(p)
    } catch (err) {
      if (err instanceof ApiError && err.code === 'active_cap') {
        capHit = true
        notice = "You've reached the active-project limit, so this one stays as it is for now. Pause or finish another project to make room."
      } else {
        notice = err instanceof ApiError ? err.message : "Couldn't change the project's status."
      }
    } finally {
      saving = false
    }
  }
</script>

<div>
  <div role="group" aria-label="Project status" class="inline-flex overflow-hidden rounded-control border border-line">
    {#each options as o (o.status)}
      <button
        type="button"
        aria-pressed={o.status === project.status}
        disabled={o.disabled || saving}
        onclick={() => change(o.status)}
        class={[
          'border-line px-3 py-1.5 text-sm transition not-first:border-l disabled:cursor-default',
          o.status === project.status
            ? 'bg-accent-deep font-medium text-on-accent'
            : o.disabled
              ? 'bg-surface text-ink-faint'
              : 'bg-surface text-ink-muted hover:text-ink',
        ]}
      >
        {o.label}
      </button>
    {/each}
  </div>
  {#if notice}
    <p class="mt-2 text-sm text-accent" role="status">{notice}</p>
  {:else if activeBlocked && cap !== null}
    <p class="mt-2 text-sm text-ink-muted">{activateCapMessage(activeCount, cap)}</p>
  {/if}
</div>
