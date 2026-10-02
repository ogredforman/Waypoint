<script lang="ts">
  import { onDestroy } from 'svelte'
  import { createProfile, nameProblem, profiles } from '../lib/profile.svelte'
  import ProfileRow from './ProfileRow.svelte'
  import Button from './ui/Button.svelte'
  import Input from './ui/Input.svelte'
  import Panel from './ui/Panel.svelte'

  // The Profiles section of Settings: every profile, and a form to add one. A new profile
  // doesn't become the one in use; its row offers to switch to it.
  let name = $state('')
  let problem = $state('')
  let busy = $state(false)

  async function add(e: SubmitEvent) {
    e.preventDefault()
    if (busy) return
    profiles.notice = ''
    if (name.trim() === '') {
      problem = 'Give the profile a name.'
      return
    }
    busy = true
    try {
      const p = await createProfile(name.trim())
      name = ''
      problem = ''
      profiles.notice = `Added ${p.name}.`
    } catch (err) {
      problem = nameProblem(err, "Couldn't add the profile.")
    } finally {
      busy = false
    }
  }

  // The notice is about something just done here; it doesn't follow you back later. Deleting
  // the profile in use remounts this section before the deletion finishes, and the notice
  // arrives after that, so the new copy shows it.
  onDestroy(() => {
    profiles.notice = ''
  })
</script>

<section aria-labelledby="profiles-heading" class="space-y-3">
  <div>
    <h2 id="profiles-heading" class="text-lg font-semibold">Profiles</h2>
    <p class="text-sm text-ink-muted">
      Each profile keeps its own projects, diary, and settings, apart from the others.
    </p>
  </div>
  <Panel>
    <ul class="divide-y divide-line" aria-label="Profiles">
      {#each profiles.list as p (p.id)}
        <ProfileRow profile={p} active={p.id === profiles.active?.id} only={profiles.list.length === 1} />
      {/each}
    </ul>
  </Panel>
  {#if profiles.list.length === 1}
    <p class="text-sm text-ink-muted">This is your only profile, so it can't be deleted. Add another first if you want to.</p>
  {/if}
  <p class="text-sm text-ink-muted" aria-live="polite">{profiles.notice}</p>
  <form class="flex flex-wrap items-end gap-2" onsubmit={add} novalidate>
    <div class="min-w-48 flex-1">
      <label for="new-profile-name" class="text-sm font-medium text-ink">New profile</label>
      <Input
        id="new-profile-name"
        bind:value={name}
        maxlength={60}
        placeholder="e.g. Portfolio"
        oninput={() => (problem = '')}
        aria-invalid={problem ? 'true' : undefined}
        aria-describedby={problem ? 'new-profile-problem' : undefined}
      />
    </div>
    <Button type="submit" variant="secondary" disabled={busy}>{busy ? 'Adding…' : 'Add profile'}</Button>
  </form>
  {#if problem}<p id="new-profile-problem" class="text-sm text-ink">{problem}</p>{/if}
</section>
