import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Entry, EntryDetail, EntryPage } from './api/types.gen'

// A fake API: routes holds the answer (or a promise of one) for each GET path.
const routes = new Map<string, unknown>()

vi.mock('./api/client', async (importOriginal) => {
  const real = await importOriginal<typeof import('./api/client')>()
  return {
    ...real,
    getJSON: async (path: string) => {
      if (path === '/api/projects') return []
      if (!routes.has(path)) throw new real.ApiError(404, 'not_found', `no route ${path}`)
      return routes.get(path)
    },
  }
})

const { addEntry, loadOlder, loadTimeline, pageSize, timeline } = await import('./timeline.svelte')

function entry(id: number): Entry {
  return { id, body: `entry ${id}`, project_id: null, reply_to_question_id: null, created_at: '' }
}

function detail(id: number): EntryDetail {
  return { entry: entry(id), run: null, observations: [], questions: [], changesets: [] }
}

function page(ids: number[], nextBefore: number | null): EntryPage {
  for (const id of ids) routes.set(`/api/entries/${id}`, detail(id))
  return { entries: ids.map(entry), next_before: nextBefore }
}

const ids = () => timeline.items.map((i) => i.entry.id)
const first = `/api/entries?limit=${pageSize}`
const older = (before: number) => `/api/entries?limit=${pageSize}&before=${before}`

beforeEach(() => {
  routes.clear()
  timeline.items = []
  timeline.nextBefore = null
  timeline.status = 'idle'
  timeline.error = ''
})

describe('timeline', () => {
  it('loads the newest page with each entry detail', async () => {
    routes.set(first, page([3, 2], 2))
    await loadTimeline()
    expect(ids()).toEqual([3, 2])
    expect(timeline.nextBefore).toBe(2)
    expect(timeline.status).toBe('ready')
    expect(timeline.items.every((i) => i.detail !== null)).toBe(true)
  })

  it('appends older entries without duplicates', async () => {
    routes.set(first, page([5, 4], 4))
    await loadTimeline()
    routes.set(older(4), page([4, 3, 2], null))
    await loadOlder()
    expect(ids()).toEqual([5, 4, 3, 2])
    expect(timeline.nextBefore).toBeNull()
  })

  it('keeps loaded older entries when the newest page reloads', async () => {
    routes.set(first, page([5, 4], 4))
    await loadTimeline()
    routes.set(older(4), page([3, 2], 2))
    await loadOlder()
    routes.set(first, page([6, 5], 5))
    await loadTimeline()
    expect(ids()).toEqual([6, 5, 4, 3, 2])
    expect(timeline.nextBefore).toBe(2)
  })

  it('drops the answer of a load that a newer one replaced', async () => {
    let release: (p: EntryPage) => void = () => {}
    routes.set(first, new Promise<EntryPage>((resolve) => (release = resolve)))
    const stale = loadTimeline()
    routes.set(first, page([9], null))
    await loadTimeline()
    release(page([1], null))
    await stale
    expect(ids()).toEqual([9])
  })

  it('shows an error only when there is nothing to show', async () => {
    await loadTimeline()
    expect(timeline.status).toBe('error')
    routes.set(first, page([1], null))
    await loadTimeline()
    routes.delete(first)
    await loadTimeline()
    expect(timeline.status).toBe('ready')
    expect(ids()).toEqual([1])
    expect(timeline.error).not.toBe('')
  })

  it('puts a saved entry on top once', async () => {
    routes.set(first, page([1], null))
    await loadTimeline()
    routes.set('/api/entries/2', detail(2))
    await addEntry(entry(2))
    await addEntry(entry(2))
    expect(ids()).toEqual([2, 1])
    expect(timeline.items[0].detail).not.toBeNull()
  })
})
