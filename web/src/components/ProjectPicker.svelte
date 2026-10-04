<script lang="ts">
  import ChevronsUpDown from '@lucide/svelte/icons/chevrons-up-down'
  import type { Project } from '../lib/api/types.gen'
  import { pickerOptions, selectedLabel, step } from '../lib/picker'
  import { fieldClass } from './ui/field'

  // A project picker you can type into (the WAI-ARIA combobox pattern): click or press an arrow
  // key to open, type to narrow by title, arrows and Enter to pick, Escape to close unchanged.
  // "No project" is always a choice. projects come in already filtered and ordered (see pickable);
  // a chosen project missing from them shows as missingLabel. Label it with <label for={id}>.

  let {
    id,
    projects,
    value,
    onpick,
    missingLabel,
    disabled = false,
    class: className = '',
  }: {
    id: string
    projects: Project[]
    value: number | null
    onpick: (id: number | null) => void
    missingLabel: string
    disabled?: boolean
    class?: string
  } = $props()

  let open = $state(false)
  let query = $state('')
  // The highlighted option's index, or -1 for none.
  let active = $state(-1)

  const options = $derived(pickerOptions(projects, query, value, missingLabel))
  const label = $derived(selectedLabel(projects, value, missingLabel))
  const listId = $derived(`${id}-list`)
  const optionId = (i: number) => `${id}-opt-${i}`

  function show() {
    if (open || disabled) return
    query = ''
    open = true
    active = pickerOptions(projects, '', value, missingLabel).findIndex((o) => o.id === value)
  }

  function close() {
    open = false
    query = ''
    active = -1
  }

  function choose(i: number) {
    const o = options[i]
    close()
    if (o && o.id !== value) onpick(o.id)
  }

  function oninput(e: Event) {
    query = (e.currentTarget as HTMLInputElement).value
    open = true
    // Highlight the first project that matches (after "No project", always first), so typing then Enter picks it.
    active = query.trim() !== '' && options.length > 1 ? 1 : 0
  }

  function onkeydown(e: KeyboardEvent) {
    switch (e.key) {
      case 'ArrowDown':
      case 'ArrowUp':
        e.preventDefault()
        if (!open) show()
        else active = step(active, e.key === 'ArrowDown' ? 1 : -1, options.length)
        break
      case 'Enter':
        // Never submit the form the picker sits in.
        e.preventDefault()
        if (!open) show()
        else if (active >= 0) choose(active)
        break
      case 'Escape':
        if (!open) return
        e.preventDefault()
        e.stopPropagation()
        close()
        break
    }
  }

  $effect(() => {
    if (open && active >= 0) document.getElementById(optionId(active))?.scrollIntoView?.({ block: 'nearest' })
  })
</script>

<div class={['relative', className]}>
  <input
    {id}
    type="text"
    role="combobox"
    autocomplete="off"
    aria-autocomplete="list"
    aria-expanded={open}
    aria-controls={listId}
    aria-activedescendant={open && active >= 0 ? optionId(active) : undefined}
    class={[fieldClass, 'mt-0! py-1.5! pr-8 text-sm']}
    value={open ? query : label}
    placeholder={label}
    {disabled}
    onclick={show}
    {oninput}
    {onkeydown}
    onblur={close}
  />
  <ChevronsUpDown
    class="pointer-events-none absolute top-1/2 right-2 size-4 -translate-y-1/2 text-ink-muted"
    aria-hidden="true"
  />
  {#if open}
    <ul
      id={listId}
      role="listbox"
      class="absolute z-20 mt-1 max-h-64 w-full min-w-48 overflow-y-auto rounded-control border border-line bg-raised py-1 text-sm shadow-lg"
    >
      {#each options as o, i (o.id ?? 'none')}
        <!-- Mouse picks keep focus in the input; keyboard picks go through the input's keydown
             (the combobox pattern), so the option needs no key handler of its own. -->
        <!-- svelte-ignore a11y_click_events_have_key_events -->
        <li
          id={optionId(i)}
          role="option"
          aria-selected={o.id === value}
          class={[
            'cursor-pointer truncate px-3 py-1.5',
            i === active ? 'bg-accent/15 text-ink' : 'text-ink-muted',
            o.id === value && 'font-medium text-ink',
          ]}
          onmousedown={(e) => e.preventDefault()}
          onclick={() => choose(i)}
        >
          {o.label}
        </li>
      {/each}
      {#if options.length === 1 && query.trim() !== ''}
        <li role="presentation" class="px-3 py-1.5 text-ink-faint">No projects match.</li>
      {/if}
    </ul>
  {/if}
</div>
