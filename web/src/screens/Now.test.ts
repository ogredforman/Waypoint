// @vitest-environment jsdom
import { flushSync, mount, unmount } from 'svelte'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Briefing, Project } from '../lib/api/types.gen'
import { startEvents } from '../lib/events'
import { FakeEventSource, fakeOpen } from '../lib/events.fake'
import Now from './Now.svelte'

const project = (id: number, title: string): Project => ({
  id,
  title,
  why: '',
  definition_of_done: '',
  status: 'active',
  created_at: '',
  updated_at: '',
})

const briefing = (projectID: number, recap: string): Briefing => ({
  project_id: projectID,
  next_task: null,
  next_reason: '',
  recap,
  source: 'heuristic',
  excerpt: null,
  blockers: [],
  alternatives: [],
  generated_at: '',
})

// A fake API: routes holds the answer, or a promise of one, for each GET path.
const routes = new Map<string, unknown>()
const fetchMock = vi.fn(async (url: string) => {
  if (!routes.has(url)) return new Response('{}', { status: 404 })
  return new Response(JSON.stringify(await routes.get(url)), { status: 200 })
})

let target: HTMLElement
let component: ReturnType<typeof mount>
let stopEvents: () => void

beforeEach(() => {
  routes.clear()
  routes.set('/api/projects', [project(1, 'Waypoint')])
  routes.set('/api/projects/1/briefing', briefing(1, 'first recap'))
  vi.stubGlobal('fetch', fetchMock)
  FakeEventSource.all = []
  stopEvents = startEvents(fakeOpen)
  target = document.createElement('div')
  document.body.append(target)
  component = mount(Now, { target })
})

afterEach(() => {
  unmount(component)
  stopEvents()
  target.remove()
  vi.unstubAllGlobals()
})

// settle lets pending fetches finish and the DOM catch up.
async function settle() {
  for (let i = 0; i < 5; i++) await new Promise((resolve) => setTimeout(resolve, 0))
  flushSync()
}

const text = () => target.textContent ?? ''

describe('Now', () => {
  it('refetches a briefing when it is updated', async () => {
    await settle()
    expect(text()).toContain('first recap')
    routes.set('/api/projects/1/briefing', briefing(1, 'second recap'))
    FakeEventSource.latest().emit('briefing.updated', { project_id: 1 })
    await settle()
    expect(text()).toContain('second recap')
  })

  it('keeps a fresh briefing that arrives after an applied changeset reloaded the list', async () => {
    await settle()
    let release: (b: Briefing) => void = () => {}
    routes.set('/api/projects/1/briefing', new Promise<Briefing>((resolve) => (release = resolve)))
    const src = FakeEventSource.latest()
    src.emit('briefing.updated', { project_id: 1 })
    src.emit('changeset.applied', { changeset_id: 3, project_ids: [1] })
    await settle()
    release(briefing(1, 'fresh recap'))
    await settle()
    expect(text()).toContain('fresh recap')
  })

  it('shows a project that became active', async () => {
    await settle()
    routes.set('/api/projects', [project(1, 'Waypoint'), project(2, 'Garden')])
    routes.set('/api/projects/2/briefing', briefing(2, 'garden recap'))
    FakeEventSource.latest().emit('changeset.applied', { changeset_id: 4, project_ids: [2] })
    await settle()
    expect(text()).toContain('Garden')
    expect(text()).toContain('garden recap')
    expect(text()).toContain('first recap')
  })
})
