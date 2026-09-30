<script lang="ts">
  import type { Snippet } from 'svelte'
  import type { HTMLButtonAttributes } from 'svelte/elements'

  // primary: the one main action in view (purple gradient with a glow). secondary: an outlined
  // alternative. quiet: text-only, for cancel and small in-line actions.
  type Variant = 'primary' | 'secondary' | 'quiet'

  let {
    variant = 'primary',
    children,
    class: className = '',
    ...rest
  }: HTMLButtonAttributes & { variant?: Variant; children: Snippet } = $props()

  const variants: Record<Variant, string> = {
    primary:
      'bg-linear-to-b from-accent-strong to-accent-deep text-on-accent shadow-glow hover:brightness-110 font-medium',
    secondary: 'border border-line bg-raised text-ink hover:border-accent/60',
    quiet: 'text-ink-muted hover:text-ink',
  }
</script>

<button
  type="button"
  {...rest}
  class={['rounded-control px-4 py-2 text-sm transition disabled:cursor-default disabled:opacity-60', variants[variant], className]}
>
  {@render children()}
</button>
