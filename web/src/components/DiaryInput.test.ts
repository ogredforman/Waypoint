// @vitest-environment jsdom
import { flushSync, mount, tick, unmount } from 'svelte'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ProjectStatus } from '../lib/api/enums.gen'
import type { Project } from '../lib/api/types.gen'
import { startEvents } from '../lib/events'
import { FakeEventSource, fakeOpen } from '../lib/events.fake'
import DiaryInput from './DiaryInput.svelte'

function project(id: number, title: string, status: ProjectStatus = 'active'): Project {
  return { id, title, why: '', definition_of_done: '', status, created_at: '', updated_at: '', last_activity_at: '' }
}

let target: HTMLElement
let component: ReturnType<typeof mount> | undefined
let stopEvents: () => void
let server: Project[]
let failing: boolean
const fetchMock = vi.fn(async () => {
  if (failing) return new Response(JSON.stringify({ error: 'down' }), { status: 500 })
  return new Response(JSON.stringify(server), { status: 200 })
})

beforeEach(() => {
  fetchMock.mockClear()
  failing = false
  vi.stubGlobal('fetch', fetchMock)
  FakeEventSource.all = []
  stopEvents = startEvents(fakeOpen)
  target = document.createElement('div')
  document.body.append(target)
})

afterEach(() => {
  if (component) unmount(component)
  component = undefined
  stopEvents()
  target.remove()
  vi.unstubAllGlobals()
})

async function settle() {
  for (let i = 0; i < 5; i++) {
    await new Promise((r) => setTimeout(r, 0))
    await tick()
  }
  flushSync()
}

async function render(projects: Project[]) {
  server = projects
  component = mount(DiaryInput, { target })
  await settle()
}

function options(): string[] {
  return [...target.querySelectorAll('option')].map((o) => o.textContent?.trim() ?? '')
}

const applied = () => FakeEventSource.latest().emit('changeset.applied', { changeset_id: 1, project_ids: [] })

describe('DiaryInput project picker', () => {
  it('picks up a project created after it loaded', async () => {
    await render([project(1, 'Garden')])
    expect(options()).toEqual(['No project', 'Garden'])
    server = [project(1, 'Garden'), project(2, 'Kiln')]
    applied()
    await settle()
    expect(options()).toEqual(['No project', 'Garden', 'Kiln'])
  })

  it('shows a rename and drops a project moved to done', async () => {
    await render([project(1, 'Garden'), project(2, 'Kiln')])
    server = [project(1, 'Vegetable garden'), project(2, 'Kiln', 'done')]
    applied()
    await settle()
    expect(options()).toEqual(['No project', 'Vegetable garden'])
  })

  it('reloads when the event stream reconnects', async () => {
    const s = FakeEventSource.latest()
    s.open()
    await render([project(1, 'Garden')])
    server = [project(1, 'Garden'), project(3, 'Bike')]
    s.fail()
    s.open()
    await settle()
    expect(options()).toEqual(['No project', 'Garden', 'Bike'])
  })

  it('keeps the list it has when a reload fails', async () => {
    await render([project(1, 'Garden')])
    failing = true
    applied()
    await settle()
    expect(options()).toEqual(['No project', 'Garden'])
  })

  it('stops reloading once unmounted', async () => {
    await render([project(1, 'Garden')])
    unmount(component!)
    component = undefined
    const calls = fetchMock.mock.calls.length
    applied()
    await settle()
    expect(fetchMock.mock.calls.length).toBe(calls)
  })
})
