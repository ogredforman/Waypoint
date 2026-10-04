<script lang="ts">
  import { ApiError, postJSON } from '../lib/api/client'
  import type { Blocker, Task } from '../lib/api/types.gen'
  import { newBlockerRequest } from '../lib/blockers'
  import BlockerRow from './BlockerRow.svelte'
  import Button from './ui/Button.svelte'
  import Input from './ui/Input.svelte'
  import Panel from './ui/Panel.svelte'

  // blockers are the task's open blockers; one resolved, deleted, or moved
  // off the task leaves the list.
  let { task, blockers = $bindable() }: { task: Task; blockers: Blocker[] } = $props()

  let description = $state('')
  let adding = $state(false)
  let addProblem = $state('')

  function changed(b: Blocker) {
    blockers =
      b.status === 'open' && b.task_id === task.id ? blockers.map((x) => (x.id === b.id ? b : x)) : blockers.filter((x) => x.id !== b.id)
  }

  function removed(id: number) {
    blockers = blockers.filter((x) => x.id !== id)
  }

  async function add(e: SubmitEvent) {
    e.preventDefault()
    addProblem = ''
    if (description.trim() === '') {
      addProblem = "Say what's in the way."
      return
    }
    adding = true
    try {
      const b = await postJSON<Blocker>(`/api/projects/${task.project_id}/blockers`, newBlockerRequest(description, task.id))
      blockers = [...blockers, b]
      description = ''
    } catch (err) {
      addProblem = err instanceof ApiError ? err.message : "Couldn't add the blocker."
    } finally {
      adding = false
    }
  }
</script>

<section class="space-y-2" aria-labelledby="task-blockers">
  <h2 id="task-blockers" class="font-medium text-ink">Blockers</h2>
  {#if blockers.length === 0}
    <p class="text-sm text-ink-muted">Nothing in the way.</p>
  {:else}
    <Panel>
      <ul class="divide-y divide-line" aria-label="Open blockers">
        {#each blockers as b (b.id)}
          <BlockerRow blocker={b} showTask={false} onchanged={changed} ondeleted={removed} />
        {/each}
      </ul>
    </Panel>
  {/if}
  <form class="flex flex-wrap items-start gap-2" onsubmit={add} novalidate>
    <div class="min-w-48 flex-1">
      <label for="task-blocker-description" class="sr-only">New blocker on this task</label>
      <Input
        id="task-blocker-description"
        bind:value={description}
        placeholder="Add a blocker"
        autocomplete="off"
        aria-invalid={addProblem ? 'true' : undefined}
        aria-describedby={addProblem ? 'task-blocker-problem' : undefined}
      />
    </div>
    <Button type="submit" class="mt-1" disabled={adding}>{adding ? 'Adding…' : 'Add'}</Button>
  </form>
  {#if addProblem}<p id="task-blocker-problem" class="text-sm text-accent">{addProblem}</p>{/if}
</section>
