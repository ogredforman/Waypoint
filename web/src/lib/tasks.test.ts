import { describe, expect, it } from 'vitest'
import type { TaskStatus } from './api/enums.gen'
import type { LinkedObservation, Task } from './api/types.gen'
import { groupTasks, nestTasks, newStepRequest, newTaskRequest, observationTimeline, stepLabel, stepStatus, taskFields, taskPatch } from './tasks'

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

describe('nestTasks', () => {
  it("groups only top-level tasks and keeps each task's steps under it by position", () => {
    const tree = nestTasks([
      task(1, 'todo', 1),
      task(2, 'done', 4, { parent_task_id: 1 }),
      task(3, 'todo', 3, { parent_task_id: 1 }),
      task(4, 'in_progress', 5, { parent_task_id: 3 }),
      task(5, 'done', 6),
    ])
    expect(tree.groups.map((g) => [g.status, g.tasks.map((t) => t.id)])).toEqual([
      ['todo', [1]],
      ['done', [5]],
    ])
    expect(tree.steps.get(1)?.map((t) => t.id)).toEqual([3, 2])
    expect(tree.steps.get(3)?.map((t) => t.id)).toEqual([4])
  })

  it('treats a task whose parent is not in the list as top-level', () => {
    const tree = nestTasks([task(2, 'todo', 1, { parent_task_id: 9 })])
    expect(tree.groups[0].tasks.map((t) => t.id)).toEqual([2])
    expect(tree.steps.size).toBe(0)
  })
})

describe('steps', () => {
  it('builds a step request, ticks to done and back to todo, and names a step', () => {
    expect(newStepRequest('  Pick a runner ', 4)).toEqual({ title: 'Pick a runner', parent_task_id: 4 })
    expect(stepStatus(true)).toBe('done')
    expect(stepStatus(false)).toBe('todo')
    expect(stepLabel({ parent_task_id: 4, parent_title: 'Set up CI', number: 2 })).toBe('Step 2 of')
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

describe('observationTimeline', () => {
  const obs = (id: number, entry_id: number): LinkedObservation => ({
    id,
    entry_id,
    entry_created_at: `2026-09-0${entry_id}T10:00:00Z`,
    kind: 'progress',
    summary: `S${id}`,
    quote: '',
    confidence: 1,
  })

  it('groups consecutive observations by entry and keeps their order', () => {
    const groups = observationTimeline([obs(1, 1), obs(2, 1), obs(3, 2)])
    expect(groups.map((g) => [g.entryId, g.at, g.observations.map((o) => o.id)])).toEqual([
      [1, '2026-09-01T10:00:00Z', [1, 2]],
      [2, '2026-09-02T10:00:00Z', [3]],
    ])
  })

  it('is empty for no observations', () => {
    expect(observationTimeline([])).toEqual([])
  })
})
