// @vitest-environment jsdom
import { flushSync, mount, unmount } from 'svelte'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Project } from '../lib/api/types.gen'
import ProjectHeader from './ProjectHeader.svelte'
import ProjectStatus from './ProjectStatus.svelte'

const garden: Project = {
  id: 7,
  title: 'Garden',
  why: 'Fresh food',
  definition_of_done: '',
  status: 'parked',
  created_at: '',
  updated_at: '',
  last_activity_at: '',
}

let target: HTMLElement
let component: ReturnType<typeof mount> | undefined
let reply: () => Response
const fetchMock = vi.fn(async (_url: string, _init?: RequestInit) => reply())

beforeEach(() => {
  fetchMock.mockClear()
  vi.stubGlobal('fetch', fetchMock)
  target = document.createElement('div')
  document.body.append(target)
})

afterEach(() => {
  if (component) unmount(component)
  component = undefined
  target.remove()
  vi.unstubAllGlobals()
})

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
  const field = target.querySelector<HTMLInputElement | HTMLTextAreaElement>(`#${id}`)!
  field.value = value
  field.dispatchEvent(new Event('input', { bubbles: true }))
  flushSync()
}
function sent(): { url: string; method?: string; body: unknown } {
  const [url, init] = fetchMock.mock.calls[0]
  return { url, method: init?.method, body: JSON.parse(init!.body as string) }
}

describe('ProjectHeader', () => {
  it('saves only the fields that changed', async () => {
    reply = () => new Response(JSON.stringify({ ...garden, title: 'Allotment' }), { status: 200 })
    const onsaved = vi.fn()
    component = mount(ProjectHeader, { target, props: { project: garden, onsaved } })
    flushSync()
    expect(target.textContent).toContain('Fresh food')

    click(button('Edit'))
    type('edit-project-title', ' Allotment ')
    type('edit-project-dod', 'Beds planted')
    click(button('Save'))

    await vi.waitFor(() => expect(onsaved).toHaveBeenCalled())
    expect(sent()).toEqual({ url: '/api/projects/7', method: 'PATCH', body: { title: 'Allotment', definition_of_done: 'Beds planted' } })
    expect(onsaved.mock.calls[0][0].title).toBe('Allotment')
  })

  it('closes without a request when nothing changed', () => {
    component = mount(ProjectHeader, { target, props: { project: garden, onsaved: () => {} } })
    flushSync()
    click(button('Edit'))
    click(button('Save'))
    expect(fetchMock).not.toHaveBeenCalled()
    expect(target.querySelector('form')).toBeNull()
  })

  it('shows field problems from the server next to the field', async () => {
    reply = () =>
      new Response(JSON.stringify({ error: { code: 'invalid', message: 'invalid', problems: [{ field: 'title', problem: 'too long' }] } }), {
        status: 422,
      })
    component = mount(ProjectHeader, { target, props: { project: garden, onsaved: () => {} } })
    flushSync()
    click(button('Edit'))
    type('edit-project-title', 'Allotment')
    click(button('Save'))
    await vi.waitFor(() => expect(target.querySelector('#edit-project-title-problem')?.textContent).toBe('too long'))
  })
})

describe('ProjectStatus', () => {
  it('sends a status change', async () => {
    reply = () => new Response(JSON.stringify({ ...garden, status: 'done' }), { status: 200 })
    const onchanged = vi.fn()
    component = mount(ProjectStatus, { target, props: { project: garden, activeCount: 0, cap: 3, onchanged } })
    flushSync()
    expect(button('Parked').getAttribute('aria-pressed')).toBe('true')

    click(button('Done'))
    await vi.waitFor(() => expect(onchanged).toHaveBeenCalled())
    expect(sent()).toEqual({ url: '/api/projects/7', method: 'PATCH', body: { status: 'done' } })
  })

  it('disables active at the cap and says why calmly', () => {
    component = mount(ProjectStatus, { target, props: { project: garden, activeCount: 3, cap: 3, onchanged: () => {} } })
    flushSync()
    expect(button('Active').disabled).toBe(true)
    expect(target.textContent).toContain('the limit is 3')
  })

  it('turns an active_cap answer into a calm notice and disables active', async () => {
    reply = () => new Response(JSON.stringify({ error: { code: 'active_cap', message: 'cap reached' } }), { status: 409 })
    const onchanged = vi.fn()
    component = mount(ProjectStatus, { target, props: { project: garden, activeCount: 1, cap: 3, onchanged } })
    flushSync()

    click(button('Active'))
    await vi.waitFor(() => expect(target.querySelector('[role="status"]')?.textContent).toContain('active-project limit'))
    expect(button('Active').disabled).toBe(true)
    expect(onchanged).not.toHaveBeenCalled()
  })
})
