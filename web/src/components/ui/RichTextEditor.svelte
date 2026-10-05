<script lang="ts">
  import { Editor } from '@tiptap/core'
  import { untrack } from 'svelte'
  import Bold from '@lucide/svelte/icons/bold'
  import Heading2 from '@lucide/svelte/icons/heading-2'
  import Heading3 from '@lucide/svelte/icons/heading-3'
  import Italic from '@lucide/svelte/icons/italic'
  import Link from '@lucide/svelte/icons/link'
  import List from '@lucide/svelte/icons/list'
  import ListChecks from '@lucide/svelte/icons/list-checks'
  import ListOrdered from '@lucide/svelte/icons/list-ordered'
  import { markdownOf, richTextExtensions } from '../../lib/richtext'
  import Button from './Button.svelte'
  import Input from './Input.svelte'

  // A word-processor field whose value is markdown (ADR 0009). Name it with the id of a visible
  // label element passed as labelledby: a <label for> can't name a contenteditable. Grows with its
  // content up to max-h-64, then scrolls, like Textarea.
  let {
    value = $bindable(''),
    id,
    labelledby,
    describedby,
    invalid = false,
  }: { value?: string; id: string; labelledby: string; describedby?: string; invalid?: boolean } = $props()

  let editor = $state<Editor>()
  // Bumped on every editor transaction, so the toolbar's pressed states follow the cursor.
  let changes = $state(0)
  // The markdown the editor holds as far as value is concerned; anything else came from outside.
  let written = untrack(() => value)
  let linking = $state(false)
  let href = $state('')

  type Tool = { label: string; icon: typeof Bold; name: string; attrs?: Record<string, unknown>; run: (e: Editor) => void }
  const tools: Tool[] = [
    { label: 'Heading', icon: Heading2, name: 'heading', attrs: { level: 2 }, run: (e) => e.chain().focus().toggleHeading({ level: 2 }).run() },
    { label: 'Subheading', icon: Heading3, name: 'heading', attrs: { level: 3 }, run: (e) => e.chain().focus().toggleHeading({ level: 3 }).run() },
    { label: 'Bold', icon: Bold, name: 'bold', run: (e) => e.chain().focus().toggleBold().run() },
    { label: 'Italic', icon: Italic, name: 'italic', run: (e) => e.chain().focus().toggleItalic().run() },
    { label: 'Bulleted list', icon: List, name: 'bulletList', run: (e) => e.chain().focus().toggleBulletList().run() },
    { label: 'Numbered list', icon: ListOrdered, name: 'orderedList', run: (e) => e.chain().focus().toggleOrderedList().run() },
    { label: 'Check list', icon: ListChecks, name: 'taskList', run: (e) => e.chain().focus().toggleTaskList().run() },
  ]

  function active(name: string, attrs?: Record<string, unknown>): boolean {
    void changes
    return editor?.isActive(name, attrs) ?? false
  }

  // mountEditor builds the editor once; reading props here mustn't rebuild it, hence untrack.
  function mountEditor(el: HTMLElement) {
    return untrack(() => {
      const ed = new Editor({
        element: el,
        extensions: richTextExtensions(),
        content: written,
        contentType: 'markdown',
        editorProps: {
          attributes: {
            id,
            role: 'textbox',
            'aria-multiline': 'true',
            'aria-labelledby': labelledby,
            class: 'rich-text max-h-64 min-h-16 overflow-y-auto px-3 py-2 focus:outline-none',
          },
        },
        // Only edits write value, so opening untouched text and saving it changes nothing.
        onUpdate: ({ editor }) => {
          written = markdownOf(editor)
          value = written
        },
        onTransaction: () => changes++,
      })
      editor = ed
      return () => {
        ed.destroy()
        editor = undefined
      }
    })
  }

  // A value set from outside (not typed here) replaces the text.
  $effect(() => {
    const md = value
    untrack(() => {
      if (!editor || md === written) return
      written = md
      editor.commands.setContent(md, { contentType: 'markdown', emitUpdate: false })
    })
  })

  // Set here rather than in editorProps, so a server problem arriving doesn't rebuild the editor.
  $effect(() => {
    const dom = editor?.view.dom
    if (!dom) return
    if (describedby) dom.setAttribute('aria-describedby', describedby)
    else dom.removeAttribute('aria-describedby')
    if (invalid) dom.setAttribute('aria-invalid', 'true')
    else dom.removeAttribute('aria-invalid')
  })

  function openLink() {
    if (!editor) return
    href = editor.getAttributes('link').href ?? ''
    linking = true
  }

  // applyLink links the selection (or the whole link the cursor is in); an empty address removes it.
  function applyLink() {
    if (!editor) return
    const url = href.trim()
    const chain = editor.chain().focus().extendMarkRange('link')
    if (url === '') chain.unsetLink().run()
    else chain.setLink({ href: url }).run()
    linking = false
  }

  function cancelLink() {
    linking = false
    editor?.commands.focus()
  }

  function linkKey(e: KeyboardEvent) {
    if (e.key === 'Enter') {
      e.preventDefault()
      e.stopPropagation()
      applyLink()
    } else if (e.key === 'Escape') {
      // Closes only the link box, not a surrounding edit.
      e.preventDefault()
      e.stopPropagation()
      cancelLink()
    }
  }

  // Toolbar buttons keep the focus (and so the selection) in the text.
  const keepFocus = (e: MouseEvent) => e.preventDefault()
</script>

<div
  class={[
    'mt-1 w-full rounded-control border bg-canvas text-ink transition focus-within:border-accent focus-within:shadow-focus',
    invalid ? 'border-accent' : 'border-line',
  ]}
>
  <div class="flex flex-wrap items-center gap-0.5 border-b border-line px-1.5 py-1" role="toolbar" aria-label="Formatting">
    {#each tools as tool (tool.label)}
      {@const Icon = tool.icon}
      {@const on = active(tool.name, tool.attrs)}
      <Button
        variant="quiet"
        size="xs"
        aria-label={tool.label}
        title={tool.label}
        aria-pressed={on}
        class={on ? 'text-accent' : ''}
        onmousedown={keepFocus}
        onclick={() => editor && tool.run(editor)}><Icon size={16} aria-hidden="true" /></Button
      >
    {/each}
    <Button
      variant="quiet"
      size="xs"
      aria-label="Link"
      title="Link"
      aria-pressed={active('link')}
      aria-expanded={linking}
      class={active('link') ? 'text-accent' : ''}
      onmousedown={keepFocus}
      onclick={openLink}><Link size={16} aria-hidden="true" /></Button
    >
  </div>
  {#if linking}
    <div class="flex flex-wrap items-center gap-2 border-b border-line px-2 pb-2">
      <label for="{id}-href" class="mt-1 text-sm text-ink-muted">Link address</label>
      <div class="min-w-40 flex-1">
        <Input id="{id}-href" bind:value={href} onkeydown={linkKey} autofocus autocomplete="off" placeholder="https://" />
      </div>
      <div class="mt-1 flex gap-1">
        <Button size="xs" onclick={applyLink}>{href.trim() === '' ? 'Remove link' : 'Apply'}</Button>
        <Button variant="quiet" size="xs" onclick={cancelLink}>Cancel</Button>
      </div>
    </div>
  {/if}
  <div {@attach mountEditor}></div>
</div>
