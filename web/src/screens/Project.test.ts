// @vitest-environment jsdom
import { flushSync, mount, tick, unmount } from 'svelte'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Blocker, Idea, Project as ProjectT, ProjectObservations, Task } from '../lib/api/types.gen'
import { startEvents } from '../lib/events'
import { FakeEventSource, fakeOpen } from '../lib/events.fake'
import Project from './Project.svelte'

const project = (id: number, title: string): ProjectT => ({
  id,
  title,
  why: '',
  definition_of_done: '',
  status: 'active',
  created_at: '',
  updated_at: '',
  last_activity_at: '',
})

const task = (id: number, title: string): Task => ({
  id,
  project_id: 7,
  milestone_id: null,
  parent_task_id: null,
  title,
  notes: '',
  status: 'todo',
  size: null,
  energy: null,
  position: id,
  origin_entry_id: null,
  created_at: '',
  updated_at: '',
})

const blocker = (id: number, description: string): Blocker => ({
  id,
  project_id: 7,
  task_id: null,
  description,
  status: 'open',
  origin_entry_id: null,
  resolved_entry_id: null,
  created_at: '',
  updated_at: '',
})

const diary = (summary: string): ProjectObservations => ({
  observations: [
    { id: 1, entry_id: 3, entry_created_at: '2026-10-01T09:00:00Z', kind: 'feeling', summary, quote: '', confidence: 1 },
  ],
  next_before: null,
})

const idea = (id: number, title: string): Idea => ({
  id,
  project_id: 7,
  title,
  body: '',
  status: 'parked',
  origin_entry_id: null,
  created_at: '',
  updated_at: '',
})

// A fake API: routes holds the answer for each GET path.
const routes = new Map<string, unknown>()
const fetchMock = vi.fn(async (url: string) => {
  if (!routes.has(url)) return new Response('{}', { status: 404 })
  return new Response(JSON.stringify(routes.get(url)), { status: 200 })
})

let target: HTMLElement
let component: ReturnType<typeof mount>
let stopEvents: () => void

beforeEach(() => {
  routes.clear()
  routes.set('/api/projects/7', project(7, 'Waypoint'))
  routes.set('/api/projects', [project(7, 'Waypoint')])
  routes.set('/api/settings', { active_project_cap: 3 })
  routes.set('/api/projects/7/tasks', [task(1, 'Write docs')])
  routes.set('/api/projects/7/blockers', [blocker(1, 'Waiting on review')])
  routes.set('/api/ideas?project_id=7', [idea(1, 'Dark mode')])
  routes.set('/api/projects/7/observations?limit=10', diary('Felt stuck'))
  fetchMock.mockClear()
  vi.stubGlobal('fetch', fetchMock)
  FakeEventSource.all = []
  stopEvents = startEvents(fakeOpen)
  target = document.createElement('div')
  document.body.append(target)
})

afterEach(() => {
  unmount(component)
  stopEvents()
  target.remove()
  vi.unstubAllGlobals()
})

async function render() {
  component = mount(Project, { target, props: { id: 7 } })
  await settle()
}

async function settle() {
  for (let i = 0; i < 5; i++) {
    await new Promise((r) => setTimeout(r, 0))
    await tick()
  }
  flushSync()
}

const text = () => target.textContent ?? ''
const fetched = (url: string) => fetchMock.mock.calls.filter(([u]) => u === url).length

function changeAll() {
  routes.set('/api/projects/7', project(7, 'Waypoint 2'))
  routes.set('/api/projects/7/tasks', [task(1, 'Write docs'), task(2, 'Ship it')])
  routes.set('/api/projects/7/blockers', [blocker(1, 'Waiting on review'), blocker(2, 'Need a key')])
  routes.set('/api/ideas?project_id=7', [idea(1, 'Dark mode'), idea(2, 'Export')])
  routes.set('/api/projects/7/observations?limit=10', diary('Felt better'))
}

describe('Project screen live updates', () => {
  it("reloads the header, tasks, blockers, and ideas after a changeset touches this project", async () => {
    await render()
    changeAll()
    FakeEventSource.latest().emit('changeset.applied', { changeset_id: 2, project_ids: [7] })
    await settle()
    expect(target.querySelector('h1')?.textContent).toContain('Waypoint 2')
    expect(text()).toContain('Ship it')
    expect(text()).toContain('Need a key')
    expect(text()).toContain('Export')
    expect(text()).toContain('Felt better')
  })

  it("reloads only the ideas and the cap count after another project's changeset", async () => {
    await render()
    changeAll()
    FakeEventSource.latest().emit('changeset.applied', { changeset_id: 3, project_ids: [8] })
    await settle()
    expect(text()).not.toContain('Ship it')
    expect(text()).not.toContain('Need a key')
    expect(text()).toContain('Export')
    expect(text()).not.toContain('Felt better')
    expect(fetched('/api/projects/7')).toBe(1)
    // Once for the task list and once for the blockers' task picker.
    expect(fetched('/api/projects/7/tasks')).toBe(2)
    expect(fetched('/api/projects/7/blockers')).toBe(1)
    expect(fetched('/api/settings')).toBe(2)
  })

  it('reloads everything when the event stream reconnects', async () => {
    await render()
    const src = FakeEventSource.latest()
    src.open()
    changeAll()
    src.fail()
    src.open()
    await settle()
    expect(target.querySelector('h1')?.textContent).toContain('Waypoint 2')
    expect(text()).toContain('Ship it')
    expect(text()).toContain('Need a key')
    expect(text()).toContain('Export')
    expect(text()).toContain('Felt better')
  })

  it('keeps an open task edit form through a reload', async () => {
    await render()
    const edit = [...target.querySelectorAll('button')].find((b) => b.textContent?.trim() === 'Edit' && b.closest('li'))
    if (!edit) throw new Error('no task Edit button')
    edit.click()
    flushSync()
    const input = target.querySelector<HTMLInputElement>('#task-1-title')!
    input.value = 'Write the docs'
    input.dispatchEvent(new Event('input', { bubbles: true }))
    routes.set('/api/projects/7/tasks', [task(1, 'Write docs'), task(2, 'Ship it')])
    FakeEventSource.latest().emit('changeset.applied', { changeset_id: 4, project_ids: [7] })
    await settle()
    expect(text()).toContain('Ship it')
    expect(target.querySelector<HTMLInputElement>('#task-1-title')?.value).toBe('Write the docs')
  })
})

describe('Project screen headings', () => {
  it('keeps an <h1> while the title is being edited', async () => {
    await render()
    const edit = [...target.querySelectorAll('button')].find((b) => b.textContent?.trim() === 'Edit' && !b.closest('li'))
    if (!edit) throw new Error('no project Edit button')
    edit.click()
    flushSync()
    expect(target.querySelector('#edit-project-title')).not.toBeNull()
    expect(target.querySelector('h1')?.textContent).toBe('Waypoint')
  })

  it("has an <h1> when the project can't be loaded", async () => {
    routes.delete('/api/projects/7')
    await render()
    expect(text()).toContain('Try again')
    expect(target.querySelector('h1')?.textContent).toBe('Project')
  })
})
