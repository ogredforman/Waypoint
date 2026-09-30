<script lang="ts">
  import type { Snippet } from 'svelte'
  import type { HTMLAttributes } from 'svelte/elements'

  // A flat surface that groups rows, such as a list of projects. Rows inside separate with divide-line.
  let { children, class: className = '', ...rest }: HTMLAttributes<HTMLDivElement> & { children: Snippet } = $props()
</script>

<div {...rest} class={['overflow-hidden rounded-card border border-line bg-surface', className]}>
  {@render children()}
</div>
