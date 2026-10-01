// @vitest-environment jsdom
import { flushSync, mount, tick, unmount } from 'svelte'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { TaskStatus } from '../lib/api/enums.gen'
import type { Task } from '../lib/api/types.gen'
import ProjectTasks from './ProjectTasks.svelte'

function task(id: number, status: TaskStatus, position: number, extra: Partial<Task> = {}): Task {
  return {
    id,
    project_id: 7,
    parent_task_id: null,
    title: `T${id}`,
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

let target: HTMLElement
let component: ReturnType<typeof mount> | undefined
let server: Task[]
const fetchMock = vi.fn(async (url: string, init?: RequestInit) => {
  const method = init?.method ?? 'GET'
  const body = init?.body ? JSON.parse(init.body as string) : undefined
  if (method === 'GET') return json(server)
  if (method === 'POST') return json(task(99, 'todo', 9, body), 201)
  const id = Number(url.split('/').pop())
  if (method === 'DELETE') return new Response(null, { status: 204 })
  const t = { ...server.find((x) => x.id === id)!, ...body }
  return json(t)
})

function json(v: unknown, status = 200): Response {
  return new Response(JSON.stringify(v), { status })
}

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

async function settle() {
  for (let i = 0; i < 5; i++) {
    await new Promise((r) => setTimeout(r, 0))
    await tick()
  }
  flushSync()
}

async function render(tasks: Task[]) {
  server = tasks
  component = mount(ProjectTasks, { target, props: { projectId: 7 } })
  await settle()
}

function button(text: string): HTMLButtonElement {
  const b = [...target.querySelectorAll('button')].find((b) => b.textContent?.trim() === text || b.getAttribute('aria-label') === text)
  if (!b) throw new Error(`no ${text} button`)
  return b
}
function type(id: string, value: string) {
  const field = target.querySelector<HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement>(`#${id}`)!
  field.value = value
  field.dispatchEvent(new Event(field instanceof HTMLSelectElement ? 'change' : 'input', { bubbles: true }))
  flushSync()
}
function submit(form: HTMLFormElement) {
  form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
}
function writes(): { url: string; method?: string; body: unknown }[] {
  return fetchMock.mock.calls
    .filter(([, init]) => init?.method && init.method !== 'GET')
    .map(([url, init]) => ({ url, method: init?.method, body: init?.body ? JSON.parse(init.body as string) : undefined }))
}
function titles(group: string): string[] {
  return [...target.querySelectorAll(`ul[aria-label="${group}"] > li a`)].map((a) => a.textContent ?? '')
}

describe('ProjectTasks', () => {
  it('groups tasks by status, ordered by position', async () => {
    await render([task(1, 'todo', 2), task(2, 'in_progress', 1), task(3, 'todo', 1)])
    expect([...target.querySelectorAll('h3')].map((h) => h.textContent)).toEqual(['In progress', 'To do'])
    expect(titles('To do')).toEqual(['T3', 'T1'])
  })

  it('shows an encouraging empty state', async () => {
    await render([])
    expect(target.textContent).toContain('No tasks yet')
  })

  it('adds a task', async () => {
    await render([])
    type('new-task-title', ' Buy seeds ')
    type('new-task-size', 'xs')
    submit(target.querySelector('form')!)
    await settle()
    expect(writes()).toEqual([{ url: '/api/projects/7/tasks', method: 'POST', body: { title: 'Buy seeds', size: 'xs' } }])
    expect(titles('To do')).toEqual(['Buy seeds'])
  })

  it('saves only the fields that changed', async () => {
    await render([task(1, 'todo', 1, { size: 's' })])
    button('Edit').click()
    flushSync()
    type('task-1-title', 'Dig')
    type('task-1-size', '')
    submit(target.querySelector('li form')!)
    await settle()
    expect(writes()).toEqual([{ url: '/api/tasks/1', method: 'PATCH', body: { title: 'Dig', size: null } }])
    expect(titles('To do')).toEqual(['Dig'])
  })

  it('changes status and moves the task to its new group', async () => {
    await render([task(1, 'todo', 1)])
    type('task-1-status', 'done')
    await settle()
    expect(writes()).toEqual([{ url: '/api/tasks/1', method: 'PATCH', body: { status: 'done' } }])
    expect(titles('Done')).toEqual(['T1'])
  })

  it('moves a task down by swapping positions with its neighbour', async () => {
    await render([task(1, 'todo', 1), task(2, 'done', 2), task(3, 'todo', 3)])
    expect(button('Move T1 up').disabled).toBe(true)
    button('Move T1 down').click()
    await settle()
    expect(writes()).toEqual([
      { url: '/api/tasks/1', method: 'PATCH', body: { position: 3 } },
      { url: '/api/tasks/3', method: 'PATCH', body: { position: 1 } },
    ])
    expect(titles('To do')).toEqual(['T3', 'T1'])
  })

  it('deletes only after confirming', async () => {
    await render([task(1, 'todo', 1)])
    button('Delete').click()
    flushSync()
    expect(writes()).toEqual([])
    button('Keep it').click()
    flushSync()
    expect(target.textContent).not.toContain('Delete this task?')
    button('Delete').click()
    flushSync()
    button('Delete task').click()
    await settle()
    expect(writes()).toEqual([{ url: '/api/tasks/1', method: 'DELETE', body: undefined }])
    expect(target.textContent).toContain('No tasks yet')
  })
})
