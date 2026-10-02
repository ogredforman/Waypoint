<script lang="ts">
  import { profiles, switchProfile } from '../lib/profile.svelte'

  // The header's profile picker: a plain select, so it works from the keyboard
  // like any other. Creating, renaming, and deleting are in Settings.
</script>

{#if profiles.active}
  <div class="flex items-center gap-2 text-sm">
    <label for="profile-switcher" class="text-ink-muted">Profile</label>
    <select
      id="profile-switcher"
      class="max-w-48 rounded-control border border-line bg-canvas px-2 py-1 text-sm text-ink transition focus:border-accent focus:shadow-focus focus:outline-none"
      value={profiles.active.id}
      onchange={(e) => switchProfile(e.currentTarget.value)}
    >
      {#each profiles.list as p (p.id)}
        <option value={p.id}>{p.name}</option>
      {/each}
    </select>
  </div>
{/if}
