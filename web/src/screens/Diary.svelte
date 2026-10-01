<script lang="ts">
  import DiaryContent from '../components/DiaryContent.svelte'
  import { pageHeading } from '../lib/navigation.svelte'
</script>

<section class="space-y-4">
  <h1 class="text-title font-semibold tracking-tight" {@attach pageHeading}>Diary</h1>
  <DiaryContent />
</section>
