// @vitest-environment jsdom
import type { Editor } from '@tiptap/core'
import { flushSync, mount, unmount } from 'svelte'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import RichText from './RichText.svelte'
import RichTextEditor from './RichTextEditor.svelte'

let target: HTMLElement
let component: ReturnType<typeof mount> | undefined

beforeEach(() => {
  target = document.createElement('div')
  document.body.append(target)
})

afterEach(() => {
  if (component) unmount(component)
  component = undefined
  target.remove()
})

function editorOf(): Editor {
  return target.querySelector<HTMLElement & { editor: Editor }>('#notes')!.editor
}
function tool(label: string): HTMLButtonElement {
  return target.querySelector<HTMLButtonElement>(`button[aria-label="${label}"]`)!
}
function click(el: HTMLElement) {
  el.dispatchEvent(new MouseEvent('click', { bubbles: true }))
  flushSync()
}

function mountEditor(value: string) {
  const props = $state({ value })
  component = mount(RichTextEditor, { target, props: { id: 'notes', labelledby: 'notes-label', get value() { return props.value }, set value(v) { props.value = v } } })
  flushSync()
  return props
}

describe('RichTextEditor', () => {
  it('is a labelled text box', () => {
    mountEditor('')
    const box = target.querySelector('#notes')!
    expect(box.getAttribute('role')).toBe('textbox')
    expect(box.getAttribute('aria-labelledby')).toBe('notes-label')
  })

  it('leaves the value alone until the text is edited', () => {
    const props = mountEditor('first line\nsecond line')
    expect(props.value).toBe('first line\nsecond line')
    expect(target.querySelector('#notes br')).not.toBeNull()
  })

  it('formats the selection from the toolbar and writes markdown', () => {
    const props = mountEditor('soil')
    editorOf().commands.selectAll()
    flushSync()
    click(tool('Bold'))
    expect(props.value).toBe('**soil**')
    expect(tool('Bold').getAttribute('aria-pressed')).toBe('true')

    click(tool('Check list'))
    expect(props.value).toBe('- [ ] **soil**')
  })

  it('links the selection from the link address box', () => {
    const props = mountEditor('the guide')
    editorOf().commands.selectAll()
    click(tool('Link'))
    const address = target.querySelector<HTMLInputElement>('#notes-href')!
    address.value = 'https://example.com'
    address.dispatchEvent(new Event('input', { bubbles: true }))
    flushSync()
    address.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
    flushSync()
    expect(props.value).toBe('[the guide](https://example.com)')
    expect(target.querySelector('#notes-href')).toBeNull()
  })

  it('shows a value set from outside', () => {
    const props = mountEditor('old')
    props.value = '## New'
    flushSync()
    expect(target.querySelector('#notes h2')?.textContent).toBe('New')
  })
})

describe('RichText', () => {
  it('shows markdown formatted and not editable', () => {
    component = mount(RichText, { target, props: { value: '## Plan\n\n- [x] dig\n\nSee [docs](https://example.com)' } })
    flushSync()
    const view = target.querySelector('.rich-text')!
    expect(view.querySelector('h2')?.textContent).toBe('Plan')
    expect(view.querySelector<HTMLInputElement>('input[type=checkbox]')?.checked).toBe(true)
    expect(view.querySelector('a')?.getAttribute('href')).toBe('https://example.com')
    expect(view.querySelector('[contenteditable=true]')).toBeNull()
  })

  it('never renders raw HTML from the text', () => {
    component = mount(RichText, { target, props: { value: 'hi <img src=x onerror="alert(1)"> <script>alert(1)</script>' } })
    flushSync()
    expect(target.querySelector('img, script')).toBeNull()
  })
})
