// @vitest-environment jsdom
import { flushSync, mount, unmount } from 'svelte'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { LinkedObservation, Project, TaskDetail } from '../lib/api/types.gen'
import { startEvents } from '../lib/events'
import { FakeEventSource, fakeOpen } from '../lib/events.fake'
import Task from './Task.svelte'

const project: Project = { id: 7, title: 'Waypoint', why: '', definition_of_done: '', status: 'active', created_at: '', updated_at: '', last_activity_at: '' }

function obs(id: number, entry_id: number, summary: string): LinkedObservation {
  return { id, entry_id, entry_created_at: '2026-09-30T10:00:00Z', kind: 'progress', summary, quote: `quote ${id}`, confidence: 1 }
}

function detail(extra: Partial<TaskDetail> = {}): TaskDetail {
  return {
    task: {
      id: 3,
      project_id: 7,
      parent_task_id: null,
      title: 'Write docs',
      notes: '',
      status: 'in_progress',
      size: 's',
      energy: null,
      position: 1,
      origin_entry_id: 1,
      created_at: '',
      updated_at: '',
    },
    origin_entry: { id: 1, body: 'todo: Write docs', project_id: 7, reply_to_question_id: null, created_at: '2026-09-29T09:00:00Z' },
    observations: [obs(10, 1, 'Plans docs'), obs(11, 1, 'Starts them'), obs(12, 2, 'Half done')],
    ...extra,
  }
}

let target: HTMLElement
let component: ReturnType<typeof mount>

// render mounts the screen and waits until loading has finished (and, by default, the project title has arrived).
async function render(task: Response, done = () => !target.textContent?.includes('Loading…') && !target.textContent?.includes('← Project')) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => (url.startsWith('/api/tasks/') ? task : new Response(JSON.stringify(project), { status: 200 }))),
  )
  target = document.createElement('div')
  document.body.append(target)
  component = mount(Task, { target, props: { id: 3 } })
  await vi.waitFor(() => {
    flushSync()
    if (!done()) throw new Error('still loading')
  })
}

const ok = (d: TaskDetail) => new Response(JSON.stringify(d), { status: 200 })

afterEach(() => {
  unmount(component)
  target.remove()
  vi.unstubAllGlobals()
})

describe('Task screen', () => {
  it('shows the task, its origin entry, and observations grouped by entry', async () => {
    await render(ok(detail()))
    expect(target.querySelector('h1')?.textContent).toBe('Write docs')
    expect(target.querySelector('a')?.textContent).toBe('← Waypoint')
    expect(target.textContent).toContain('In progress')
    expect(target.querySelector('[data-origin-entry="1"]')?.textContent).toContain('todo: Write docs')
    expect(target.querySelector('[data-origin-entry="1"] a')?.getAttribute('href')).toBe('/diary?entry=1')
    const groups = [...target.querySelectorAll('[data-entry-id]')]
    for (const g of groups) expect(g.querySelector('a')?.getAttribute('href')).toBe(`/diary?entry=${g.getAttribute('data-entry-id')}`)
    expect(groups.map((g) => [...g.querySelectorAll('[data-observation-id]')].map((o) => o.getAttribute('data-observation-id')))).toEqual([
      ['10', '11'],
      ['12'],
    ])
    expect(groups[0].textContent).toContain('Progress:')
    expect(groups[0].textContent).toContain('“quote 10”')
  })

  it('says calmly when a task was added by hand and has nothing written about it', async () => {
    await render(ok(detail({ origin_entry: null, observations: [] })))
    expect(target.textContent).toContain('Added by hand.')
    expect(target.textContent).toContain('Nothing yet.')
  })

  it('says the task is not here for an unknown id', async () => {
    await render(new Response(JSON.stringify({ error: { code: 'not_found', message: 'not found' } }), { status: 404 }), () => !target.textContent?.includes('Loading…'))
    expect(target.textContent).toContain("This task isn't here.")
    expect(target.querySelector('h1')?.textContent).toBe('Task')
  })

  it("reloads after a changeset touches the task's project, and on reconnect", async () => {
    FakeEventSource.all = []
    const stopEvents = startEvents(fakeOpen)
    let current = detail()
    const fetchMock = vi.fn(async (url: string) =>
      new Response(JSON.stringify(url.startsWith('/api/tasks/') ? current : project), { status: 200 }),
    )
    vi.stubGlobal('fetch', fetchMock)
    target = document.createElement('div')
    document.body.append(target)
    component = mount(Task, { target, props: { id: 3 } })
    const shows = (title: string) =>
      vi.waitFor(() => {
        flushSync()
        if (target.querySelector('h1')?.textContent !== title) throw new Error('not yet')
      })
    await shows('Write docs')
    const taskLoads = () => fetchMock.mock.calls.filter(([u]) => u === '/api/tasks/3').length

    const src = FakeEventSource.latest()
    current = detail({ task: { ...detail().task, title: 'Write the docs' } })
    src.emit('changeset.applied', { changeset_id: 2, project_ids: [8] })
    await new Promise((r) => setTimeout(r, 0))
    expect(taskLoads()).toBe(1)
    src.emit('changeset.applied', { changeset_id: 3, project_ids: [7] })
    await shows('Write the docs')

    src.open()
    current = detail({ task: { ...detail().task, title: 'Docs written' } })
    src.fail()
    src.open()
    await shows('Docs written')
    stopEvents()
  })
})
