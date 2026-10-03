<script lang="ts">
  import { onMount } from 'svelte'
  import { loadAppearance } from '../lib/appearance'
  import { startEvents } from '../lib/events'
  import type { Route } from '../lib/router'
  import { followEvents } from '../lib/timeline.svelte'
  import Diary from '../screens/Diary.svelte'
  import NotFound from '../screens/NotFound.svelte'
  import Now from '../screens/Now.svelte'
  import ParkingLot from '../screens/ParkingLot.svelte'
  import Project from '../screens/Project.svelte'
  import Projects from '../screens/Projects.svelte'
  import Settings from '../screens/Settings.svelte'
  import Task from '../screens/Task.svelte'
  import DiaryOverlay from './DiaryOverlay.svelte'
  import DiaryPanel from './DiaryPanel.svelte'

  // Everything that shows one profile's data: the screens, the diary, and the
  // event stream. App.svelte mounts it once per active profile (ADR 0005).
  let { route, wide }: { route: Route; wide: boolean } = $props()

  onMount(() => {
    // This profile's fonts; the previous profile's stay until they arrive.
    void loadAppearance()
    // Subscribe before connecting, so no event slips past the timeline.
    const stopTimeline = followEvents()
    const stopEvents = startEvents()
    return () => {
      stopEvents()
      stopTimeline()
    }
  })
</script>

<div class="flex min-h-0 flex-1">
  <div class="flex min-w-0 flex-1 flex-col overflow-y-auto">
    <main class="mx-auto w-full max-w-3xl flex-1 px-gutter py-8">
      {#if route.name === 'now'}
        <Now />
      {:else if route.name === 'diary'}
        <Diary />
      {:else if route.name === 'projects'}
        <Projects />
      {:else if route.name === 'project'}
        {#key route.id}<Project id={route.id} />{/key}
      {:else if route.name === 'task'}
        {#key route.id}<Task id={route.id} />{/key}
      {:else if route.name === 'parkingLot'}
        <ParkingLot />
      {:else if route.name === 'settings'}
        <Settings />
      {:else}
        <NotFound />
      {/if}
    </main>
  </div>

  {#if wide && route.name !== 'diary'}
    <DiaryPanel />
  {/if}
</div>

{#if !wide && route.name !== 'diary'}
  <DiaryOverlay />
{/if}
