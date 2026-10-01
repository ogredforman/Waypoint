import { ApiError, getJSON } from './api/client'
import type { Entry, EntryDetail, EntryPage, Project } from './api/types.gen'

// The diary timeline, shared by every copy of the diary (panel, overlay, and
// /diary) so it survives them closing (ADR 0004). Entries load a page at a
// time, newest first; each entry's detail (observations, questions,
// changesets) loads on its own, so one that fails doesn't hide the others.
// Live updates arrive in 1.22; until then the timeline reloads when a diary
// opens and after the user acts on it.

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
    timeline.loadingOlder = false
  }
}

// loadEntry (re)loads one entry's detail, if the entry is in the timeline.
export async function loadEntry(id: number): Promise<void> {
  try {
    const detail = await getJSON<EntryDetail>(`/api/entries/${id}`)
    const item = timeline.items.find((i) => i.entry.id === id)
    if (item) {
      item.detail = detail
      item.error = ''
    }
  } catch (err) {
    const item = timeline.items.find((i) => i.entry.id === id)
    if (item) item.error = message(err, "Couldn't load this entry's details.")
  }
}

// addEntry puts a just-saved entry at the top of the timeline.
export async function addEntry(entry: Entry): Promise<void> {
  if (timeline.items.some((i) => i.entry.id === entry.id)) return
  timeline.items.unshift({ entry, detail: null, error: '' })
  await loadEntry(entry.id)
}
