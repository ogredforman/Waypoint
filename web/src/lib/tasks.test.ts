import { describe, expect, it } from 'vitest'
import type { TaskStatus } from './api/enums.gen'
import type { Task } from './api/types.gen'
import { groupTasks, newTaskRequest, taskFields, taskPatch } from './tasks'

function task(id: number, status: TaskStatus, position: number, extra: Partial<Task> = {}): Task {
  return {
    id,
    project_id: 1,
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

describe('groupTasks', () => {
  it('orders sections in progress, to do, done, dropped and sorts by position then id', () => {
    const groups = groupTasks([
      task(1, 'dropped', 1),
      task(2, 'todo', 5),
      task(3, 'done', 2),
      task(4, 'todo', 3),
      task(5, 'in_progress', 4),
      task(6, 'todo', 3),
    ])
    expect(groups.map((g) => g.status)).toEqual(['in_progress', 'todo', 'done', 'dropped'])
    expect(groups[1].label).toBe('To do')
    expect(groups[1].tasks.map((t) => t.id)).toEqual([4, 6, 2])
  })

  it('leaves out empty sections', () => {
    expect(groupTasks([task(1, 'todo', 1)]).map((g) => g.status)).toEqual(['todo'])
    expect(groupTasks([])).toEqual([])
  })
})

describe('taskPatch', () => {
  const t = task(1, 'todo', 1, { title: 'Dig', notes: 'north bed', size: 's', energy: 'low' })

  it('returns null when nothing changed', () => {
    expect(taskPatch(t, { ...taskFields(t), title: ' Dig ' })).toBeNull()
  })

  it('sends only changed fields, trimmed', () => {
    expect(taskPatch(t, { ...taskFields(t), title: 'Dig deeper ', notes: '' })).toEqual({ title: 'Dig deeper', notes: '' })
  })

  it('clears size and energy with null', () => {
    expect(taskPatch(t, { ...taskFields(t), size: '', energy: 'deep' })).toEqual({ size: null, energy: 'deep' })
  })
})

describe('newTaskRequest', () => {
  it('leaves out unset size and energy', () => {
    expect(newTaskRequest({ title: ' Dig ', size: '', energy: '' })).toEqual({ title: 'Dig' })
    expect(newTaskRequest({ title: 'Dig', size: 'xs', energy: 'low' })).toEqual({ title: 'Dig', size: 'xs', energy: 'low' })
  })
})
