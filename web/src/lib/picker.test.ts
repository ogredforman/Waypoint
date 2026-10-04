import { describe, expect, it } from 'vitest'
import type { Project } from './api/types.gen'
import { pickerOptions, selectedLabel, step } from './picker'

function project(id: number, title: string): Project {
  return { id, title, why: '', definition_of_done: '', status: 'active', created_at: '', updated_at: '', last_activity_at: '' }
}

const projects = [project(1, 'Garden shed'), project(2, 'Kiln'), project(3, 'Herb garden')]

describe('pickerOptions', () => {
  it('lists no project first, then the projects in order', () => {
    expect(pickerOptions(projects, '', 1, 'This project').map((o) => o.id)).toEqual([null, 1, 2, 3])
  })

  it('narrows by title but keeps no project', () => {
    expect(pickerOptions(projects, ' GARDEN', null, 'x').map((o) => o.label)).toEqual([
      'No project',
      'Garden shed',
      'Herb garden',
    ])
    expect(pickerOptions(projects, 'zzz', null, 'x').map((o) => o.label)).toEqual(['No project'])
  })

  it('shows a chosen project that is not in the list yet', () => {
    expect(pickerOptions([], '', 5, 'This project')).toEqual([
      { id: null, label: 'No project' },
      { id: 5, label: 'This project' },
    ])
  })
})

describe('selectedLabel', () => {
  it('names the chosen project, no project, or the missing label', () => {
    expect(selectedLabel(projects, 2, 'x')).toBe('Kiln')
    expect(selectedLabel(projects, null, 'x')).toBe('No project')
    expect(selectedLabel(projects, 9, 'This project')).toBe('This project')
  })
})

describe('step', () => {
  it('moves and wraps at either end', () => {
    expect(step(0, 1, 3)).toBe(1)
    expect(step(2, 1, 3)).toBe(0)
    expect(step(0, -1, 3)).toBe(2)
  })

  it('starts from the first or last with nothing highlighted', () => {
    expect(step(-1, 1, 3)).toBe(0)
    expect(step(-1, -1, 3)).toBe(2)
    expect(step(0, 1, 0)).toBe(-1)
  })
})
