import { ApiError, getJSON, postJSON } from './api/client'
import type { Entry, EntryDetail, EntryPage, ExtractionRun, Project } from './api/types.gen'
import { on, onReconnect } from './events'

// The diary timeline, shared by every copy of the diary (panel, overlay, and
// /diary) so it survives them closing (ADR 0004). Entries load a page at a
// time, newest first; each entry's detail (observations, questions,
// changesets) loads on its own, so one that fails doesn't hide the others.
// followEvents keeps it current: an entry reloads when its run finishes, so
// "Waypoint is reading this…" turns into the result without a page reload.

export interface TimelineItem {
  entry: Entry
  detail: EntryDetail | null
  error: string
}

export const pageSize = 10

export const timeline = $state<{
  items: TimelineItem[]
  projects: Project[]
  nextBefore: number | null
  status: 'idle' | 'loading' | 'ready' | 'error'
  error: string
  loadingOlder: boolean
}>({ items: [], projects: [], nextBefore: null, status: 'idle', error: '', loadingOlder: false })

// Bumped by each loadTimeline, so answers meant for a replaced list are dropped.
let generation = 0
// The latest loadEntry request per entry, so a slow answer can't overwrite a
// newer one (e.g. "still reading" arriving after "done").
const entryLoads = new Map<number, number>()
let entryLoadSeq = 0

function message(err: unknown, fallback: string): string {
  return err instanceof ApiError ? err.message : fallback
}

// loadTimeline (re)loads the newest page. Entries already shown keep their
// detail until the fresh one arrives, so reopening the diary doesn't flicker,
// and older entries already loaded stay in the list below the fresh page.
export async function loadTimeline(): Promise<void> {
  const gen = ++generation
  if (timeline.status !== 'ready') timeline.status = 'loading'
  getJSON<Project[]>('/api/projects')
    .then((ps) => {
      if (gen === generation) timeline.projects = ps
    })
    .catch(() => {}) // only used for project names
  let page: EntryPage
  try {
    page = await getJSON<EntryPage>(`/api/entries?limit=${pageSize}`)
  } catch (err) {
    if (gen !== generation) return
    timeline.error = message(err, "Couldn't load your entries.")
    if (timeline.items.length === 0) timeline.status = 'error'
    return
  }
  if (gen !== generation) return
  const known = new Map(timeline.items.map((i) => [i.entry.id, i.detail]))
  const oldest = page.entries.at(-1)?.id
  const older = oldest === undefined ? [] : timeline.items.filter((i) => i.entry.id < oldest)
  timeline.items = [...page.entries.map((entry) => ({ entry, detail: known.get(entry.id) ?? null, error: '' })), ...older]
  if (older.length === 0) timeline.nextBefore = page.next_before
  timeline.status = 'ready'
  timeline.error = ''
  await Promise.all(page.entries.map((e) => loadEntry(e.id)))
}

// resetTimeline empties the timeline, e.g. when the profile changes, and drops
// any answer still on its way for the old one.
export function resetTimeline(): void {
  generation++
  entryLoads.clear()
  timeline.items = []
  timeline.projects = []
  timeline.nextBefore = null
  timeline.status = 'idle'
  timeline.error = ''
  timeline.loadingOlder = false
}

// loadOlder appends the next page of older entries.
export async function loadOlder(): Promise<void> {
  const before = timeline.nextBefore
  if (before === null || timeline.loadingOlder) return
  const gen = generation
  timeline.loadingOlder = true
  timeline.error = ''
  try {
    const page = await getJSON<EntryPage>(`/api/entries?limit=${pageSize}&before=${before}`)
    if (gen !== generation) return
    const fresh = page.entries.filter((e) => !timeline.items.some((i) => i.entry.id === e.id))
    timeline.items.push(...fresh.map((entry) => ({ entry, detail: null, error: '' })))
    timeline.nextBefore = page.next_before
    await Promise.all(fresh.map((e) => loadEntry(e.id)))
  } catch (err) {
    if (gen === generation) timeline.error = message(err, "Couldn't load older entries.")
  } finally {
    if (gen === generation) timeline.loadingOlder = false
  }
}

// loadEntry (re)loads one entry's detail, if the entry is in the timeline.
export async function loadEntry(id: number): Promise<void> {
  const seq = ++entryLoadSeq
  entryLoads.set(id, seq)
  try {
    const detail = await getJSON<EntryDetail>(`/api/entries/${id}`)
    if (entryLoads.get(id) !== seq) return
    const item = timeline.items.find((i) => i.entry.id === id)
    if (item) {
      item.detail = detail
      item.error = ''
    }
  } catch (err) {
    if (entryLoads.get(id) !== seq) return
    const item = timeline.items.find((i) => i.entry.id === id)
    if (item) item.error = message(err, "Couldn't load this entry's details.")
  }
}

// addEntry puts a just-saved entry at the top of the timeline. With the run
// the save queued, it shows as being read straight away, before its detail
// loads.
export async function addEntry(entry: Entry, run?: ExtractionRun): Promise<void> {
  if (timeline.items.some((i) => i.entry.id === entry.id)) return
  const detail = run ? { entry, run, observations: [], questions: [], changesets: [] } : null
  timeline.items.unshift({ entry, detail, error: '' })
  await loadEntry(entry.id)
}

// reprocessEntry asks Waypoint to read an entry again. The server rejects the
// entry's pending changesets and dismisses its open questions; the entry shows
// as being read straight away, then reloads so those go too. Errors (e.g. a
// 409 while a run is under way) are the caller's to show.
export async function reprocessEntry(id: number): Promise<void> {
  const run = await postJSON<ExtractionRun>(`/api/entries/${id}/reprocess`, undefined)
  const item = timeline.items.find((i) => i.entry.id === id)
  if (item?.detail) item.detail.run = run
  await loadEntry(id)
}

// entryChanged reloads an entry the server says changed. An entry newer than
// any shown (e.g. one written in another tab) reloads the newest page instead.
function entryChanged(id: number) {
  if (timeline.status === 'idle') return
  if (timeline.items.some((i) => i.entry.id === id)) void loadEntry(id)
  else if (id > (timeline.items[0]?.entry.id ?? 0)) void loadTimeline()
}

// followEvents keeps the timeline current from server events until the
// returned function is called. Nothing loads until a diary first opens.
export function followEvents(): () => void {
  const offs = [
    on('entry.processed', (e) => entryChanged(e.entry_id)),
    on('changeset.ready', (e) => entryChanged(e.entry_id)),
    on('run.failed', (e) => entryChanged(e.entry_id)),
    // A changeset applied elsewhere (e.g. another tab) is no longer pending here.
    on('changeset.applied', (e) => {
      for (const i of timeline.items) {
        if (i.detail?.changesets.some((c) => c.id === e.changeset_id)) void loadEntry(i.entry.id)
      }
    }),
    onReconnect(() => {
      if (timeline.status !== 'idle') void loadTimeline()
    }),
  ]
  return () => offs.forEach((off) => off())
}
