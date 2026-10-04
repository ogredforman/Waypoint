// @vitest-environment jsdom
import { flushSync, mount, unmount } from 'svelte'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ChangeOp, Changeset, Entry, Project } from '../lib/api/types.gen'
import ChangesetReview from './ChangesetReview.svelte'

function op(position: number, op_type: ChangeOp['op_type'], payload: ChangeOp['payload']): ChangeOp {
  return {
    id: position * 10,
    position,
    op_type,
    payload,
    rationale: 'from a line; project guessed from the entry',
    confidence: 0.5,
    status: 'pending',
    applied_payload: null,
    refs: { 'project:7': 'Waypoint' },
  }
}

const changeset: Changeset = {
  id: 3,
  source: 'extraction',
  entry_id: 1,
  parent_changeset_id: null,
  status: 'pending',
  ops: [
    op(1, 'create_task', { project_id: 7, title: 'Write docs' }),
    op(2, 'set_task_status', { task_id: '$new:1', status: 'done' }),
    op(3, 'park_idea', { project_id: 7, title: 'Dark mode' }),
  ],
  created_at: '',
  resolved_at: null,
}
const entry: Entry = { id: 1, body: 'done: Write docs\nidea: Dark mode', project_id: null, reply_to_question_id: null, created_at: '' }
const project = (id: number, title: string): Project => ({
  id,
  title,
  why: '',
  definition_of_done: '',
  status: 'active',
  created_at: '',
  updated_at: '',
  last_activity_at: '',
})

let target: HTMLElement
let component: ReturnType<typeof mount>
const fetchMock = vi.fn(async (url: string, _init?: RequestInit) => {
  const body = url.endsWith('/apply')
    ? { changeset, split_off: null, held_by_cap: [] }
    : { entry, run: null, observations: [], questions: [], changesets: [] }
  return new Response(JSON.stringify(body), { status: 200 })
})

beforeEach(() => {
  fetchMock.mockClear()
  vi.stubGlobal('fetch', fetchMock)
  target = document.createElement('div')
  document.body.append(target)
  component = mount(ChangesetReview, {
    target,
    props: { changeset, entry, projects: [project(7, 'Waypoint'), project(9, 'Garden')], onnotice: () => {} },
  })
  flushSync()
})

afterEach(() => {
  unmount(component)
  target.remove()
  vi.unstubAllGlobals()
})

const rows = () => [...target.querySelectorAll('li')]
const sentence = (row: Element) => row.querySelector('p')!.textContent
function button(root: Element, text: string): HTMLButtonElement {
  const b = [...root.querySelectorAll('button')].find((b) => b.textContent?.trim() === text)
  if (!b) throw new Error(`no ${text} button`)
  return b
}
function click(el: HTMLElement) {
  el.click()
  flushSync()
}
function pick(value: string) {
  const select = target.querySelector('select')!
  select.value = value
  select.dispatchEvent(new Event('change', { bubbles: true }))
  flushSync()
}
async function sentBody() {
  await vi.waitFor(() => expect(fetchMock.mock.calls.some(([u]) => u === '/api/changesets/3/apply')).toBe(true))
  const [, init] = fetchMock.mock.calls.find(([u]) => u === '/api/changesets/3/apply')!
  return JSON.parse(init!.body as string)
}

describe('ChangesetReview', () => {
  it('shows each op as a sentence, under the guessed project', () => {
    expect(rows().map(sentence)).toEqual(['New task: Write docs', 'Mark “Write docs” done', 'Park idea: Dark mode'])
    expect(target.querySelector('select')!.value).toBe('7')
    expect(target.textContent).toContain('(guessed)')
    expect(button(target, 'Apply').disabled).toBe(true)
  })

  it('rejecting an op rejects what uses it, and Apply sends only decided ops', async () => {
    click(button(rows()[0], 'Reject'))
    expect(rows().map((r) => r.textContent!.includes('Rejected'))).toEqual([true, true, false])
    click(button(target, 'Apply'))
    expect(await sentBody()).toEqual({
      ops: [
        { id: 10, decision: 'reject' },
        { id: 20, decision: 'reject' },
      ],
    })
  })

  it("can't accept an op that uses a rejected one", () => {
    click(button(rows()[0], 'Reject'))
    click(button(rows()[1], 'Undo'))
    expect(button(rows()[1], 'Accept').disabled).toBe(true)
    expect(rows()[1].textContent).toContain('which you rejected')
  })

  it("with no project, tasks and blockers can't be accepted", () => {
    pick('')
    click(button(rows()[0], 'Undo'))
    expect(button(rows()[0], 'Accept').disabled).toBe(true)
    expect(rows()[0].textContent).toContain('need a project')
  })

  it('accept all with no project rejects the task and unties the idea', async () => {
    pick('')
    click(button(target, 'Accept all'))
    expect(await sentBody()).toEqual({
      ops: [
        { id: 10, decision: 'reject' },
        { id: 20, decision: 'reject' },
        { id: 30, decision: 'edit', payload: { title: 'Dark mode' } },
      ],
    })
  })

  it('edits an op and moves it to the chosen project', async () => {
    pick('9')
    click(button(rows()[0], 'Edit'))
    const input = rows()[0].querySelector('input')!
    input.value = 'Write the docs'
    input.dispatchEvent(new Event('input'))
    flushSync()
    click(button(rows()[0], 'Save and accept'))
    expect(sentence(rows()[1])).toBe('Mark “Write the docs” done')
    click(button(target, 'Apply'))
    expect(await sentBody()).toEqual({ ops: [{ id: 10, decision: 'edit', payload: { project_id: 9, title: 'Write the docs' } }] })
  })
})
