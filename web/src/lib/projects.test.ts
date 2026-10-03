import { describe, expect, it } from 'vitest'
import type { ProjectStatus } from './api/enums.gen'
import type { Project } from './api/types.gen'
import {
  activateCapMessage,
  activeProjects,
  parkedProjects,
  atCap,
  capMessage,
  countActive,
  groupByStatus,
  newProjectRequest,
  projectFields,
  projectPatch,
  statusOptions,
} from './projects'

function project(id: number, status: ProjectStatus): Project {
  return { id, title: `P${id}`, why: '', definition_of_done: '', status, created_at: '', updated_at: '' }
}

describe('groupByStatus', () => {
  it('orders sections active, paused, parked, done and keeps project order', () => {
    const groups = groupByStatus([
      project(6, 'archived'),
      project(1, 'done'),
      project(2, 'parked'),
      project(3, 'active'),
      project(4, 'paused'),
      project(5, 'active'),
    ])
    expect(groups.map((g) => g.status)).toEqual(['active', 'paused', 'parked', 'done', 'archived'])
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

describe('activeProjects', () => {
  it('keeps only active projects, in order', () => {
    expect(activeProjects([project(1, 'active'), project(2, 'parked'), project(3, 'active')]).map((p) => p.id)).toEqual([1, 3])
  })
})

describe('parkedProjects', () => {
  it('keeps only parked projects, in order', () => {
    expect(parkedProjects([project(1, 'parked'), project(2, 'paused'), project(3, 'parked')]).map((p) => p.id)).toEqual([1, 3])
  })
})

describe('projectPatch', () => {
  const p: Project = { ...project(1, 'active'), title: 'Garden', why: 'Fresh food', definition_of_done: 'Beds planted' }

  it('is null when nothing changed, ignoring surrounding space', () => {
    expect(projectPatch(p, projectFields(p))).toBeNull()
    expect(projectPatch(p, { title: ' Garden ', why: 'Fresh food  ', definitionOfDone: 'Beds planted' })).toBeNull()
  })

  it('sends only the changed fields, trimmed', () => {
    expect(projectPatch(p, { title: ' Allotment ', why: 'Fresh food', definitionOfDone: 'Beds planted' })).toEqual({ title: 'Allotment' })
  })

  it('clears why and definition of done to empty', () => {
    expect(projectPatch(p, { title: 'Garden', why: '  ', definitionOfDone: '' })).toEqual({ why: '', definition_of_done: '' })
  })
})

describe('statusOptions', () => {
  const disabled = (opts: ReturnType<typeof statusOptions>) => opts.filter((o) => o.disabled).map((o) => o.status)

  it('lists every status in section order', () => {
    expect(statusOptions('parked', 0, 3).map((o) => o.status)).toEqual(['active', 'paused', 'parked', 'done', 'archived'])
  })

  it('disables active at the cap for a project that is not active', () => {
    expect(disabled(statusOptions('parked', 3, 3))).toEqual(['active'])
    expect(disabled(statusOptions('paused', 2, 3))).toEqual([])
  })

  it('never disables anything for an active project or an unknown cap', () => {
    expect(disabled(statusOptions('active', 3, 3))).toEqual([])
    expect(disabled(statusOptions('parked', 9, null))).toEqual([])
  })

  it('explains the cap calmly', () => {
    expect(activateCapMessage(1, 1)).toBe("You have 1 active project, and the limit is 1. Pause or finish one when you'd like to make this one active.")
  })
})
