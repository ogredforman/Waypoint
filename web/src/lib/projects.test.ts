import { describe, expect, it } from 'vitest'
import type { ProjectStatus } from './api/enums.gen'
import type { Project } from './api/types.gen'
import { atCap, capMessage, countActive, groupByStatus, newProjectRequest } from './projects'

function project(id: number, status: ProjectStatus): Project {
  return { id, title: `P${id}`, why: '', definition_of_done: '', status, created_at: '', updated_at: '' }
}

describe('groupByStatus', () => {
  it('orders sections active, paused, parked, done and keeps project order', () => {
    const groups = groupByStatus([project(1, 'done'), project(2, 'parked'), project(3, 'active'), project(4, 'paused'), project(5, 'active')])
    expect(groups.map((g) => g.status)).toEqual(['active', 'paused', 'parked', 'done'])
    expect(groups[0].label).toBe('Active')
    expect(groups[0].projects.map((p) => p.id)).toEqual([3, 5])
  })

  it('leaves out empty sections', () => {
    expect(groupByStatus([project(1, 'parked')]).map((g) => g.status)).toEqual(['parked'])
    expect(groupByStatus([])).toEqual([])
  })
})

describe('the active-project cap', () => {
  it('counts active projects only', () => {
    expect(countActive([project(1, 'active'), project(2, 'paused'), project(3, 'active')])).toBe(2)
  })

  it('is reached at the cap, not before', () => {
    expect(atCap(2, 3)).toBe(false)
    expect(atCap(3, 3)).toBe(true)
    expect(atCap(4, 3)).toBe(true) // the cap was lowered after they became active
  })

  it('never blocks when the cap is unknown', () => {
    expect(atCap(10, null)).toBe(false)
  })

  it('explains the cap calmly', () => {
    expect(capMessage(1, 1)).toBe('You have 1 active project, and the limit is 1. This one can wait in the parking lot until you pause or finish one.')
    expect(capMessage(3, 3)).toContain('3 active projects')
  })
})

describe('newProjectRequest', () => {
  it('trims fields and leaves out blank ones', () => {
    expect(newProjectRequest({ title: '  Garden  ', why: '   ', definitionOfDone: '', startActive: false })).toEqual({ title: 'Garden' })
  })

  it('sends why, definition of done, and active status when given', () => {
    expect(newProjectRequest({ title: 'Garden', why: ' Fresh food ', definitionOfDone: 'Beds planted', startActive: true })).toEqual({
      title: 'Garden',
      why: 'Fresh food',
      definition_of_done: 'Beds planted',
      status: 'active',
    })
  })
})
