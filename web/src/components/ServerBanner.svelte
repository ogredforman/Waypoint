<script lang="ts">
  import type { Health } from '../lib/health'

  // Shows the server's state only when something is wrong; a healthy server shows nothing.
  // Calm by design (SPEC §5): a caution dot, never red, and it clears on its own.
  let { health }: { health: Health } = $props()

  const messages: Record<Exclude<Health, 'ok'>, string> = {
    unreachable: "Can't reach the Waypoint server. Trying again…",
    unavailable: "The server is running but can't reach its database. Trying again…",
  }
</script>

<!-- The live region stays mounted so the message is announced when it appears. -->
<div role="status" aria-live="polite">
  {#if health !== 'ok'}
    <p class="flex items-center gap-2 border-b border-line bg-raised px-gutter py-2 text-sm text-ink" data-health={health}>
      <span class="inline-block size-2 shrink-0 rounded-full bg-caution" aria-hidden="true"></span>
      {messages[health]}
    </p>
  {/if}
</div>
