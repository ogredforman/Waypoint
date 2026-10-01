import type { Blocker, CreateBlockerRequest, Task } from './api/types.gen'

// splitBlockers separates open blockers (oldest first, so the longest-standing
// one leads) from resolved ones (most recently resolved first).
export function splitBlockers(bs: Blocker[]): { open: Blocker[]; resolved: Blocker[] } {
  const open = bs.filter((b) => b.status === 'open').sort((a, b) => a.created_at.localeCompare(b.created_at) || a.id - b.id)
  const resolved = bs
    .filter((b) => b.status === 'resolved')
    .sort((a, b) => b.updated_at.localeCompare(a.updated_at) || b.id - a.id)
  return { open, resolved }
}

// openTasks lists the tasks a new blocker can be on: those still todo or in
// progress, in list order.
export function openTasks(ts: Task[]): Task[] {
  return ts
    .filter((t) => t.status === 'todo' || t.status === 'in_progress')
    .sort((a, b) => a.position - b.position || a.id - b.id)
}

// newBlockerRequest builds the POST /api/projects/{id}/blockers body. No task
// ('') leaves the blocker on the project alone.
export function newBlockerRequest(description: string, taskId: number | ''): CreateBlockerRequest {
  const req: CreateBlockerRequest = { description: description.trim() }
  if (taskId !== '') req.task_id = taskId
  return req
}
