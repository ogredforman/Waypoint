// Rich text for task notes, a project's why and definition of done, and an idea's body (ADR 0009).
// The text is stored as markdown; Tiptap edits and shows it. The diary stays plain text.
import { Editor, type Extensions } from '@tiptap/core'
import { TaskItem, TaskList } from '@tiptap/extension-list'
import { Markdown } from '@tiptap/markdown'
import StarterKit from '@tiptap/starter-kit'

// richTextExtensions is what the editor and the read-only view both understand. Underline is off
// because markdown can't store it; headings are levels 2 and 3 because these are notes, not documents.
export function richTextExtensions(): Extensions {
  return [
    StarterKit.configure({
      underline: false,
      heading: { levels: [2, 3] },
      link: { openOnClick: false, autolink: true, defaultProtocol: 'https' },
    }),
    TaskList,
    // A check list item's box is named by its text; "task item" would clash with Waypoint's tasks.
    TaskItem.configure({ nested: true, a11y: { checkboxLabel: (node) => node.textContent || 'Empty item' } }),
    // breaks: a single line break is kept, so text written before the editor existed reads as it did.
    Markdown.configure({ markedOptions: { breaks: true } }),
  ]
}

// plainText is markdown's text without its markup, blocks joined by spaces, for one-line previews
// such as a project's why in the Projects list. Results are kept, since lists ask again on each render.
const plainCache = new Map<string, string>()
export function plainText(markdown: string): string {
  let text = plainCache.get(markdown)
  if (text === undefined) {
    const editor = new Editor({ extensions: richTextExtensions(), content: markdown, contentType: 'markdown' })
    text = editor.getText({ blockSeparator: ' ' }).replace(/\s+/g, ' ').trim()
    editor.destroy()
    plainCache.set(markdown, text)
  }
  return text
}

// markdownOf is the editor's text as markdown, '' when it's empty.
export function markdownOf(editor: Editor): string {
  if (editor.isEmpty) return ''
  return editor.getMarkdown().trim()
}
