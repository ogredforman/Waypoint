<script lang="ts">
  let { id }: { id: number } = $props()
</script>

<section data-task-id={id}>
  <h1 class="text-title font-semibold tracking-tight">Task</h1>
  <p class="mt-2 text-ink-muted">Where this task came from, and everything you've written about it, will show up here.</p>
</section>
