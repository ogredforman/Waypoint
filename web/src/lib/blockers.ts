import type { Blocker } from './api/types.gen'

// splitBlockers separates open blockers (oldest first, so the longest-standing
// one leads) from resolved ones (most recently resolved first).
export function splitBlockers(bs: Blocker[]): { open: Blocker[]; resolved: Blocker[] } {
  const open = bs.filter((b) => b.status === 'open').sort((a, b) => a.created_at.localeCompare(b.created_at) || a.id - b.id)
  const resolved = bs
    .filter((b) => b.status === 'resolved')
    .sort((a, b) => b.updated_at.localeCompare(a.updated_at) || b.id - a.id)
  return { open, resolved }
}
