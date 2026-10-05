import { describe, expect, it } from 'vitest'
import type { TaskStatus } from './api/enums.gen'
import type { Milestone, Task } from './api/types.gen'
import { milestoneLabel, milestonePatch, milestoneSections } from './milestones'
import { nestTasks } from './tasks'

function task(id: number, status: TaskStatus, position: number, extra: Partial<Task> = {}): Task {
  return {
    id,
    project_id: 1,
    milestone_id: null,
    parent_task_id: null,
    title: `T${id}`,
    notes: '',
    status,
    size: null,
    energy: null,
    position,
    origin_entry_id: null,
    created_at: '',
    updated_at: '',
    ...extra,
  }
}

function milestone(id: number, position: number, extra: Partial<Milestone> = {}): Milestone {
  return { id, project_id: 1, title: `M${id}`, description: '', status: 'open', position, created_at: '', updated_at: '', ...extra }
}

function shape(tasks: Task[], milestones: Milestone[]) {
  return milestoneSections(nestTasks(tasks), milestones).map((s) => ({
    milestone: s.milestone?.id ?? null,
    groups: s.groups.map((g) => [g.status, g.tasks.map((t) => t.id)]),
  }))
}

describe('milestoneSections', () => {
  it('is one section of everything when there are no milestones', () => {
    expect(shape([task(1, 'todo', 1), task(2, 'done', 2)], [])).toEqual([
      {
        milestone: null,
        groups: [
          ['todo', [1]],
          ['done', [2]],
        ],
      },
    ])
    expect(shape([], [])).toEqual([{ milestone: null, groups: [] }])
  })

  it('orders milestones by position, keeps empty ones, and puts no milestone last', () => {
    expect(
      shape(
        [task(1, 'todo', 1, { milestone_id: 20 }), task(2, 'in_progress', 2), task(3, 'todo', 3, { milestone_id: 99 })],
        [milestone(10, 2), milestone(20, 1)],
      ),
    ).toEqual([
      { milestone: 20, groups: [['todo', [1]]] },
      { milestone: 10, groups: [] },
      // A milestone that isn't listed counts as none.
      {
        milestone: null,
        groups: [
          ['in_progress', [2]],
          ['todo', [3]],
        ],
      },
    ])
  })

  it('leaves out an empty no-milestone section, and steps follow their parent', () => {
    expect(shape([task(1, 'todo', 1, { milestone_id: 10 }), task(2, 'todo', 2, { parent_task_id: 1 })], [milestone(10, 1)])).toEqual([
      { milestone: 10, groups: [['todo', [1]]] },
    ])
  })
})

describe('milestonePatch', () => {
  const m = milestone(1, 1, { title: 'MVP', description: 'the loop works' })

  it('sends only changed fields, trimmed, or null', () => {
    expect(milestonePatch(m, { title: ' MVP ', description: 'the loop works' })).toBeNull()
    expect(milestonePatch(m, { title: 'MVP ', description: ' ' })).toEqual({ description: '' })
  })
})

describe('milestoneLabel', () => {
  it('names closed milestones with their status', () => {
    expect(milestoneLabel(milestone(1, 1))).toBe('M1')
    expect(milestoneLabel(milestone(1, 1, { status: 'dropped' }))).toBe('M1 (dropped)')
  })
})
