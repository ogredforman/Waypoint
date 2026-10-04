import { afterAll, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Changeset, Entry, EntryDetail, EntryPage, ExtractionRun, Question } from './api/types.gen'
import { startEvents } from './events'
import { FakeEventSource, fakeOpen } from './events.fake'

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
    deleteJSON: async (path: string) => {
      if (!routes.has(`DELETE ${path}`)) throw new real.ApiError(409, 'run_in_progress', 'busy')
    },
    postJSON: async (path: string) => {
      if (!routes.has(`POST ${path}`)) throw new real.ApiError(404, 'not_found', `no route ${path}`)
      return routes.get(`POST ${path}`)
    },
  }
})

const { addEntry, deleteEntry, followEvents, loadEntry, loadOlder, loadThrough, loadTimeline, pageSize, reprocessEntry, timeline } = await import('./timeline.svelte')

const stopFollowing = followEvents()
const stopEvents = startEvents(fakeOpen)
afterAll(() => {
  stopEvents()
  stopFollowing()
})

// settle lets the loads an event started finish.
const settle = () => new Promise((resolve) => setTimeout(resolve, 0))

function run(status: ExtractionRun['status']): ExtractionRun {
  return { id: 1, entry_id: 1, status, error: null, created_at: '', updated_at: '' }
}

