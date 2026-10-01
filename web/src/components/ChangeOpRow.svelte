<script lang="ts">
  import type { ChangeOp } from '../lib/api/types.gen'
  import { editedPayload, editFields, type Choice, type Payload } from '../lib/review'
  import Button from './ui/Button.svelte'
  import Input from './ui/Input.svelte'
  import Textarea from './ui/Textarea.svelte'

  // One proposed change in a changeset review: what it will do, why it was proposed, and the
  // user's call on it. Undecided ops offer accept, reject, and (for ops with text) edit; decided ones
  // say what was chosen and can be taken back. blockedBy, when set, says why the op can't be accepted.

  let {
    op,
    sentence,
    choice,
    payload,
    blockedBy,
    disabled,
    onaccept,
    onreject,
    onundo,
    onedit,
  }: {
    op: ChangeOp
    sentence: string
    choice: Choice | undefined
    payload: Payload
    blockedBy: string | null
    disabled: boolean
    onaccept: () => void
    onreject: () => void
    onundo: () => void
    onedit: (payload: Payload) => void
  } = $props()

  const fields = $derived(editFields(op))
  let editing = $state(false)
  let values = $state<Record<string, string>>({})
  const canSave = $derived(fields.every((f) => !f.required || (values[f.key] ?? '').trim() !== ''))

  const decided: Record<Choice['action'], string> = { accept: 'Accepted', reject: 'Rejected', edit: 'Accepted with edits' }

  function startEdit() {
    values = Object.fromEntries(fields.map((f) => [f.key, typeof payload[f.key] === 'string' ? (payload[f.key] as string) : '']))
    editing = true
  }

  function save() {
    if (!canSave) return
    onedit(editedPayload(op, payload, values))
    editing = false
  }
</script>

<li class="space-y-2 py-3 first:pt-0 last:pb-0">
  <p class={['text-sm', choice?.action === 'reject' ? 'text-ink-faint line-through' : 'text-ink']}>{sentence}</p>
  {#if op.rationale}
    <p class="text-xs text-ink-muted">{op.rationale}</p>
  {/if}

  {#if editing}
    <form
      class="space-y-2"
      onsubmit={(e) => {
        e.preventDefault()
        save()
      }}
    >
      {#each fields as f (f.key)}
        <div class="space-y-1">
          <label for="op-{op.id}-{f.key}" class="text-xs text-ink-muted">{f.label}</label>
          {#if f.long}
            <Textarea id="op-{op.id}-{f.key}" rows={2} class="text-sm" bind:value={values[f.key]} />
          {:else}
            <Input id="op-{op.id}-{f.key}" class="text-sm" bind:value={values[f.key]} />
          {/if}
        </div>
      {/each}
      <div class="flex flex-wrap gap-2">
        <Button type="submit" variant="secondary" disabled={!canSave}>Save and accept</Button>
        <Button variant="quiet" onclick={() => (editing = false)}>Cancel</Button>
      </div>
    </form>
  {:else if choice}
    <div class="flex flex-wrap items-center gap-2 text-xs text-ink-muted">
      <span>{decided[choice.action]}</span>
      <Button variant="quiet" class="px-2 py-1 text-xs" onclick={onundo} {disabled}>Undo</Button>
    </div>
  {:else}
    <div class="flex flex-wrap gap-2">
      <Button variant="secondary" class="px-3 py-1" onclick={onaccept} disabled={disabled || blockedBy !== null}>Accept</Button>
      <Button variant="quiet" class="px-3 py-1" onclick={onreject} {disabled}>Reject</Button>
      {#if fields.length > 0}
        <Button variant="quiet" class="px-3 py-1" onclick={startEdit} {disabled}>Edit</Button>
      {/if}
    </div>
    {#if blockedBy !== null}
      <p class="text-xs text-ink-muted">{blockedBy}</p>
    {/if}
  {/if}
</li>
