import { describe, expect, it } from 'vitest'
import type { Blocker } from './api/types.gen'
import { splitBlockers } from './blockers'

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
