import { describe, expect, it } from 'vitest'
import type { Entry, EntryDetail, Observation, ObservationLink } from './api/types.gen'
import {
  canReprocess,
  highlightClass,
  kindLabel,
  observationHref,
  pendingChangesets,
  repliedQuestion,
  reprocessImpact,
  reprocessWarning,
  runState,
  segments,
  shownQuestions,
} from './entries'

function obs(id: number, start: number, end: number, links: ObservationLink[] = []): Observation {
  return { id, kind: 'note', summary: `obs ${id}`, span_start: start, span_end: end, links }
}

function link(target_type: ObservationLink['target_type'], target_id: number): ObservationLink {
  return { target_type, target_id, confidence: 1 }
}

function entry(id: number, reply: number | null = null): Entry {
  return { id, body: 'x', project_id: null, reply_to_question_id: reply, created_at: '2026-09-30T10:00:00Z' }
}

function detail(over: Partial<EntryDetail> = {}): EntryDetail {
  return { entry: entry(1), run: null, observations: [], questions: [], changesets: [], ...over }
}

const texts = (body: string, os: Observation[]) => segments(body, os).map((s) => [s.text, s.observation?.id ?? null])

describe('segments', () => {
  it('returns the whole body when nothing is highlighted', () => {
    expect(texts('hello', [])).toEqual([['hello', null]])
  })

  it('splits around highlights in span order', () => {
    const body = 'done: a\ntodo: b\nplain'
    expect(texts(body, [obs(2, 8, 15), obs(1, 0, 7)])).toEqual([
      ['done: a', 1],
      ['\n', null],
      ['todo: b', 2],
      ['\nplain', null],
    ])
  })

  it('starts an overlapping span where the earlier one ends, and drops one inside it', () => {
    expect(texts('abcdefgh', [obs(1, 0, 4), obs(2, 2, 6), obs(3, 1, 3)])).toEqual([
      ['abcd', 1],
      ['ef', 2],
      ['gh', null],
    ])
  })

  it('skips empty and out-of-range spans', () => {
    expect(texts('abc', [obs(1, 1, 1), obs(2, 2, 9), obs(3, -1, 2)])).toEqual([['abc', null]])
  })

  it('slices by UTF-16 offsets', () => {
    const body = 'é 🌱 ok'
    expect(texts(body, [obs(1, 5, 7)])).toEqual([
      ['é 🌱 ', null],
      ['ok', 1],
    ])
  })
})

describe('highlightClass', () => {
  it('gives plans their own tint and every other kind the accent', () => {
    expect(highlightClass('plan', false)).toBe('bg-plan/15')
    expect(highlightClass('plan', true)).toContain('decoration-plan/50')
    expect(highlightClass('note', false)).toBe('bg-accent/15')
    expect(highlightClass('progress', true)).toContain('hover:bg-accent/25')
    expect(kindLabel('plan')).toBe('Plan')
  })
})

describe('observationHref', () => {
  it('prefers the task, then the project, then the parking lot', () => {
    expect(observationHref(obs(1, 0, 1, [link('project', 3), link('task', 7)]))).toBe('/tasks/7')
    expect(observationHref(obs(1, 0, 1, [link('blocker', 4), link('project', 3)]))).toBe('/projects/3')
    expect(observationHref(obs(1, 0, 1, [link('idea', 5)]))).toBe('/parking-lot')
  })

  it('is null without a link that has a page', () => {
    expect(observationHref(obs(1, 0, 1))).toBeNull()
    expect(observationHref(obs(1, 0, 1, [link('blocker', 4)]))).toBeNull()
  })
})

