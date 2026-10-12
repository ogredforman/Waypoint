// @vitest-environment jsdom
import { flushSync, mount, unmount } from 'svelte'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { Changeset, EntryDetail, ExtractionRun, Question } from '../lib/api/types.gen'
import { timeline } from '../lib/timeline.svelte'
import EntryCard from './EntryCard.svelte'

function run(status: ExtractionRun['status']): ExtractionRun {
  return { id: 1, entry_id: 1, status, error: null, failure: null, created_at: '', updated_at: '' }
}

const changeset: Changeset = {
  id: 3,
  source: 'extraction',
  entry_id: 1,
  parent_changeset_id: null,
  status: 'pending',
  ops: [1, 2].map((position) => ({
    id: position * 10,
    position,
    op_type: 'park_idea' as const,
    payload: { title: `Idea ${position}` },
    rationale: null,
    confidence: null,
    status: 'pending' as const,
    applied_payload: null,
    refs: {},
  })),
  created_at: '',
  resolved_at: null,
}
const question: Question = { id: 5, entry_id: 1, text: 'Which first?', reason: '', status: 'open', answer_entry_id: null, created_at: '' }

function detail(over: Partial<EntryDetail> = {}): EntryDetail {
  return {
    entry: { id: 1, body: 'idea: Idea 1', project_id: null, reply_to_question_id: null, created_at: '' },
    run: run('succeeded'),
    observations: [],
    questions: [],
    changesets: [],
    ...over,
  }
}

let target: HTMLElement
let component: ReturnType<typeof mount>
// reprocessStatus is how the server answers POST /reprocess. Afterwards the
// entry reads as running, with its proposals and questions gone.
let reprocessStatus = 202
let afterReprocess: EntryDetail
// deleteStatus is how the server answers DELETE /entries/{id}.
let deleteStatus = 204
const fetchMock = vi.fn(async (_url: string, init?: RequestInit) => {
  if (init?.method === 'DELETE') {
    if (deleteStatus === 409) {
      return new Response(JSON.stringify({ error: { code: 'run_in_progress', message: 'run in progress' } }), { status: 409 })
    }
    return new Response(null, { status: 204 })
  }
  if (init?.method === 'POST') {
    if (reprocessStatus === 409) {
      return new Response(JSON.stringify({ error: { code: 'conflict', message: 'run in progress' } }), { status: 409 })
    }
    return new Response(JSON.stringify(run('pending')), { status: 202 })
  }
  return new Response(JSON.stringify(afterReprocess), { status: 200 })
})

function show(d: EntryDetail) {
  reprocessStatus = 202
  deleteStatus = 204
  afterReprocess = { ...d, run: run('running'), questions: [], changesets: [] }
  fetchMock.mockClear()
  vi.stubGlobal('fetch', fetchMock)
  timeline.items = [{ entry: d.entry, detail: d, error: '' }]
  target = document.createElement('div')
  document.body.append(target)
  component = mount(EntryCard, { target, props: { item: timeline.items[0], projectTitle: null, replyTo: null, projects: [] } })
  flushSync()
}

afterEach(() => {
  unmount(component)
  target.remove()
  timeline.items = []
  vi.unstubAllGlobals()
})

const buttons = (text: string) => [...target.querySelectorAll('button')].filter((b) => b.textContent?.trim() === text)
function button(text: string): HTMLButtonElement {
  const b = buttons(text).at(-1)
  if (!b) throw new Error(`no ${text} button`)
  return b
}
function click(el: HTMLElement) {
  el.click()
  flushSync()
}
const posts = () => fetchMock.mock.calls.filter(([, init]) => init?.method === 'POST').map(([u]) => u)
const reading = () => target.textContent!.includes('Waypoint is reading this')

