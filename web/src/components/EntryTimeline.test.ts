// @vitest-environment jsdom
import { flushSync, mount, unmount } from 'svelte'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Entry, EntryDetail, EntryPage } from '../lib/api/types.gen'
import { resetTimeline } from '../lib/timeline.svelte'
import EntryTimeline from './EntryTimeline.svelte'

function entry(id: number): Entry {
  return { id, body: `entry ${id}`, project_id: null, reply_to_question_id: null, created_at: '2026-10-01T09:00:00Z' }
}

function detail(id: number): EntryDetail {
  return { entry: entry(id), run: null, observations: [], questions: [], changesets: [] }
}

// Entries 1–20, newest first, ten to a page.
function page(before: number): EntryPage {
  const ids = Array.from({ length: 10 }, (_, i) => before - 1 - i).filter((id) => id >= 1)
  const last = ids.at(-1)!
  return { entries: ids.map(entry), next_before: last > 1 ? last : null }
}

const requested: string[] = []
// failOlder makes every older page fail.
let failOlder = false
const fetchMock = vi.fn(async (url: string) => {
  const u = new URL(url, 'http://localhost')
  requested.push(u.pathname + u.search)
  if (failOlder && u.searchParams.has('before')) {
    return new Response(JSON.stringify({ error: { code: 'internal', message: 'Something went wrong.' } }), { status: 500 })
  }
  let body: unknown
  if (u.pathname.endsWith('/projects')) body = []
  else if (u.pathname.endsWith('/entries')) body = page(Number(u.searchParams.get('before') ?? 21))
  else body = detail(Number(u.pathname.split('/').at(-1)))
  return new Response(JSON.stringify(body), { status: 200 })
})

let target: HTMLElement
let component: ReturnType<typeof mount> | undefined
const scrolled: Element[] = []

beforeEach(() => {
  vi.stubGlobal('fetch', fetchMock)
  Element.prototype.scrollIntoView = function (this: Element) {
    scrolled.push(this)
  }
  target = document.createElement('div')
  document.body.append(target)
})

afterEach(() => {
  if (component) unmount(component)
  component = undefined
  target.remove()
  resetTimeline()
  requested.length = 0
  scrolled.length = 0
  failOlder = false
  vi.unstubAllGlobals()
})

function show(focusEntry: number | null) {
  component = mount(EntryTimeline, { target, props: { focusEntry } })
  flushSync()
}

const card = (id: number) => target.querySelector(`li[data-entry-id="${id}"]`)
const highlightedIn = (id: number) => card(id)?.querySelector('.ring-2') ?? null

describe('EntryTimeline', () => {
  it('loads back as far as a linked entry, scrolls to it, and highlights it', async () => {
    show(5)
    await vi.waitFor(() => expect(highlightedIn(5)).not.toBeNull())
    expect(scrolled).toEqual([card(5)])
    expect(highlightedIn(6)).toBeNull()
    expect(requested.filter((r) => r.endsWith('/entries?limit=10&before=11'))).toHaveLength(1)
  })

  it('says so calmly when the linked entry is not there, and shows the rest', async () => {
    // 25 is newer than any entry, so the first page already rules it out.
    show(25)
    await vi.waitFor(() => expect(target.textContent).toContain("That entry isn't in your diary."))
    expect(card(20)).not.toBeNull()
    expect(requested.some((r) => r.includes('before='))).toBe(false)
    expect(scrolled).toEqual([])
  })

  it("shows the error, not \"isn't in your diary\", when an older page fails", async () => {
    failOlder = true
    show(5)
    await vi.waitFor(() => expect(target.textContent).toContain("Something went wrong on Waypoint's side. Try again in a moment."))
    expect(target.textContent).not.toContain("That entry isn't in your diary.")
    expect(card(11)).not.toBeNull()
  })

  it('loads only the newest page without a linked entry', async () => {
    show(null)
    await vi.waitFor(() => expect(card(11)).not.toBeNull())
    expect(card(10)).toBeNull()
    expect(target.querySelector('.ring-2')).toBeNull()
  })
})
