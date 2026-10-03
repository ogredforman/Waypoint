<script lang="ts">
  import DiaryContent from '../components/DiaryContent.svelte'
  import { current, pageHeading } from '../lib/navigation.svelte'
  import { entryParam } from '../lib/router'

  // /diary?entry=N opens the diary at that entry.
  const focusEntry = $derived(entryParam(current.search))
</script>

<section class="space-y-4">
  <h1 class="text-title font-semibold tracking-tight" {@attach pageHeading}>Diary</h1>
  <DiaryContent {focusEntry} />
</section>
