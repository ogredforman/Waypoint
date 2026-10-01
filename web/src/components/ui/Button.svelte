<script lang="ts">
  import type { Snippet } from 'svelte'
  import type { HTMLButtonAttributes } from 'svelte/elements'

  // primary: the one main action in view (purple gradient with a glow). secondary: an outlined
  // alternative, e.g. Edit. quiet: a subtle background, for cancel and small in-line actions.
  // danger: red, for delete and remove only. Red never marks state (SPEC §11).
  type Variant = 'primary' | 'secondary' | 'quiet' | 'danger'
  type Size = 'md' | 'sm' | 'xs'

  let {
    variant = 'primary',
    size = 'md',
    children,
    class: className = '',
    ...rest
  }: HTMLButtonAttributes & { variant?: Variant; size?: Size; children: Snippet } = $props()

  const variants: Record<Variant, string> = {
    primary:
      'bg-linear-to-b from-accent-strong to-accent-deep text-on-accent shadow-glow hover:brightness-110 font-medium',
    secondary: 'border border-line bg-raised text-ink hover:border-accent/60',
    quiet: 'bg-raised/50 text-ink-muted hover:bg-raised hover:text-ink',
    danger: 'bg-danger text-on-accent hover:brightness-110 font-medium',
  }

  const sizes: Record<Size, string> = {
    md: 'px-4 py-2 text-sm',
    sm: 'px-3 py-1 text-sm',
    xs: 'px-2 py-0.5 text-xs',
  }
</script>

<button
  type="button"
  {...rest}
  class={['rounded-control transition disabled:cursor-default disabled:opacity-60', variants[variant], sizes[size], className]}
>
  {@render children()}
</button>
