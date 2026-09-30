<script lang="ts">
  let { id }: { id: number } = $props()
</script>

<section data-task-id={id}>
  <h1 class="text-2xl font-semibold tracking-tight">Task</h1>
  <p class="mt-2 text-stone-600">Where this task came from, and everything you've written about it, will show up here.</p>
</section>
