<script lang="ts">
  import type { Snippet } from 'svelte'
  import type { HTMLAttributes } from 'svelte/elements'

  // A raised block of content. glow marks the one thing that deserves attention on a screen,
  // like the next-action card; use it sparingly.
  let {
    glow = false,
    children,
    class: className = '',
    ...rest
  }: HTMLAttributes<HTMLDivElement> & { glow?: boolean; children: Snippet } = $props()
</script>

<div
  {...rest}
  class={[
    'rounded-card border bg-raised p-4',
    glow ? 'border-accent/40 bg-linear-to-br from-raised to-accent-deep/25 shadow-glow' : 'border-line',
    className,
  ]}
>
  {@render children()}
</div>
