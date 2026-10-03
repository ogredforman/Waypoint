// @vitest-environment jsdom
import { flushSync, mount, tick, unmount } from 'svelte'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Briefing, Task } from '../lib/api/types.gen'
import { startEvents } from '../lib/events'
import { FakeEventSource, fakeOpen } from '../lib/events.fake'
import ProjectNext from './ProjectNext.svelte'

function briefing(next: Task | null, reason = ''): Briefing {
  return {
    project_id: 7,
    next_task: next,
    next_reason: reason,
    recap: '',
    source: 'heuristic',
    excerpt: null,
    blockers: [],
    alternatives: [],
    generated_at: '2026-10-03T09:00:00Z',
  }
}

let answer: Briefing | null = null
const fetchMock = vi.fn(async (url: string) => {
  if (url !== '/api/projects/7/briefing' || answer === null) {
    return new Response('{"error":{"code":"internal","message":"Something went wrong."}}', { status: 500 })
  }
  return new Response(JSON.stringify(answer), { status: 200 })
})

let target: HTMLElement
let component: ReturnType<typeof mount> | undefined
let stopEvents: () => void

beforeEach(() => {
  answer = null
  fetchMock.mockClear()
  vi.stubGlobal('fetch', fetchMock)
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

async function render() {
  component = mount(ProjectNext, { target, props: { projectId: 7 } })
  await settle()
}

const task = (id: number, title: string) => ({ id, title, status: 'todo' }) as Task

describe('ProjectNext', () => {
  it('shows the next task with its reason and a link to it', async () => {
    answer = briefing(task(3, 'Write the README'), 'It is the first open task.')
    await render()
    const link = target.querySelector('a')!
    expect(link.textContent).toBe('Write the README')
    expect(link.getAttribute('href')).toBe('/tasks/3')
    expect(target.textContent).toContain('It is the first open task.')
  })

  it('says so calmly when nothing is ready', async () => {
    answer = briefing(null)
    await render()
    expect(target.querySelector('a')).toBeNull()
    expect(target.textContent).toContain('Nothing here is ready to start yet.')
  })

  it('shows a load failure with a way to try again', async () => {
    await render()
    expect(target.textContent).toContain('Something went wrong.')
    answer = briefing(task(3, 'Write the README'))
    ;[...target.querySelectorAll('button')].find((b) => b.textContent?.trim() === 'Try again')!.click()
    await settle()
    expect(target.querySelector('a')?.textContent).toBe('Write the README')
  })

  it("reloads when this project's briefing is updated, not another's", async () => {
    answer = briefing(null)
    await render()
    expect(fetchMock).toHaveBeenCalledTimes(1)

    FakeEventSource.latest().emit('briefing.updated', { project_id: 8 })
    await settle()
    expect(fetchMock).toHaveBeenCalledTimes(1)

    answer = briefing(task(4, 'Add a logo'))
    FakeEventSource.latest().emit('briefing.updated', { project_id: 7 })
    await settle()
    expect(fetchMock).toHaveBeenCalledTimes(2)
    expect(target.querySelector('a')?.textContent).toBe('Add a logo')
  })
})
