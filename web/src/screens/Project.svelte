<script lang="ts">
  import { paths } from '../lib/router'

  let { id }: { id: number } = $props()
</script>

<section data-project-id={id}>
  <a href={paths.projects} class="text-sm text-stone-500 hover:text-stone-800">← Projects</a>
  <h1 class="mt-2 text-2xl font-semibold tracking-tight">Project</h1>
  <p class="mt-2 text-stone-600">This project's tasks, blockers, and ideas will show up here.</p>
</section>
