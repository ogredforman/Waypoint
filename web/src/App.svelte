<script lang="ts">
  import { onMount } from 'svelte'
  import DiaryOverlay from './components/DiaryOverlay.svelte'
  import DiaryPanel from './components/DiaryPanel.svelte'
  import ServerBanner from './components/ServerBanner.svelte'
  import { onDisconnect, onReconnect, startEvents } from './lib/events'
  import { watchHealth, type Health } from './lib/health'
  import { current, startRouting } from './lib/navigation.svelte'
  import { pageTitle, paths, sectionOf, type Section } from './lib/router'
  import { followEvents } from './lib/timeline.svelte'
  import Diary from './screens/Diary.svelte'
  import NotFound from './screens/NotFound.svelte'
  import Now from './screens/Now.svelte'
  import ParkingLot from './screens/ParkingLot.svelte'
  import Project from './screens/Project.svelte'
  import Projects from './screens/Projects.svelte'
  import Task from './screens/Task.svelte'

  const nav: { section: Section; label: string; href: string }[] = [
    { section: 'now', label: 'Now', href: paths.now },
    { section: 'diary', label: 'Diary', href: paths.diary },
    { section: 'projects', label: 'Projects', href: paths.projects },
    { section: 'parkingLot', label: 'Parking lot', href: paths.parkingLot },
  ]

  const route = $derived(current.route)
  const section = $derived(sectionOf(route))

  // Assume the server is fine until it says otherwise: the banner only appears when it isn't.
  let health = $state<Health>('ok')

  // Only one diary is mounted at a time: the side panel from Tailwind's lg breakpoint up, the
  // floating button and overlay below it (ADR 0004).
  const wideQuery = window.matchMedia('(min-width: 64rem)')
  let wide = $state(wideQuery.matches)

  $effect(() => {
    document.title = pageTitle(route)
  })

  onMount(() => {
    const healthWatch = watchHealth((h) => (health = h))
    const offDown = onDisconnect(healthWatch.check)
    const offUp = onReconnect(healthWatch.check)
    const onWidth = () => (wide = wideQuery.matches)
    wideQuery.addEventListener('change', onWidth)
    const stopRouting = startRouting()
    // Subscribe before connecting, so no event slips past the timeline.
    const stopTimeline = followEvents()
    const stopEvents = startEvents()
    return () => {
      wideQuery.removeEventListener('change', onWidth)
      stopRouting()
      stopEvents()
      stopTimeline()
      offDown()
      offUp()
      healthWatch.stop()
    }
  })
</script>

<!--
  Layout (ADR 0004): header over a row of the main pane and, on wide windows, the diary panel.
  Each scrolls on its own. /diary shows the diary full-page instead, so it has no panel beside it.
-->
<div class="flex h-dvh flex-col">
  <header class="shrink-0 border-b border-line bg-surface/60 backdrop-blur">
    <div class="flex flex-wrap items-center gap-x-8 gap-y-2 px-gutter py-3">
      <a href={paths.now} class="text-lg font-semibold tracking-tight text-ink">
        Way<span class="text-accent">point</span>
      </a>
      <nav aria-label="Main">
        <ul class="flex gap-1">
          {#each nav as item (item.section)}
            <li>
              <a
                href={item.href}
                class={[
                  'rounded-control px-3 py-1.5 text-sm transition',
                  section === item.section ? 'bg-raised text-ink shadow-glow' : 'text-ink-muted hover:text-ink',
                ]}
                aria-current={section === item.section ? 'page' : undefined}
              >
                {item.label}
              </a>
            </li>
          {/each}
        </ul>
      </nav>
    </div>
  </header>

  <ServerBanner {health} />

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
        {:else}
          <NotFound />
        {/if}
      </main>
    </div>

    {#if wide && route.name !== 'diary'}
      <DiaryPanel />
    {/if}
  </div>
</div>

{#if !wide && route.name !== 'diary'}
  <DiaryOverlay />
{/if}