describe('EntryCard reading again', () => {
  it('reads a failed entry again straight away when nothing would be replaced', async () => {
    show(detail({ run: run('failed') }))
    click(button('Read again'))
    await vi.waitFor(() => expect(reading()).toBe(true))
    expect(posts()).toEqual(['/api/entries/1/reprocess'])
    expect(buttons('Read again')).toHaveLength(0)
    expect(buttons('More')).toHaveLength(0)
  })

  it('says why the read failed, next to Read again', () => {
    show(detail({ run: { ...run('failed'), error: 'timeout: x', failure: 'timeout' } }))
    expect(target.textContent).toContain('the model took too long to answer')
    expect(buttons('Read again')).toHaveLength(1)
  })

  it('keeps the action behind More on a finished entry', () => {
    show(detail())
    expect(buttons('Read again')).toHaveLength(0)
    click(button('More'))
    expect(button('More').getAttribute('aria-expanded')).toBe('true')
    expect(buttons('Read again')).toHaveLength(1)
  })

  it('says what would be replaced first, and Keep these sends nothing', async () => {
    show(detail({ changesets: [changeset], questions: [question] }))
    click(button('More'))
    click(button('Read again'))
    const group = target.querySelector('[aria-label="Confirm reading again"]')!
    expect(group.textContent).toContain('2 proposed changes and 1 open question')
    click(button('Keep these'))
    expect(target.querySelector('[aria-label="Confirm reading again"]')).toBeNull()
    expect(posts()).toEqual([])

    click(button('More'))
    click(button('Read again'))
    click(button('Read again'))
    await vi.waitFor(() => expect(reading()).toBe(true))
    expect(posts()).toEqual(['/api/entries/1/reprocess'])
    await vi.waitFor(() => expect(target.textContent).not.toContain('Which first?'))
  })

  it('says so calmly when a run is already under way', async () => {
    show(detail({ run: run('failed') }))
    reprocessStatus = 409
    click(button('Read again'))
    await vi.waitFor(() => expect(target.querySelector('[role="status"]')?.textContent).toBe('Waypoint is already reading this entry.'))
    await vi.waitFor(() => expect(reading()).toBe(true))
  })

  it('keeps the warning current, and ends it when a run starts elsewhere', () => {
    show(detail({ changesets: [changeset], questions: [question] }))
    click(button('More'))
    click(button('Read again'))
    timeline.items[0].detail!.questions = []
    flushSync()
    expect(target.querySelector('[aria-label="Confirm reading again"]')!.textContent).toContain(': 2 proposed changes.')
    timeline.items[0].detail!.run = run('running')
    flushSync()
    timeline.items[0].detail!.run = run('succeeded')
    flushSync()
    expect(target.querySelector('[aria-label="Confirm reading again"]')).toBeNull()
  })

  it("isn't offered while Waypoint is reading", () => {
    show(detail({ run: run('pending') }))
    expect(buttons('Read again')).toHaveLength(0)
    expect(buttons('More')).toHaveLength(0)
  })
})

describe('EntryCard deleting', () => {
  const deletes = () => fetchMock.mock.calls.filter(([, init]) => init?.method === 'DELETE').map(([u]) => u)
  const confirmGroup = () => target.querySelector('[aria-label="Confirm deleting"]')

  it('says what goes first, and Keep it sends nothing', () => {
    show(detail({ changesets: [changeset] }))
    click(button('More'))
    click(button('Delete entry'))
    expect(confirmGroup()!.textContent).toContain('changes you already')
    expect(buttons('More')).toHaveLength(0)
    click(button('Keep it'))
    expect(confirmGroup()).toBeNull()
    expect(deletes()).toEqual([])
  })

  it('deletes the entry and takes it out of the timeline', async () => {
    show(detail())
    click(button('More'))
    click(button('Delete entry'))
    click(button('Delete'))
    await vi.waitFor(() => expect(timeline.items).toHaveLength(0))
    expect(deletes()).toEqual(['/api/entries/1'])
  })

  it('is offered on a failed entry too, but not while Waypoint is reading', () => {
    show(detail({ run: run('failed') }))
    click(button('More'))
    expect(buttons('Delete entry')).toHaveLength(1)
    expect(buttons('Read again')).toHaveLength(1)
    timeline.items[0].detail!.run = run('running')
    flushSync()
    expect(buttons('Delete entry')).toHaveLength(0)
  })

  it('says so calmly when a run started meanwhile', async () => {
    show(detail())
    deleteStatus = 409
    click(button('More'))
    click(button('Delete entry'))
    click(button('Delete'))
    await vi.waitFor(() => expect(target.querySelector('[role="status"]')?.textContent).toContain("once that's done"))
    expect(timeline.items).toHaveLength(1)
  })
})
