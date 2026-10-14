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
  last_activity_at: '',
})

const briefing = (projectID: number, recap: string): Briefing => ({
  project_id: projectID,
  next_task: null,
  next_reason: '',
  next_step: null,
  recap: [{ entry_id: 1, created_at: new Date().toISOString(), points: [{ kind: 'progress', text: recap }] }],
  source: 'heuristic',
  excerpt: null,
  blockers: [],
  alternatives: [],
  generated_at: '',
})

// A fake API: routes holds the answer, or a promise of one, for each GET path.
const routes = new Map<string, unknown>()
// A POST to /api/projects answers with created, and adds it to the list.
let created: Project | null = null
const fetchMock = vi.fn(async (url: string, init?: RequestInit) => {
  if (init?.method === 'POST' && url === '/api/projects' && created) {
    routes.set('/api/projects', [created])
    return new Response(JSON.stringify(created), { status: 201 })
  }
  if (!routes.has(url)) return new Response('{}', { status: 404 })
  return new Response(JSON.stringify(await routes.get(url)), { status: 200 })
})

let target: HTMLElement
let component: ReturnType<typeof mount>
let stopEvents: () => void

beforeEach(() => {
  routes.clear()
  created = null
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

// remount shows Now again with the routes as they are now.
async function remount() {
  unmount(component)
  component = mount(Now, { target })
  await settle()
}

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

  it('says where to find parked projects when none is active', async () => {
    routes.set('/api/projects', [{ ...project(1, 'Waypoint'), status: 'parked' }])
    await remount()
    expect(text()).toContain('No active projects right now')
    expect(target.querySelector('a[href="/parking-lot"]')).not.toBeNull()
    expect(target.querySelector('#project-title')).toBeNull()
  })
})

describe('Now on first run', () => {
  beforeEach(() => {
    routes.set('/api/projects', [])
    routes.set('/api/provider', { provider: 'anthropic', model: '', endpoint: '', api_key_set: true, from_env: [], providers: [] })
  })

  it('welcomes and offers the first project, set to start active', async () => {
    await remount()
    expect(text()).toContain('Welcome')
    expect(target.querySelector('#project-title')).not.toBeNull()
    const active = target.querySelector<HTMLInputElement>('input[name="project-start"][value="true"]')
    expect(active?.checked).toBe(true)
    expect(text()).not.toContain('No model is set up')
  })

  it('says how entries are read when no model is set up', async () => {
    routes.set('/api/provider', { provider: 'mock', model: '', endpoint: '', api_key_set: false, from_env: [], providers: [] })
    await remount()
    expect(text()).toContain('No model is set up yet')
    expect(target.querySelector('a[href="/settings"]')).not.toBeNull()
  })

  it("shows the new project's briefing once it is created", async () => {
    await remount()
    created = project(7, 'Garden shed')
    routes.set('/api/projects/7/briefing', briefing(7, 'shed recap'))
    const title = target.querySelector<HTMLInputElement>('#project-title')!
    title.value = 'Garden shed'
    title.dispatchEvent(new Event('input', { bubbles: true }))
    flushSync()
    target.querySelector('form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await settle()
    expect(text()).not.toContain('Welcome')
    expect(text()).toContain('Garden shed')
    expect(text()).toContain('shed recap')
    const sent = fetchMock.mock.calls.find(([, init]) => init?.method === 'POST')
    expect(JSON.parse(String(sent?.[1]?.body))).toMatchObject({ title: 'Garden shed', status: 'active' })
  })
})
