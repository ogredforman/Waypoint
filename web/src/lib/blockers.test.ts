import { describe, expect, it } from 'vitest'
import type { Blocker, Task } from './api/types.gen'
import { newBlockerRequest, openTasks, splitBlockers } from './blockers'

function blocker(id: number, status: Blocker['status'], created_at: string, updated_at = created_at): Blocker {
  return {
    id,
    project_id: 7,
    task_id: null,
    description: `B${id}`,
    status,
    origin_entry_id: null,
    resolved_entry_id: null,
    created_at,
    updated_at,
  }
}

describe('splitBlockers', () => {
  it('puts open blockers oldest first and resolved ones most recently resolved first', () => {
    const { open, resolved } = splitBlockers([
      blocker(1, 'open', '2026-09-02'),
      blocker(2, 'resolved', '2026-09-01', '2026-09-03'),
      blocker(3, 'open', '2026-09-01'),
      blocker(4, 'resolved', '2026-09-01', '2026-09-05'),
    ])
    expect(open.map((b) => b.id)).toEqual([3, 1])
    expect(resolved.map((b) => b.id)).toEqual([4, 2])
  })

  it('handles an empty list', () => {
    expect(splitBlockers([])).toEqual({ open: [], resolved: [] })
  })
})

describe('openTasks', () => {
  it('keeps todo and in-progress tasks in position order', () => {
    const task = (id: number, status: Task['status'], position: number) => ({ id, status, position }) as Task
    const got = openTasks([task(1, 'done', 0), task(2, 'todo', 3), task(3, 'in_progress', 1), task(4, 'dropped', 2)])
    expect(got.map((t) => t.id)).toEqual([3, 2])
  })
})

describe('newBlockerRequest', () => {
  it('trims the description and leaves out an empty task', () => {
    expect(newBlockerRequest('  no key  ', '')).toEqual({ description: 'no key' })
    expect(newBlockerRequest('no key', 5)).toEqual({ description: 'no key', task_id: 5 })
  })
})
