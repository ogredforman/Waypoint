<script lang="ts">
  import SettingsIcon from '@lucide/svelte/icons/settings'
  import { onMount } from 'svelte'
  import ProfileSwitcher from './components/ProfileSwitcher.svelte'
  import ProfileWorkspace from './components/ProfileWorkspace.svelte'
  import ServerBanner from './components/ServerBanner.svelte'
  import { onDisconnect, onReconnect } from './lib/events'
  import { watchHealth, type Health } from './lib/health'
  import { current, startRouting } from './lib/navigation.svelte'
  import { profiles } from './lib/profile.svelte'
  import { pageTitle, paths, sectionOf, type Section } from './lib/router'

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
    return () => {
      wideQuery.removeEventListener('change', onWidth)
      stopRouting()
      offDown()
      offUp()
      healthWatch.stop()
    }
  })
</script>

<!--
  Layout (ADR 0004): header over a row of the main pane and, on wide windows, the diary panel.
  Each scrolls on its own. /diary shows the diary full-page instead, so it has no panel beside it.
  Everything showing a profile's data is in ProfileWorkspace, keyed by the active profile: switching
  profiles remounts it, so the event stream reopens and every screen loads again (ADR 0005).
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
      <div class="ml-auto flex items-center gap-3">
        <ProfileSwitcher />
        <a
          href={paths.settings}
          class={[
            'flex items-center gap-1.5 rounded-control px-3 py-1.5 text-sm transition',
            section === 'settings' ? 'bg-raised text-ink shadow-glow' : 'text-ink-muted hover:text-ink',
          ]}
          aria-current={section === 'settings' ? 'page' : undefined}
        >
          <SettingsIcon size={16} aria-hidden="true" />
          Settings
        </a>
      </div>
    </div>
  </header>

  <ServerBanner {health} />

  {#key profiles.active?.id}
    <ProfileWorkspace {route} {wide} />
  {/key}
</div>
