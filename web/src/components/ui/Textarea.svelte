<script lang="ts">
  import type { HTMLTextareaAttributes } from 'svelte/elements'
  import { fieldClass } from './field'

  let { value = $bindable(''), class: className = '', ...rest }: HTMLTextareaAttributes = $props()
</script>

<textarea bind:value {...rest} class={[fieldClass, className]}></textarea>
