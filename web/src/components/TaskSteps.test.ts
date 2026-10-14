// @vitest-environment jsdom
import { flushSync, mount, tick, unmount } from 'svelte'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { TaskStatus } from '../lib/api/enums.gen'
import type { Task } from '../lib/api/types.gen'
import TaskSteps from './TaskSteps.svelte'

function task(id: number, status: TaskStatus, position: number, extra: Partial<Task> = {}): Task {
  return {
    id,
    project_id: 7,
    milestone_id: null,
    parent_task_id: 1,
    title: `S${id}`,
    notes: '',
    status,
    size: null,
    energy: null,
    position,
    origin_entry_id: null,
    created_at: '',
    updated_at: '',
    ...extra,
  }
}

const parent = task(1, 'todo', 1, { parent_task_id: null, title: 'Set up CI' })

let target: HTMLElement
let component: ReturnType<typeof mount> | undefined
let moved: Task[]
let failNext = false
const fetchMock = vi.fn(async (url: string, init?: RequestInit) => {
  if (failNext) {
    failNext = false
    return json({ error: { code: 'internal', message: 'Something went wrong.' } }, 500)
  }
  const method = init?.method ?? 'GET'
  const body = init?.body ? JSON.parse(init.body as string) : undefined
  if (url.endsWith('/move')) return json(moved)
  if (method === 'POST') return json(task(99, 'todo', 9, body), 201)
  if (method === 'DELETE') return new Response(null, { status: 204 })
  const id = Number(url.split('/').pop())
  return json(task(id, body.status, id + 1))
})

function json(v: unknown, status = 200): Response {
  return new Response(JSON.stringify(v), { status })
}

beforeEach(() => {
  fetchMock.mockClear()
  moved = []
  failNext = false
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

async function settle() {
  for (let i = 0; i < 5; i++) {
    await new Promise((r) => setTimeout(r, 0))
    await tick()
  }
  flushSync()
}

function render(steps: Task[]) {
  component = mount(TaskSteps, { target, props: { task: parent, steps } })
  flushSync()
}

function button(text: string): HTMLButtonElement {
  const b = [...target.querySelectorAll('button')].find((b) => b.textContent?.trim() === text || b.getAttribute('aria-label') === text)
  if (!b) throw new Error(`no ${text} button`)
  return b
}
function box(id: number): HTMLInputElement {
  return target.querySelector(`#step-${id}-done`) as HTMLInputElement
}
function titles(): string[] {
  return [...target.querySelectorAll('ol[aria-label="Steps"] > li a')].map((a) => a.textContent ?? '')
}
function writes(): { url: string; method?: string; body: unknown }[] {
  return fetchMock.mock.calls.map(([url, init]) => ({ url, method: init?.method, body: init?.body ? JSON.parse(init.body as string) : undefined }))
}

describe('TaskSteps', () => {
  it('lists the steps in order with done ones ticked, and encourages a first step when there are none', () => {
    render([task(2, 'done', 2), task(3, 'todo', 3)])
    expect(titles()).toEqual(['S2', 'S3'])
    expect(box(2).checked).toBe(true)
    expect(box(3).checked).toBe(false)
    unmount(component!)
    render([])
    expect(target.textContent).toContain('first small step')
  })

  it('adds a step under the task', async () => {
    render([task(2, 'todo', 2)])
    const input = target.querySelector('#task-step-title') as HTMLInputElement
    input.value = 'Write the workflow'
    input.dispatchEvent(new Event('input', { bubbles: true }))
    button('Add step').click()
    await settle()
    expect(writes()).toEqual([{ url: '/api/projects/7/tasks', method: 'POST', body: { title: 'Write the workflow', parent_task_id: 1 } }])
    expect(titles()).toEqual(['S2', 'Write the workflow'])
    expect(input.value).toBe('')
  })

  it('ticks a step done and back to todo', async () => {
    render([task(2, 'todo', 2)])
    box(2).click()
    await settle()
    box(2).click()
    await settle()
    expect(writes().map((w) => w.body)).toEqual([{ status: 'done' }, { status: 'todo' }])
    expect(box(2).checked).toBe(false)
  })

  it('puts the box back when ticking fails', async () => {
    render([task(2, 'todo', 2)])
    failNext = true
    box(2).click()
    await settle()
    expect(box(2).checked).toBe(false)
    expect(target.textContent).toContain("Something went wrong on Waypoint's side. Try again in a moment.")
  })

  it('moves a step and reorders by the positions the server returns', async () => {
    render([task(2, 'todo', 2), task(3, 'done', 3)])
    expect(button('Move S2 up').disabled).toBe(true)
    moved = [task(2, 'todo', 3), task(3, 'done', 2)]
    button('Move S2 down').click()
    await settle()
    expect(writes()).toEqual([{ url: '/api/tasks/2/move', method: 'POST', body: { direction: 'down' } }])
    expect(titles()).toEqual(['S3', 'S2'])
  })

  it('deletes a step only after confirming', async () => {
    render([task(2, 'todo', 2)])
    button('Delete S2').click()
    flushSync()
    expect(writes()).toEqual([])
    button('Keep it').click()
    flushSync()
    expect(target.textContent).not.toContain('Delete this step?')
    button('Delete S2').click()
    flushSync()
    button('Delete step').click()
    await settle()
    expect(writes()).toEqual([{ url: '/api/tasks/2', method: 'DELETE', body: undefined }])
    expect(titles()).toEqual([])
  })
})
