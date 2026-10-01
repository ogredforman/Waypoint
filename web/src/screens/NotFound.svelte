<script lang="ts">
  import { pageHeading } from '../lib/navigation.svelte'
  import { paths } from '../lib/router'
</script>

<section>
  <h1 class="text-title font-semibold tracking-tight" {@attach pageHeading}>Nothing here</h1>
  <p class="mt-2 text-ink-muted">
    This page doesn't exist. <a href={paths.now} class="text-accent underline underline-offset-2">Back to Now</a>
  </p>
</section>
