<script lang="ts">
  import type { Snippet } from 'svelte'
  import { pageHeading } from '../lib/navigation.svelte'
  import Button from './ui/Button.svelte'

  // ScreenBoundary keeps a screen that fails while showing from blanking the
  // app: the header and the diary stay, and the screen says so calmly. Going
  // to another screen (a new screen key) tries again by itself.

  let { screen, children }: { screen: string; children: Snippet } = $props()

  let retry: (() => void) | null = null

  $effect(() => {
    void screen
    const r = retry
    retry = null
    r?.()
  })

  function caught(err: unknown, reset: () => void) {
    retry = reset
    console.error('screen failed', err)
  }
</script>

<svelte:boundary onerror={caught}>
  {@render children()}

  {#snippet failed(_err, reset)}
    <section class="space-y-4">
      <h1 class="text-title font-semibold tracking-tight" {@attach pageHeading}>This screen ran into a problem</h1>
      <p class="text-ink-muted">
        Nothing you wrote is lost. Try again, or go to another screen.
        <Button variant="quiet" size="xs" class="ml-1" onclick={reset}>Try again</Button>
      </p>
    </section>
  {/snippet}
</svelte:boundary>
