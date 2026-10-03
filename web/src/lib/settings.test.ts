import { describe, expect, it } from 'vitest'
import { ApiError } from './api/client'
import type { Settings } from './api/types.gen'
import { capNote, capProblem, fieldProblems, parseCap, settingsPatch } from './settings'

const saved: Settings = { active_project_cap: 3, questions_per_entry: 2, interface_font: 'inter', diary_font: 'literata' }

describe('parseCap', () => {
  it('accepts whole numbers of at least 1', () => {
    expect(parseCap('1')).toBe(1)
    expect(parseCap(' 12 ')).toBe(12)
    expect(parseCap(4)).toBe(4)
  })

  it('rejects anything else', () => {
    for (const v of ['', '0', '-1', '1.5', 'abc', '1e3', 0, 2.5]) expect(parseCap(v), String(v)).toBeNull()
  })
})

describe('settingsPatch', () => {
  it('is null when nothing changed', () => {
    expect(settingsPatch(saved, { cap: '3', questions: 2 })).toEqual({ patch: null, problems: {} })
  })

  it('holds only the changed fields', () => {
    expect(settingsPatch(saved, { cap: '1', questions: 2 }).patch).toEqual({ active_project_cap: 1 })
    expect(settingsPatch(saved, { cap: 3, questions: 0 }).patch).toEqual({ questions_per_entry: 0 })
  })

  it('reports a bad cap instead of sending it', () => {
    expect(settingsPatch(saved, { cap: '0', questions: 1 })).toEqual({
      patch: null,
      problems: { active_project_cap: capProblem },
    })
  })
})

describe('capNote', () => {
  it('only speaks when the cap is below the active count', () => {
    expect(capNote(3, 3)).toBeNull()
    expect(capNote(2, 3)).toBeNull()
    expect(capNote(null, 1)).toBeNull()
    expect(capNote(3, null)).toBeNull()
    expect(capNote(3, 2)).toContain('Nothing gets paused')
  })
})

describe('fieldProblems', () => {
  it('maps invalid fields and puts the rest under form', () => {
    const err = new ApiError(422, 'invalid', 'Invalid.', [
      { field: 'questions_per_entry', problem: 'must be from 0 to 2, got 3' },
      { field: 'request', problem: 'set at least one setting' },
    ])
    expect(fieldProblems(err)).toEqual({
      questions_per_entry: 'Must be from 0 to 2, got 3.',
      form: 'Set at least one setting.',
    })
  })

  it('uses the message for other errors', () => {
    expect(fieldProblems(new ApiError(0, 'unreachable', 'Down.'))).toEqual({ form: 'Down.' })
    expect(fieldProblems(new Error('x'))).toEqual({ form: "Couldn't save your settings." })
  })
})
