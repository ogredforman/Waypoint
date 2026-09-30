<script lang="ts">
  import type { HTMLInputAttributes } from 'svelte/elements'
  import { fieldClass } from './field'

  let { value = $bindable(''), class: className = '', ...rest }: HTMLInputAttributes = $props()
</script>

<input bind:value {...rest} class={[fieldClass, className]} />
