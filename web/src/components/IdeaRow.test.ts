// @vitest-environment jsdom
import { flushSync, mount, unmount } from 'svelte'
import type { Editor } from '@tiptap/core'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Idea } from '../lib/api/types.gen'
import IdeaRow from './IdeaRow.svelte'

const herbs: Idea = {
  id: 4,
  project_id: null,
  title: 'Herb spiral',
  body: 'Near the kitchen',
  status: 'parked',
  origin_entry_id: null,
  created_at: '',
  updated_at: '',
}

let target: HTMLElement
let component: ReturnType<typeof mount> | undefined
let reply: () => Response
const fetchMock = vi.fn(async (_url: string, _init?: RequestInit) => reply())

beforeEach(() => {
  fetchMock.mockClear()
  vi.stubGlobal('fetch', fetchMock)
  target = document.createElement('ul')
  document.body.append(target)
})

afterEach(() => {
  if (component) unmount(component)
  component = undefined
  target.remove()
  vi.unstubAllGlobals()
})

function show(onchanged = vi.fn()) {
  component = mount(IdeaRow, {
    target,
    props: { idea: herbs, projects: [], onchanged, onpromoted: vi.fn(), ondeleted: vi.fn() },
  })
  flushSync()
  return onchanged
}
function button(text: string): HTMLButtonElement {
  const b = [...target.querySelectorAll('button')].find((b) => b.textContent?.trim() === text)
  if (!b) throw new Error(`no ${text} button`)
  return b
}
function click(el: HTMLElement) {
  el.click()
  flushSync()
}
function type(id: string, value: string) {
  const field = target.querySelector<HTMLInputElement>(`#${id}`)!
  field.value = value
  field.dispatchEvent(new Event('input', { bubbles: true }))
  flushSync()
}
// write replaces a rich text field's text, as typing would.
function write(id: string, markdown: string) {
  const field = target.querySelector<HTMLElement & { editor: Editor }>(`#${id}`)!
  field.editor.commands.setContent(markdown, { contentType: 'markdown' })
  flushSync()
}
const editForm = () => target.querySelector('form[aria-label="Edit idea"]')

describe('IdeaRow editing', () => {
  it('saves only the changed fields and closes the form', async () => {
    reply = () => new Response(JSON.stringify({ ...herbs, body: 'By the **back door**' }), { status: 200 })
    const onchanged = show()

    click(button('Edit'))
    expect(target.querySelector<HTMLInputElement>('#idea-4-title')!.value).toBe('Herb spiral')
    write('idea-4-body', 'By the **back door**')
    click(button('Save'))

    await vi.waitFor(() => expect(onchanged).toHaveBeenCalled())
    const [url, init] = fetchMock.mock.calls[0]
    expect([url, init?.method, JSON.parse(init!.body as string)]).toEqual([
      '/api/ideas/4',
      'PATCH',
      { body: 'By the **back door**' },
    ])
    expect(onchanged.mock.calls[0][0].body).toBe('By the **back door**')
    flushSync()
    expect(editForm()).toBeNull()
  })

  it('sends nothing when nothing changed, or when the title is empty', () => {
    show()
    click(button('Edit'))
    click(button('Save'))
    expect(editForm()).toBeNull()

    click(button('Edit'))
    type('idea-4-title', '  ')
    click(button('Save'))
    expect(target.textContent).toContain('Give it a short title.')
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('keeps the form open with the message when saving fails', async () => {
    reply = () =>
      new Response(JSON.stringify({ error: { code: 'internal', message: 'Something went wrong.' } }), { status: 500 })
    const onchanged = show()
    click(button('Edit'))
    type('idea-4-title', 'Herb tower')
    click(button('Save'))

    await vi.waitFor(() => expect(target.textContent).toContain('Something went wrong.'))
    expect(onchanged).not.toHaveBeenCalled()
    expect(editForm()).not.toBeNull()
  })

  it('cancel drops the changes', () => {
    show()
    click(button('Edit'))
    type('idea-4-title', 'Herb tower')
    click(button('Cancel'))
    expect(target.textContent).toContain('Herb spiral')
    expect(fetchMock).not.toHaveBeenCalled()
  })
})
