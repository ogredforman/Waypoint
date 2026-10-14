// @vitest-environment jsdom
import { flushSync, mount, tick, unmount } from 'svelte'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { LinkedObservation, ProjectObservations } from '../lib/api/types.gen'
import { startEvents } from '../lib/events'
import { FakeEventSource, fakeOpen } from '../lib/events.fake'
import ProjectDiary from './ProjectDiary.svelte'

function obs(id: number, entryId: number, kind: LinkedObservation['kind'], summary: string): LinkedObservation {
  return { id, entry_id: entryId, entry_created_at: `2026-10-0${entryId}T09:00:00Z`, kind, summary, quote: `${summary}!`, confidence: 1 }
}

// routes holds the answer for each GET path.
const routes = new Map<string, ProjectObservations>()
const fetchMock = vi.fn(async (url: string) => {
  if (!routes.has(url)) return new Response('{"error":{"code":"internal","message":"Something went wrong."}}', { status: 500 })
  return new Response(JSON.stringify(routes.get(url)), { status: 200 })
})

let target: HTMLElement
let component: ReturnType<typeof mount> | undefined
let stopEvents: () => void

beforeEach(() => {
  routes.clear()
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
  component = mount(ProjectDiary, { target, props: { projectId: 7 } })
  await settle()
}

const entryIds = () => [...target.querySelectorAll<HTMLElement>('li[data-entry-id]')].map((li) => Number(li.dataset.entryId))
const showMore = () => [...target.querySelectorAll('button')].find((b) => b.textContent?.trim() === 'Show more')

describe('ProjectDiary', () => {
  it('groups observations by entry, newest first, with their kinds highlighted', async () => {
    routes.set('/api/projects/7/observations?limit=10', {
      observations: [obs(5, 3, 'plan', 'Add tests'), obs(6, 3, 'feeling', 'Tired'), obs(2, 1, 'note', 'Started')],
      next_before: null,
    })
    await render()
    expect(entryIds()).toEqual([3, 1])
    const first = target.querySelector('li[data-entry-id="3"]')!
    expect(first.querySelector('a')?.getAttribute('href')).toBe('/diary?entry=3')
    expect(first.querySelectorAll('li[data-observation-id]')).toHaveLength(2)
    expect(first.textContent).toContain('“Tired!”')
    const plan = [...first.querySelectorAll('span')].find((s) => s.textContent === 'Add tests')!
    expect(plan.className).toContain('bg-plan/15')
    const feeling = [...first.querySelectorAll('span')].find((s) => s.textContent === 'Tired')!
    expect(feeling.className).toContain('bg-accent/15')
    expect(showMore()).toBeUndefined()
  })

  it('shows more past the first page', async () => {
    routes.set('/api/projects/7/observations?limit=10', { observations: [obs(9, 4, 'note', 'Newest')], next_before: 4 })
    routes.set('/api/projects/7/observations?before=4&limit=10', { observations: [obs(1, 2, 'note', 'Older')], next_before: null })
    await render()
    expect(entryIds()).toEqual([4])
    showMore()!.click()
    await settle()
    expect(entryIds()).toEqual([4, 2])
    expect(showMore()).toBeUndefined()
  })

  it('reloads when an entry is processed', async () => {
    routes.set('/api/projects/7/observations?limit=10', { observations: [], next_before: null })
    await render()
    expect(target.textContent).toContain('Nothing yet')
    routes.set('/api/projects/7/observations?limit=10', { observations: [obs(1, 5, 'decision', 'Use Go')], next_before: null })
    FakeEventSource.latest().emit('entry.processed', { entry_id: 5, run_id: 1 })
    await settle()
    expect(target.textContent).toContain('Use Go')
  })

  it('says calmly when it cannot load', async () => {
    await render()
    expect(target.textContent).toContain("Something went wrong on Waypoint's side. Try again in a moment.")
    expect([...target.querySelectorAll('button')].some((b) => b.textContent?.includes('Try again'))).toBe(true)
  })
})
