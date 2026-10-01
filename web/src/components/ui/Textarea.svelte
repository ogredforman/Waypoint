<script lang="ts">
  import type { HTMLTextareaAttributes } from 'svelte/elements'
  import { fitHeight, refitOnResize } from '../../lib/autosize'
  import { fieldClass } from './field'

  // Grows with its content up to a maximum height (16rem, or 24rem when tall), then scrolls.
  let {
    value = $bindable(''),
    tall = false,
    class: className = '',
    ...rest
  }: HTMLTextareaAttributes & { tall?: boolean } = $props()
</script>

<textarea
  bind:value
  {...rest}
  class={[fieldClass, 'resize-none', tall ? 'max-h-96' : 'max-h-64', className]}
  {@attach (el) => {
    void value // refit whenever the value changes, typed or set from code
    fitHeight(el)
  }}
  {@attach refitOnResize}
></textarea>
