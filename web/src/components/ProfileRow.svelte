<script lang="ts">
  import type { Profile } from '../lib/api/types.gen'
  import { deleteConfirmed, nameProblem, profiles, removeProfile, renameProfile, successor, switchProfile } from '../lib/profile.svelte'
  import Button from './ui/Button.svelte'
  import Input from './ui/Input.svelte'

  // One profile in Settings: switch to it, rename it, or delete it after typing its name.
  // The only profile has no delete button; ProfileSettings says why.
  let { profile, active, only }: { profile: Profile; active: boolean; only: boolean } = $props()

  let mode = $state<'view' | 'rename' | 'delete'>('view')
  let name = $state('')
  let typed = $state('')
  let problem = $state('')
  let busy = $state(false)

  const confirmed = $derived(deleteConfirmed(typed, profile.name))
  const next = $derived(active ? successor(profiles.list, profile.id) : null)
  const base = $derived(`profile-${profile.id}`)

  function startRename() {
    name = profile.name
    problem = ''
    profiles.notice = ''
    mode = 'rename'
  }

  function startDelete() {
    typed = ''
    problem = ''
    profiles.notice = ''
    mode = 'delete'
  }

  function cancel() {
    problem = ''
    mode = 'view'
  }

  async function rename(e: SubmitEvent) {
    e.preventDefault()
    if (busy) return
    if (name.trim() === profile.name) return cancel()
    busy = true
    try {
      await renameProfile(profile.id, name.trim())
      mode = 'view'
      problem = ''
    } catch (err) {
      problem = nameProblem(err, "Couldn't rename this profile.")
    } finally {
      busy = false
    }
  }

  async function remove(e: SubmitEvent) {
    e.preventDefault()
    if (busy || !confirmed) return
    busy = true
    const gone = profile.name
    const movingTo = next?.name
    try {
      await removeProfile(profile.id)
      profiles.notice = `Deleted ${gone}.`
    } catch (err) {
      const problem = nameProblem(err, `Couldn't delete ${gone}. It's still here.`)
      const moved = movingTo && profiles.active?.name === movingTo ? ` You're now in ${movingTo}.` : ''
      profiles.notice = problem + moved
    } finally {
      busy = false
    }
  }
</script>

<li class="space-y-3 p-4" aria-label={profile.name}>
  {#if mode === 'rename'}
    <form class="space-y-2" onsubmit={rename} novalidate>
      <label for="{base}-name" class="text-sm font-medium text-ink">Name</label>
      <Input
        id="{base}-name"
        bind:value={name}
        maxlength={60}
        aria-invalid={problem ? 'true' : undefined}
        aria-describedby={problem ? `${base}-problem` : undefined}
      />
      {#if problem}<p id="{base}-problem" class="text-sm text-ink">{problem}</p>{/if}
      <div class="flex gap-2">
        <Button type="submit" size="sm" disabled={busy || name.trim() === ''}>{busy ? 'Saving…' : 'Save'}</Button>
        <Button variant="quiet" size="sm" onclick={cancel}>Cancel</Button>
      </div>
    </form>
  {:else}
    <div class="flex flex-wrap items-center gap-2">
      <span class="font-medium text-ink">{profile.name}</span>
      {#if active}<span class="rounded-control bg-raised px-2 py-0.5 text-xs text-ink-muted">In use</span>{/if}
      <div class="ml-auto flex gap-2">
        {#if !active}
          <Button variant="secondary" size="sm" onclick={() => switchProfile(profile.id)}>Switch to</Button>
        {/if}
        <Button variant="secondary" size="sm" onclick={startRename} aria-label="Rename {profile.name}">Rename</Button>
        {#if !only && mode === 'view'}
          <Button variant="quiet" size="sm" onclick={startDelete} aria-label="Delete {profile.name}…">Delete…</Button>
        {/if}
      </div>
    </div>
    {#if mode === 'delete'}
      <form class="space-y-2 rounded-control border border-line bg-canvas/60 p-3" onsubmit={remove} novalidate>
        <p id="{base}-warning" class="text-sm text-ink">
          Deleting <strong>{profile.name}</strong> removes everything in it: its projects, tasks, diary entries, ideas,
          and settings. This can't be undone. Type <strong>{profile.name}</strong> to confirm.
          {#if next}Waypoint will switch to {next.name} first.{/if}
        </p>
        <label for="{base}-confirm" class="sr-only">Type {profile.name} to confirm</label>
        <Input id="{base}-confirm" bind:value={typed} autocomplete="off" aria-describedby="{base}-warning" />
        <div class="flex gap-2">
          <Button type="submit" variant="danger" size="sm" disabled={!confirmed || busy}>
            {busy ? 'Deleting…' : `Delete ${profile.name}`}
          </Button>
          <Button variant="quiet" size="sm" onclick={cancel}>Cancel</Button>
        </div>
      </form>
    {/if}
  {/if}
</li>
