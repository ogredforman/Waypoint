<script lang="ts">
  import { onMount } from 'svelte'
  import { fetchHealth, type Health } from './lib/health'
  import { current, startRouting } from './lib/navigation.svelte'
  import { pageTitle, paths, sectionOf, type Section } from './lib/router'
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

  let health = $state<Health | 'checking'>('checking')

  const healthLabels: Record<Health | 'checking', string> = {
    checking: 'Checking the server…',
    ok: 'Server is running',
    unavailable: "Server is running, but can't reach its database",
    unreachable: "Can't reach the server",
  }

  $effect(() => {
    document.title = pageTitle(route)
  })

  onMount(() => {
    fetchHealth().then((h) => (health = h))
    return startRouting()
  })
</script>

<div class="flex min-h-screen flex-col text-stone-800">
  <header class="border-b border-stone-200">
    <div class="mx-auto flex max-w-3xl flex-wrap items-center gap-x-8 gap-y-2 px-6 py-4">
      <a href={paths.now} class="text-lg font-semibold tracking-tight text-stone-900">Waypoint</a>
      <nav aria-label="Main">
        <ul class="flex gap-1">
          {#each nav as item (item.section)}
            <li>
              <a
                href={item.href}
                class={[
                  'rounded-md px-3 py-1.5 text-sm',
                  section === item.section ? 'bg-stone-200 text-stone-900' : 'text-stone-600 hover:text-stone-900',
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

  <main class="mx-auto w-full max-w-3xl flex-1 px-6 py-8">
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

  <footer class="mx-auto w-full max-w-3xl px-6 py-4">
    <p class="flex items-center gap-2 text-xs text-stone-500" data-health={health}>
      <span
        class="inline-block size-2 rounded-full"
        class:bg-stone-300={health === 'checking'}
        class:bg-emerald-500={health === 'ok'}
        class:bg-amber-400={health === 'unavailable' || health === 'unreachable'}
        aria-hidden="true"
      ></span>
      {healthLabels[health]}
    </p>
  </footer>
</div>
