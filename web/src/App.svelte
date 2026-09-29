<script lang="ts">
  import { onMount } from 'svelte'
  import { fetchHealth, type Health } from './lib/health'

  let health = $state<Health | 'checking'>('checking')

  const labels: Record<Health | 'checking', string> = {
    checking: 'Checking the server…',
    ok: 'Server is running',
    unavailable: "Server is running, but can't reach its database",
    unreachable: "Can't reach the server",
  }

  onMount(async () => {
    health = await fetchHealth()
  })
</script>

<main class="mx-auto flex min-h-screen max-w-xl flex-col justify-center gap-4 px-6 text-stone-800">
  <h1 class="text-4xl font-semibold tracking-tight">Waypoint</h1>
  <p class="flex items-center gap-2 text-stone-600" data-health={health}>
    <span
      class="inline-block size-2 rounded-full"
      class:bg-stone-300={health === 'checking'}
      class:bg-emerald-500={health === 'ok'}
      class:bg-amber-400={health === 'unavailable' || health === 'unreachable'}
      aria-hidden="true"
    ></span>
    {labels[health]}
  </p>
</main>
