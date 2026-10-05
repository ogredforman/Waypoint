// @vitest-environment jsdom
import { Editor } from '@tiptap/core'
import { describe, expect, it } from 'vitest'
import { markdownOf, plainText, richTextExtensions } from './richtext'

// roundTrip reads markdown into a headless editor and writes it back out, as saving would.
function roundTrip(markdown: string): string {
  const editor = new Editor({ extensions: richTextExtensions(), content: markdown, contentType: 'markdown' })
  try {
    return markdownOf(editor)
  } finally {
    editor.destroy()
  }
}

describe('rich text markdown', () => {
  it.each([
    ['headings', '## Plan\n\n### Later'],
    ['bold and italic', 'Some **bold** and *italic* text'],
    ['bulleted list', '- one\n- two'],
    ['numbered list', '1. one\n2. two'],
    ['check list', '- [ ] buy soil\n- [x] pick a spot'],
    ['link', 'See [the guide](https://example.com/guide)'],
  ])('keeps %s', (_, md) => {
    expect(roundTrip(md)).toBe(md)
  })

  it('is empty for empty text', () => {
    expect(roundTrip('')).toBe('')
  })

  it('keeps line breaks in plain text written before the editor', () => {
    expect(roundTrip('first line\nsecond line')).toBe('first line  \nsecond line')
  })

  it('gives the plain text of markdown for previews', () => {
    expect(plainText('## Why\n\nGrow **fresh** food:\n\n- [ ] beans\n\nSee [the plan](https://example.com)')).toBe(
      'Why Grow fresh food: beans See the plan',
    )
    expect(plainText('')).toBe('')
  })

  it('drops raw HTML rather than rendering it', () => {
    expect(roundTrip('<img src=x onerror="alert(1)">hi')).not.toContain('onerror')
  })
})