// changeset is a stand-in with only the fields the timeline reads.
function changeset(id: number, status: Changeset['status']): Changeset {
  return { id, status } as Changeset
}

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
  it('loads older pages until the linked entry is in the timeline', async () => {
    routes.set(first, page([6, 5], 5))
    routes.set(older(5), page([4, 3], 3))
    routes.set(older(3), page([2, 1], null))
    expect(await loadThrough(4)).toBe('found')
    expect(ids()).toEqual([6, 5, 4, 3])
  })

  it('stops paging once older pages cannot hold the linked entry', async () => {
    routes.set(first, page([6, 4], 4))
    expect(await loadThrough(5)).toBe('absent')
    expect(ids()).toEqual([6, 4])
  })

  it('stops paging when an older page fails', async () => {
    routes.set(first, page([6, 5], 5))
    expect(await loadThrough(2)).toBe('failed')
    expect(ids()).toEqual([6, 5])
  })

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

  it('shows a saved entry as being read before its detail loads', async () => {
    let release: (d: EntryDetail) => void = () => {}
    routes.set('/api/entries/2', new Promise<EntryDetail>((resolve) => (release = resolve)))
    const adding = addEntry(entry(2), run('pending'))
    expect(timeline.items[0].detail?.run?.status).toBe('pending')
    release({ ...detail(2), run: run('succeeded') })
    await adding
    expect(timeline.items[0].detail?.run?.status).toBe('succeeded')
  })

  it("drops an entry's older detail answer once a newer one arrived", async () => {
    routes.set(first, page([1], null))
    await loadTimeline()
    let release: (d: EntryDetail) => void = () => {}
    routes.set('/api/entries/1', new Promise<EntryDetail>((resolve) => (release = resolve)))
    const stale = loadEntry(1)
    routes.set('/api/entries/1', { ...detail(1), run: run('succeeded') })
    await loadEntry(1)
    release({ ...detail(1), run: run('running') })
    await stale
    expect(timeline.items[0].detail?.run?.status).toBe('succeeded')
  })

  it('shows a reprocessed entry as being read, then reloads it', async () => {
    routes.set(first, page([1], null))
    routes.set('/api/entries/1', { ...detail(1), run: run('failed') })
    await loadTimeline()
    let release: (d: EntryDetail) => void = () => {}
    routes.set('POST /api/entries/1/reprocess', run('pending'))
    routes.set('/api/entries/1', new Promise<EntryDetail>((resolve) => (release = resolve)))
    const reprocessing = reprocessEntry(1)
    await settle()
    expect(timeline.items[0].detail?.run?.status).toBe('pending')
    release({ ...detail(1), run: run('running') })
    await reprocessing
    expect(timeline.items[0].detail?.run?.status).toBe('running')
  })

  it('reloads an entry when its run finishes', async () => {
    routes.set(first, page([1], null))
    await loadTimeline()
    routes.set('/api/entries/1', { ...detail(1), run: run('failed') })
    FakeEventSource.latest().emit('run.failed', { entry_id: 1, run_id: 1 })
    await settle()
    expect(timeline.items[0].detail?.run?.status).toBe('failed')
  })

  it('reloads the newest page for an entry it has not seen', async () => {
    routes.set(first, page([1], null))
    await loadTimeline()
    routes.set(first, page([2, 1], null))
    FakeEventSource.latest().emit('entry.processed', { entry_id: 2, run_id: 2 })
    await settle()
    expect(ids()).toEqual([2, 1])
  })

  it('ignores events until a diary has opened', async () => {
    routes.set(first, page([1], null))
    FakeEventSource.latest().emit('entry.processed', { entry_id: 1, run_id: 1 })
    await settle()
    expect(timeline.status).toBe('idle')
    expect(ids()).toEqual([])
  })

  it('reloads entries whose pending changeset was applied elsewhere', async () => {
    routes.set(first, page([2, 1], null))
    routes.set('/api/entries/1', { ...detail(1), changesets: [changeset(7, 'pending')] })
    await loadTimeline()
    routes.set('/api/entries/1', { ...detail(1), changesets: [changeset(7, 'applied')] })
    routes.delete('/api/entries/2') // reloading entry 2 would set its error
    FakeEventSource.latest().emit('changeset.applied', { changeset_id: 7, project_ids: [] })
    await settle()
    expect(timeline.items[1].detail?.changesets[0].status).toBe('applied')
    expect(timeline.items[0].error).toBe('')
  })

  it('takes a deleted entry out, and reloads the entries tied to it by a question', async () => {
    const question = (id: number, entryId: number, status: Question['status']) =>
      ({ id, entry_id: entryId, status }) as Question
    routes.set(first, page([3, 2, 1], null))
    routes.set('/api/entries/3', { ...detail(3), entry: { ...entry(3), reply_to_question_id: 9 } })
    routes.set('/api/entries/2', { ...detail(2), entry: { ...entry(2), reply_to_question_id: 5 }, questions: [question(9, 2, 'answered')] })
    routes.set('/api/entries/1', { ...detail(1), questions: [question(5, 1, 'answered')] })
    await loadTimeline()
    timeline.items[0].entry = { ...entry(3), reply_to_question_id: 9 }
    timeline.items[1].entry = { ...entry(2), reply_to_question_id: 5 }

    routes.set('DELETE /api/entries/2', null)
    routes.set('/api/entries/1', { ...detail(1), questions: [question(5, 1, 'dismissed')] })
    await deleteEntry(2)
    await settle()
    expect(ids()).toEqual([3, 1])
    expect(timeline.items[0].entry.reply_to_question_id).toBeNull()
    expect(timeline.items[1].detail?.questions[0].status).toBe('dismissed')
  })

  it('keeps an entry the server would not delete, and leaves the error to the caller', async () => {
    routes.set(first, page([1], null))
    await loadTimeline()
    await expect(deleteEntry(1)).rejects.toThrow()
    expect(ids()).toEqual([1])
  })

  it('takes out an entry deleted in another tab', async () => {
    routes.set(first, page([2, 1], null))
    await loadTimeline()
    FakeEventSource.latest().emit('entry.deleted', { entry_id: 2 })
    expect(ids()).toEqual([1])
  })

  it('reloads after reconnecting', async () => {
    routes.set(first, page([1], null))
    await loadTimeline()
    const src = FakeEventSource.latest()
    src.open()
    routes.set(first, page([2, 1], null))
    src.fail()
    src.open()
    await settle()
    expect(ids()).toEqual([2, 1])
  })
})
