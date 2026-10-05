<script lang="ts">
  import { Editor } from '@tiptap/core'
  import { untrack } from 'svelte'
  import { richTextExtensions } from '../../lib/richtext'

  // Shows stored markdown formatted, read-only (ADR 0009). It goes through the editor's schema, so
  // no raw HTML from the text reaches the page. Check lists show their state but can't be ticked.
  let { value, class: className = '' }: { value: string; class?: string } = $props()

  let editor = $state<Editor>()
  // The markdown the editor shows.
  let shown = untrack(() => value)

  $effect(() => {
    const md = value // follow later changes to the text
    untrack(() => {
      if (editor && md !== shown) {
        shown = md
        editor.commands.setContent(md, { contentType: 'markdown', emitUpdate: false })
      }
    })
  })
</script>

<div
  class={['rich-text', className]}
  {@attach (el) => {
    const ed = new Editor({ element: el, editable: false, extensions: richTextExtensions(), content: shown, contentType: 'markdown' })
    editor = ed
    return () => {
      ed.destroy()
      editor = undefined
    }
  }}
></div>
