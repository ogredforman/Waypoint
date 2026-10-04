// @vitest-environment jsdom
import { flushSync, mount, unmount } from 'svelte'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Project, ProjectDeletion } from '../lib/api/types.gen'
import { current, navigate } from '../lib/navigation.svelte'
import ProjectDelete from './ProjectDelete.svelte'

const garden: Project = {
  id: 7,
  title: 'Garden',
  why: '',
  definition_of_done: '',
  status: 'parked',
  created_at: '',
  updated_at: '',
  last_activity_at: '',
}

let target: HTMLElement
let component: ReturnType<typeof mount> | undefined
let preview: ProjectDeletion
let deleteReply: () => Response
const fetchMock = vi.fn(async (url: string, init?: RequestInit) => {
  if (init?.method === 'DELETE') return deleteReply()
  if (url.endsWith('/deletion')) return new Response(JSON.stringify(preview), { status: 200 })
  return new Response('{}', { status: 404 })
})

beforeEach(() => {
  fetchMock.mockClear()
  vi.stubGlobal('fetch', fetchMock)
  vi.stubGlobal('scrollTo', () => {})
  navigate('/projects/7', { replace: true })
  preview = { tasks: 3, blockers: 1, ideas: 0, milestones: 0, entries: 2 }
  deleteReply = () => new Response(null, { status: 204 })
  target = document.createElement('div')
  document.body.append(target)
  component = mount(ProjectDelete, { target, props: { project: garden } })
})

afterEach(() => {
  if (component) unmount(component)
  component = undefined
  target.remove()
  vi.unstubAllGlobals()
})

const settle = () => new Promise((resolve) => setTimeout(resolve, 0))

function button(text: string): HTMLButtonElement {
  const b = [...target.querySelectorAll('button')].find((b) => b.textContent?.trim() === text)
  if (!b) throw new Error(`no ${text} button`)
  return b
}
async function click(el: HTMLElement) {
  el.click()
  await settle()
  flushSync()
}
function deleteURL(): string | undefined {
  return fetchMock.mock.calls.find(([, init]) => init?.method === 'DELETE')?.[0]
}

describe('ProjectDelete', () => {
  it('shows what goes with the project and keeps its entries by default', async () => {
    await click(button('Delete project…'))
    expect(target.textContent).toContain('3 tasks and 1 blocker go with it.')
    expect(target.textContent).toContain('its 2 diary entries')
    const keep = target.querySelector<HTMLInputElement>('input[value="keep"]')!
    expect(keep.checked).toBe(true)
    await click(button('Delete project'))
    expect(deleteURL()).toBe('/api/projects/7?entries=keep')
    expect(current.path).toBe('/projects')
  })

  it('deletes the entries too when asked', async () => {
    await click(button('Delete project…'))
    const del = target.querySelector<HTMLInputElement>('input[value="delete"]')!
    del.click()
    flushSync()
    await click(button('Delete project'))
    expect(deleteURL()).toBe('/api/projects/7?entries=delete')
  })

  it("doesn't ask about entries when there are none", async () => {
    preview = { tasks: 0, blockers: 0, ideas: 0, milestones: 0, entries: 0 }
    await click(button('Delete project…'))
    expect(target.textContent).toContain("There's nothing in it yet.")
    expect(target.querySelector('fieldset')).toBeNull()
  })

  it('stays put with a calm note when an entry is still being read', async () => {
    deleteReply = () =>
      new Response(JSON.stringify({ error: { code: 'run_in_progress', message: 'busy' } }), { status: 409 })
    await click(button('Delete project…'))
    await click(button('Delete project'))
    expect(target.querySelector('[role="status"]')?.textContent).toContain('still reading')
    expect(current.path).toBe('/projects/7')
  })

  it('keeps the project on Keep the project', async () => {
    await click(button('Delete project…'))
    await click(button('Keep the project'))
    expect(deleteURL()).toBeUndefined()
    expect(target.textContent).toContain('Delete project…')
  })
})
