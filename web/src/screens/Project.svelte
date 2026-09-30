<script lang="ts">
  import { paths } from '../lib/router'

  let { id }: { id: number } = $props()
</script>

<section data-project-id={id}>
  <a href={paths.projects} class="text-sm text-ink-muted hover:text-ink">← Projects</a>
  <h1 class="mt-2 text-title font-semibold tracking-tight">Project</h1>
  <p class="mt-2 text-ink-muted">This project's tasks, blockers, and ideas will show up here.</p>
</section>