describe('entry detail helpers', () => {
  const q = (id: number, status: 'open' | 'answered' | 'dismissed') => ({
    id,
    entry_id: 1,
    text: `q${id}`,
    reason: '',
    status,
    answer_entry_id: null,
    created_at: '',
  })

  it('shows open and answered questions, not dismissed ones', () => {
    expect(shownQuestions(detail({ questions: [q(1, 'open'), q(2, 'answered'), q(3, 'dismissed')] })).map((x) => x.id)).toEqual([1, 2])
  })

  it('lists pending changesets with ops left to decide', () => {
    const op = (status: 'pending' | 'applied' | 'rejected') => ({
      id: 1,
      position: 1,
      op_type: 'create_task' as const,
      payload: {},
      rationale: null,
      confidence: null,
      status,
      applied_payload: null,
      refs: {},
    })
    const cs = (id: number, status: 'pending' | 'applied', ops: ReturnType<typeof op>[]) => ({
      id,
      source: 'extraction' as const,
      entry_id: 1,
      parent_changeset_id: null,
      status,
      ops,
      created_at: '',
      resolved_at: null,
    })
    const d = detail({
      changesets: [cs(1, 'pending', [op('pending'), op('pending')]), cs(2, 'applied', [op('applied')]), cs(3, 'pending', [op('rejected')])],
    })
    expect(pendingChangesets(d).map((c) => c.id)).toEqual([1])
  })

  it('reads the run state', () => {
    const run = (status: 'pending' | 'running' | 'succeeded' | 'failed') => ({
      id: 1,
      entry_id: 1,
      status,
      error: null,
      created_at: '',
      updated_at: '',
    })
    expect(runState(detail({ run: run('pending') }))).toBe('reading')
    expect(runState(detail({ run: run('running') }))).toBe('reading')
    expect(runState(detail({ run: run('failed') }))).toBe('failed')
    expect(runState(detail({ run: run('succeeded') }))).toBe('done')
    expect(runState(detail())).toBe('done')
  })

  it('finds the question a reply answers among loaded details', () => {
    const details = [null, detail({ questions: [q(4, 'answered')] })]
    expect(repliedQuestion(entry(2, 4), details)?.text).toBe('q4')
    expect(repliedQuestion(entry(2, 9), details)).toBeNull()
    expect(repliedQuestion(entry(2), details)).toBeNull()
  })
})

describe('reading again', () => {
  const op = (status: 'pending' | 'applied' | 'rejected') => ({
    id: 1,
    position: 1,
    op_type: 'create_task' as const,
    payload: {},
    rationale: null,
    confidence: null,
    status,
    applied_payload: null,
    refs: {},
  })
  const cs = (id: number, status: 'pending' | 'applied' | 'rejected', ops: ReturnType<typeof op>[], parent: number | null = null) => ({
    id,
    source: 'extraction' as const,
    entry_id: 1,
    parent_changeset_id: parent,
    status,
    ops,
    created_at: '',
    resolved_at: null,
  })
  const q = (id: number, status: 'open' | 'answered' | 'dismissed') => ({
    id,
    entry_id: 1,
    text: '',
    reason: '',
    status,
    answer_entry_id: null,
    created_at: '',
  })
  const run = (status: 'pending' | 'running' | 'succeeded' | 'failed') => ({
    id: 1,
    entry_id: 1,
    status,
    error: null,
    created_at: '',
    updated_at: '',
  })

  it("isn't offered before the detail loads or while a run is under way", () => {
    expect(canReprocess(null)).toBe(false)
    expect(canReprocess(detail({ run: run('pending') }))).toBe(false)
    expect(canReprocess(detail({ run: run('running') }))).toBe(false)
    expect(canReprocess(detail({ run: run('failed') }))).toBe(true)
    expect(canReprocess(detail({ run: run('succeeded') }))).toBe(true)
  })

  it('counts undecided ops in pending changesets, split-offs included, and open questions', () => {
    const d = detail({
      changesets: [
        cs(1, 'applied', [op('applied'), op('rejected')]),
        cs(2, 'pending', [op('pending'), op('pending')], 1),
        cs(3, 'pending', [op('pending'), op('rejected')]),
        cs(4, 'rejected', [op('rejected')]),
      ],
      questions: [q(1, 'open'), q(2, 'answered'), q(3, 'dismissed')],
    })
    expect(reprocessImpact(d)).toEqual({ ops: 3, questions: 1 })
  })

  it('says what is replaced, or nothing when nothing is', () => {
    expect(reprocessWarning({ ops: 0, questions: 0 })).toBeNull()
    expect(reprocessWarning({ ops: 1, questions: 0 })).toBe("Reading this again replaces what's waiting here: 1 proposed change.")
    expect(reprocessWarning({ ops: 3, questions: 1 })).toBe(
      "Reading this again replaces what's waiting here: 3 proposed changes and 1 open question.",
    )
    expect(reprocessWarning({ ops: 0, questions: 2 })).toBe("Reading this again replaces what's waiting here: 2 open questions.")
  })
})
